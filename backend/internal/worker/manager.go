package worker

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/bootstrap"
	"github.com/k8s-platform/console/internal/config"
	ws "github.com/k8s-platform/console/internal/websocket"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// Manager 统一管理所有 Worker 的生命周期（启动、停止、依赖注入）
type Manager struct {
	cfg  *config.Config
	log  *logger.Logger
	app  *bootstrap.App

	workers []Worker
	wg      sync.WaitGroup
	once    sync.Once
}

type Worker interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
}

func NewManager(cfg *config.Config, log *logger.Logger, app *bootstrap.App) *Manager {
	return &Manager{cfg: cfg, log: log, app: app}
}

// StartAll 启动所有 Worker（异步）
func (m *Manager) StartAll() {
	m.workers = []Worker{
		// 真实 Backup Worker：消费 Redis Stream 备份/恢复任务
		NewBackupWorker(m.cfg, m.log, m.app.DB, m.app.Redis, m.app.PluginMgr, m.app.ClusterMgr),
		// MVP 占位：Informer Sync 逻辑由 API Server 端 InformerMgr 按需懒启动
		NewNoopWorker("InformerSyncWorker"),
		// 集群健康检测（MVP 最小版本）
		NewClusterHealthWorker(m.log),
		// WebSocket 通知 Worker（设计文档 9.2 节三通道）
		// Hub 自身已订阅 Redis PubSub 3 个 channel（notifier:task_progress/cluster_event/pod_logs）
		// 本 Worker 在 task-worker 进程内运行：Hub 在 API Server 进程已自订阅，
		// 故此处仅做 Redis 连通性健康检查 + 启动日志记录
		NewPubSubNotifierWorker(m.log, m.app.Redis, m.app.WSHub),
	}

	for _, w := range m.workers {
		w := w
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			ctx := context.Background()
			m.log.Infof("▶️ starting worker: %s", w.Name())
			if err := w.Start(ctx); err != nil {
				m.log.Errorf("❌ worker %s exited with error: %v", w.Name(), err)
			} else {
				m.log.Infof("✅ worker %s stopped cleanly", w.Name())
			}
		}()
	}
}

// StopAll 停止所有 Worker
func (m *Manager) StopAll() {
	m.once.Do(func() {
		for i := len(m.workers) - 1; i >= 0; i-- {
			w := m.workers[i]
			m.log.Infof("⏹️ stopping worker: %s", w.Name())
			if err := w.Stop(); err != nil {
				m.log.Errorf("worker %s stop error: %v", w.Name(), err)
			}
		}
		m.wg.Wait()
	})
}

// HealthServer 轻量 HTTP Server（健康检查 + Prometheus 指标）
func (m *Manager) HealthServer(addr string) *http.Server {
	gin.SetMode("release")
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "role": "task-worker", "time": time.Now()})
	})
	r.GET("/workers", func(c *gin.Context) {
		names := make([]string, 0, len(m.workers))
		for _, w := range m.workers {
			names = append(names, w.Name())
		}
		c.JSON(200, gin.H{"workers": names})
	})
	// TODO: /metrics (Prometheus)

	return &http.Server{Addr: addr, Handler: r, ReadTimeout: 10 * time.Second}
}

// ---------- 占位 Worker ----------

// NoopWorker 空实现 Worker，仅用于骨架占位。后续替换为真实实现。
type NoopWorker struct{ name string }

func NewNoopWorker(name string) *NoopWorker { return &NoopWorker{name: name} }

func (n *NoopWorker) Name() string                                            { return n.name }
func (n *NoopWorker) Start(ctx context.Context) error                         { <-ctx.Done(); return nil }
func (n *NoopWorker) Stop() error                                             { return nil }

// NewClusterHealthWorker MVP 占位：仅打日志，实际逻辑后续实现
type clusterHealthWorker struct{ log *logger.Logger }

func NewClusterHealthWorker(log *logger.Logger) *clusterHealthWorker { return &clusterHealthWorker{log: log} }
func (w *clusterHealthWorker) Name() string                           { return "ClusterHealthWorker" }
func (w *clusterHealthWorker) Start(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	w.log.Infof("ClusterHealthWorker started (interval=5m, MVP placeholder)")
	for {
		select {
		case <-ticker.C:
			w.log.Debugf("ClusterHealthWorker tick (MVP placeholder)")
		case <-ctx.Done():
			return nil
		}
	}
}
func (w *clusterHealthWorker) Stop() error { return nil }

// ---------- WebSocket 通知 Worker ----------
//
// 设计文档 9.2 节三通道（Pod 日志流 / 任务进度推送 / 集群事件通知）。
//
// 实现策略：Hub 自身订阅 Redis PubSub 3 个 channel（notifier:task_progress /
// notifier:cluster_event / notifier:pod_logs），见 internal/websocket/hub.go。
//
// 本 Worker 仅在 task-worker 进程内运行，作用是：
//  1. 健康检查 Redis 连通性（Hub 订阅依赖 Redis）
//  2. 兜底启动 Hub（如果 Worker 进程的 Hub 未启动，会调用 Start；通常 Worker 进程无 WS 客户端，
//     Hub.Start 实际无效果，订阅消息会被丢弃）
//  3. 周期打日志，标识 WebSocket 通知链路存活
type PubSubNotifierWorker struct {
	log  *logger.Logger
	rdb  *redis.Client
	hub  *ws.Hub
	stop chan struct{}
	once sync.Once
}

func NewPubSubNotifierWorker(log *logger.Logger, rdb *redis.Client, hub *ws.Hub) *PubSubNotifierWorker {
	return &PubSubNotifierWorker{
		log:  log,
		rdb:  rdb,
		hub:  hub,
		stop: make(chan struct{}),
	}
}

func (w *PubSubNotifierWorker) Name() string { return "WebSocketNotifier" }

func (w *PubSubNotifierWorker) Start(ctx context.Context) error {
	w.log.Info("📡 WebSocketNotifier started (Hub self-subscribes Redis PubSub 3 channels)")

	// 兜底启动 Hub（API Server 进程内 Hub 已在 NewApp 中 Start；Worker 进程内 Hub 若未启动则启动）
	if w.hub != nil {
		w.hub.Start()
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-w.stop:
			return nil
		case <-ticker.C:
			// Redis 连通性健康检查（Hub 订阅依赖 Redis）
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := w.rdb.Ping(pingCtx).Err(); err != nil {
				w.log.Warnf("WebSocketNotifier: Redis ping failed: %v", err)
			}
			cancel()
		}
	}
}

func (w *PubSubNotifierWorker) Stop() error {
	w.once.Do(func() { close(w.stop) })
	return nil
}
