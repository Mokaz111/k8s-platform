package handler

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
	ws "github.com/k8s-platform/console/internal/websocket"
)

// PodLogHandler 触发 Pod 日志流，通过 WebSocket Hub 推送给订阅了对应频道的客户端
type PodLogHandler struct {
	svc *ws.PodLogService

	mu     sync.Mutex
	active map[string]contextCancel // key: channel name → cancel
}

type contextCancel struct {
	cancel func()
	done   chan struct{}
}

func NewPodLogHandler(svc *ws.PodLogService) *PodLogHandler {
	return &PodLogHandler{
		svc:    svc,
		active: make(map[string]contextCancel),
	}
}

// StreamPodLogsHandler GET /api/v1/clusters/:code/pods/:namespace/:pod/logs?container=&follow=&tail=
//
// 触发 Pod 日志流：本接口异步启动后台 StreamPodLogs，返回订阅频道名
// 客户端需先 WebSocket 连接 /api/v1/ws 并 subscribe 该频道才能收到日志
func (h *PodLogHandler) StreamPodLogsHandler(c *gin.Context) {
	clusterCode := c.Param("code")
	namespace := c.Param("namespace")
	podName := c.Param("pod")
	container := c.Query("container")

	follow := false
	if v := c.Query("follow"); v == "1" || v == "true" || v == "yes" {
		follow = true
	}
	tailLines := 0
	if v := c.Query("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			tailLines = n
		}
	}

	if clusterCode == "" || namespace == "" || podName == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "code/namespace/pod 不能为空"))
		return
	}

	channel := ws.PodLogChannelName(clusterCode, namespace, podName)

	// stream 持续时间上限（防止 follow 模式泄露 goroutine）
	streamTimeout := 30 * time.Minute
	if !follow {
		// 非 follow 模式：单次拉取，5 分钟内必须读完
		streamTimeout = 5 * time.Minute
	}

	// 若同一频道已有活跃 stream，则先取消旧的（避免重复）
	h.mu.Lock()
	if old, ok := h.active[channel]; ok {
		old.cancel()
		delete(h.active, channel)
		// 释放锁等待旧 goroutine 退出（避免阻塞 HTTP）
		go func() { <-old.done }()
	}

	ctx, cancel := context.WithCancel(c.Request.Context())
	ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
	ctx, timeoutCancel := context.WithTimeout(ctx, streamTimeout)
	done := make(chan struct{})
	h.active[channel] = contextCancel{cancel: cancel, done: done}
	h.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			h.mu.Lock()
			delete(h.active, channel)
			h.mu.Unlock()
			cancel()
			timeoutCancel()
		}()

		_ = h.svc.StreamPodLogs(ctx, clusterCode, namespace, podName, container, follow, tailLines)
	}()

	response.OK(c, gin.H{
		"channel":     channel,
		"follow":      follow,
		"tail":        tailLines,
		"container":   container,
		"pod":         podName,
		"namespace":   namespace,
		"cluster":     clusterCode,
		"ws_endpoint": "/api/v1/ws",
		"action":      "subscribe",
		"hint":        "请使用 WebSocket 连接 /api/v1/ws?token=<JWT>，并发送 {action:'subscribe', channel:'<channel>'} 订阅日志",
	})
}
