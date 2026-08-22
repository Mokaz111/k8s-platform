package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/k8s-platform/console/internal/bootstrap"
	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/worker"
	"github.com/k8s-platform/console/pkg/logger"
)

// Task Worker 主入口（独立进程，可水平扩缩容）
// 职责：
//   - BackupWorker: 消费 Redis Stream 备份/恢复任务
//   - InformerSyncWorker: 按需启动/停止 Informer + List 限流同步
//   - ClusterHealthWorker: 周期性集群连通性检测
//   - WebSocketNotifier: 推送任务进度/集群事件到 Redis PubSub（API Server 端订阅转给浏览器）
func main() {
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Sprintf("load config failed: %v", err))
	}

	log := logger.New(cfg.Log)
	defer log.Sync()

	log.Infof("🚀 starting %s (Task Worker) ...", cfg.Server.Name)

	app, err := bootstrap.NewApp(cfg, log, bootstrap.RoleTaskWorker)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer app.Shutdown()

	// 启动 Worker Manager（统一管理所有 Worker 生命周期）
	wm := worker.NewManager(cfg, log, app)
	wm.StartAll()

	// Worker 进程也启动一个轻量 HTTP 端口，提供健康检查 / Prometheus metrics / 手动触发同步
	healthSrv := wm.HealthServer(fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.WorkerPort))
	go func() {
		log.Infof("🔧 Worker health endpoint listening on %s", healthSrv.Addr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("worker health server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Infof("🛑 received signal %v, stopping workers gracefully...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	wm.StopAll()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("worker health server shutdown error: %v", err)
	}

	log.Infof("✅ Task Worker exited")
}
