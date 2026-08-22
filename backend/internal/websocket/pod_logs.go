package websocket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/pkg/errcode"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// PodLogService 封装 Pod 日志流读取与推送逻辑
type PodLogService struct {
	hub        *Hub
	clusterMgr *cluster.Manager
}

func NewPodLogService(hub *Hub, mgr *cluster.Manager) *PodLogService {
	return &PodLogService{hub: hub, clusterMgr: mgr}
}

// podLogLine 单条 Pod 日志行（推送给客户端的 data 结构）
type podLogLine struct {
	ClusterCode   string `json:"cluster_code"`
	Namespace     string `json:"namespace"`
	PodName       string `json:"pod_name"`
	ContainerName string `json:"container_name,omitempty"`
	Line          string `json:"line"`
	Follow        bool   `json:"follow,omitempty"`
	Eof           bool   `json:"eof,omitempty"`
}

// StreamPodLogs 读取 Pod 日志，逐行通过 Hub 推送给订阅了
// channel = "pod_logs:{clusterCode}:{ns}:{podName}" 的客户端
//
// follow=true 时持续 stream（--tail -f）；follow=false 时一次性拉取
// context 取消（客户端关闭/超时）时立即关闭 Stream
func (s *PodLogService) StreamPodLogs(ctx context.Context, clusterCode, namespace, podName, containerName string, follow bool, tailLines int) error {
	if clusterCode == "" || namespace == "" || podName == "" {
		return errcode.New(errcode.InvalidArgument, "clusterCode/namespace/podName 不能为空")
	}

	// 1. 获取 dynamic client + rest.Config
	_, restCfg, err := s.clusterMgr.GetDynamicClient(clusterCode)
	if err != nil {
		return err
	}
	if restCfg == nil {
		return errcode.New(errcode.ClusterConnectFail, "无法获取集群 rest.Config")
	}

	// 2. 创建 clientset（Pod Logs API 在 CoreV1 而非 Dynamic）
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return errcode.Wrap(errcode.ClusterConnectFail, err, "创建 kubernetes clientset 失败")
	}

	// 3. 构造 PodLogOptions
	opts := &corev1.PodLogOptions{
		Container: containerName,
		Follow:    follow,
	}
	if tailLines > 0 {
		tail := int64(tailLines)
		opts.TailLines = &tail
	}

	// 4. 获取日志 Stream（io.ReadCloser）
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return errcode.Wrap(errcode.K8SAPIError, err, "打开 Pod 日志流失败")
	}
	defer func() { _ = stream.Close() }()

	// 5. 构造订阅频道：pod_logs:{clusterCode}:{ns}:{podName}
	channel := PodLogChannelName(clusterCode, namespace, podName)

	// 6. 逐行读取并推送
	reader := bufio.NewReaderSize(stream, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, readErr := reader.ReadString('\n')
		if line != "" {
			payload, _ := json.Marshal(podLogLine{
				ClusterCode:   clusterCode,
				Namespace:     namespace,
				PodName:       podName,
				ContainerName: containerName,
				Line:          strings.TrimRight(line, "\r\n"),
				Follow:        follow,
			})
			out, _ := json.Marshal(Message{
				Type:    TypePodLogs,
				Channel: channel,
				Data:    json.RawMessage(payload),
			})
			s.hub.BroadcastToChannel(channel, out)
		}

		if readErr != nil {
			if readErr == io.EOF {
				// 流结束：follow=false 时为正常 EOF；follow=true 时表示 Kubernetes 端流被关闭
				// 推送一个 eof 标记给客户端
				payload, _ := json.Marshal(podLogLine{
					ClusterCode:   clusterCode,
					Namespace:     namespace,
					PodName:       podName,
					ContainerName: containerName,
					Follow:        follow,
					Eof:           true,
				})
				out, _ := json.Marshal(Message{
					Type:    TypePodLogs,
					Channel: channel,
					Data:    json.RawMessage(payload),
				})
				s.hub.BroadcastToChannel(channel, out)
				return nil
			}
			return errcode.Wrap(errcode.K8SAPIError, readErr, "读取 Pod 日志流失败")
		}
	}
}

// PodLogChannelName 返回 Pod 日志订阅频道名
// 频道格式：pod_logs:{clusterCode}:{namespace}:{podName}
func PodLogChannelName(clusterCode, namespace, podName string) string {
	return fmt.Sprintf("%s:%s:%s:%s", TypePodLogs, clusterCode, namespace, podName)
}
