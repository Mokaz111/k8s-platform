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
	"github.com/k8s-platform/console/pkg/logger"
)

// API Server 主入口
// 职责：HTTP 服务、路由注册、中间件、同步 API 处理
// 异步任务（备份/恢复/Informer 同步/健康检测）由 cmd/task-worker 独立进程承担
func main() {
	// 1. 加载配置
	cfg, err := config.Load("")
	if err != nil {
		panic(fmt.Sprintf("load config failed: %v", err))
	}

	// 2. 初始化全局日志
	log := logger.New(cfg.Log)
	defer log.Sync()

	log.Infof("🚀 starting %s (API Server) mode=%s ...", cfg.Server.Name, cfg.Server.Mode)

	// 3. 引导所有基础设施（DB/Redis/KMS/插件/连接池）
	app, err := bootstrap.NewApp(cfg, log, bootstrap.RoleAPIServer)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer app.Shutdown()

	// 4. 注册 HTTP 路由（含中间件）
	router := app.RegisterAPIRoutes()

	// 5. 启动 HTTP Server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Infof("🌐 HTTP server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// 6. 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Infof("🛑 received signal %v, shutting down gracefully...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("http server shutdown error: %v", err)
	}

	log.Infof("✅ API Server exited")
}
