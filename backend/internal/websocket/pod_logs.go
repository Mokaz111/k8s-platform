package websocket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

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

func (s *PodLogService) pushLine(channel string, line podLogLine) {
	payload, err := json.Marshal(line)
	if err != nil {
		return
	}
	out, err := json.Marshal(Message{
		Type:    TypePodLogs,
		Channel: channel,
		Data:    json.RawMessage(payload),
	})
	if err != nil {
		return
	}
	s.hub.BroadcastToChannel(channel, out)
}

// WaitForSubscribers 等到至少有一个客户端订阅该频道，或 ctx 取消。
// 避免 HTTP 触发后立刻推送 tail 日志时，前端还没完成 WebSocket subscribe。
func (s *PodLogService) WaitForSubscribers(ctx context.Context, channel string) {
	if s.hub.ChannelSubscriberCount(channel) > 0 {
		return
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if s.hub.ChannelSubscriberCount(channel) > 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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

	channel := PodLogChannelName(clusterCode, namespace, podName)
	base := podLogLine{
		ClusterCode:   clusterCode,
		Namespace:     namespace,
		PodName:       podName,
		ContainerName: containerName,
		Follow:        follow,
	}

	pushErr := func(msg string) {
		line := base
		line.Line = msg
		line.Eof = true
		s.pushLine(channel, line)
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, 3*time.Second)
	s.WaitForSubscribers(waitCtx, channel)
	waitCancel()

	_, restCfg, err := s.clusterMgr.GetDynamicClient(clusterCode)
	if err != nil {
		pushErr("打开日志流失败: " + err.Error())
		return err
	}
	if restCfg == nil {
		err := errcode.New(errcode.ClusterConnectFail, "无法获取集群 rest.Config")
		pushErr(err.Error())
		return err
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		wrapped := errcode.Wrap(errcode.ClusterConnectFail, err, "创建 kubernetes clientset 失败")
		pushErr(wrapped.Error())
		return wrapped
	}

	opts := &corev1.PodLogOptions{
		Container: containerName,
		Follow:    follow,
	}
	if tailLines > 0 {
		tail := int64(tailLines)
		opts.TailLines = &tail
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		wrapped := errcode.Wrap(errcode.K8SAPIError, err, "打开 Pod 日志流失败")
		pushErr(wrapped.Error())
		return wrapped
	}
	defer func() { _ = stream.Close() }()

	reader := bufio.NewReaderSize(stream, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, readErr := reader.ReadString('\n')
		if line != "" {
			item := base
			item.Line = strings.TrimRight(line, "\r\n")
			s.pushLine(channel, item)
		}

		if readErr != nil {
			if readErr == io.EOF {
				item := base
				item.Eof = true
				s.pushLine(channel, item)
				return nil
			}
			wrapped := errcode.Wrap(errcode.K8SAPIError, readErr, "读取 Pod 日志流失败")
			pushErr(wrapped.Error())
			return wrapped
		}
	}
}

// PodLogChannelName 返回 Pod 日志订阅频道名
// 频道格式：pod_logs:{clusterCode}:{namespace}:{podName}
func PodLogChannelName(clusterCode, namespace, podName string) string {
	return fmt.Sprintf("%s:%s:%s:%s", TypePodLogs, clusterCode, namespace, podName)
}
