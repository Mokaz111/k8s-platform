package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api"
	"github.com/k8s-platform/console/internal/api/handler"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/internal/backup"
	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/helm"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/internal/plugins"
	"github.com/k8s-platform/console/internal/resource"
	"github.com/k8s-platform/console/internal/version"
	ws "github.com/k8s-platform/console/internal/websocket"
	"github.com/k8s-platform/console/pkg/crypto"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// AppRole 区分启动角色
type AppRole int

const (
	RoleAPIServer  AppRole = 1
	RoleTaskWorker AppRole = 2
)

// App 应用全局容器，贯穿各模块的依赖注入容器
type App struct {
	Cfg   *config.Config
	Log   *logger.Logger
	DB    *gorm.DB
	Redis *redis.Client
	KMS   *crypto.AESGCM
	Role  AppRole

	// 业务模块
	AuthSvc     *auth.Service
	ClusterMgr  *cluster.Manager
	InformerMgr *resource.InformerManager
	ResourceMgr *resource.Manager
	VersionMgr  *version.Manager
	BackupMgr   *backup.Manager
	PluginMgr   *plugins.Manager
	HelmMgr     *helm.Manager

	// WebSocket Hub（设计文档 9.2 节三通道：Pod 日志流 / 任务进度推送 / 集群事件通知）
	WSHub *ws.Hub
}

// NewApp 引导基础设施 + 实例化所有业务模块
func NewApp(cfg *config.Config, log *logger.Logger, role AppRole) (*App, error) {
	app := &App{Cfg: cfg, Log: log, Role: role}

	// 1. KMS 初始化（本地 AES 实现）
	kms, err := crypto.NewAESGCM(cfg.KMS.LocalAESKeyHex, cfg.KMS.KeyVersion, cfg.KMS.PreviousAESKeyHex)
	if err != nil {
		return nil, fmt.Errorf("init kms failed: %w", err)
	}
	app.KMS = kms
	log.Infof("🔐 KMS provider=%s initialized (key_version=%s)", cfg.KMS.Provider, kms.KeyVersion())

	// 2. PostgreSQL 初始化
	db, err := initPostgres(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("init postgres failed: %w", err)
	}
	app.DB = db
	log.Info("📦 PostgreSQL connected")

	if cfg.Database.AutoMigrate && role == RoleAPIServer {
		if err := models.AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("auto migrate failed: %w", err)
		}
		log.Info("📦 DB schema auto migrated")
		if err := models.SeedInitialData(db, log); err != nil {
			return nil, fmt.Errorf("seed initial data failed: %w", err)
		}
		log.Info("🌱 Initial seed data applied")
	}

	// 3. Redis 初始化
	rdb, err := initRedis(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("init redis failed: %w", err)
	}
	app.Redis = rdb
	log.Info("⚡ Redis connected")

	// 4. 实例化所有业务模块（按依赖顺序）
	// 4.1 Auth Service（JWT + RBAC + Redis 缓存）
	app.AuthSvc = auth.NewService(db, rdb, &cfg.Auth, log)
	log.Info("🔑 Auth service initialized")

	// 4.2 Plugin Manager（先初始化，Backup Manager 依赖它）
	app.PluginMgr = plugins.NewManager(cfg, log)
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()
	if err := app.PluginMgr.Init(initCtx); err != nil {
		return nil, fmt.Errorf("init plugin manager failed: %w", err)
	}
	log.Info("🔌 Plugin manager initialized")

	// 4.3 Cluster Manager（kubeconfig 加解密 + Dynamic Client 连接池）
	app.ClusterMgr = cluster.NewManager(db, kms, cfg)
	log.Info("🌐 Cluster manager initialized")

	// 4.4 Informer Manager（懒加载 + List 限流 + 并发信号量）
	app.InformerMgr = resource.NewInformerManager(app.ClusterMgr, &cfg.Informer, log)
	log.Info("📊 Informer manager initialized (lazy_start=true)")

	// 4.5 Resource Manager（Dynamic Client 封装 + CRUD）
	app.ResourceMgr = resource.NewManager(app.ClusterMgr, app.InformerMgr, log)
	log.Info("📋 Resource manager initialized")

	// 4.6 Version Manager（快照 + diff + 回滚）
	app.VersionMgr = version.NewManager(db, app.ResourceMgr, log)
	log.Info("📜 Version manager initialized")

	// 4.7 Backup Manager（Redis Stream 任务提交）
	app.BackupMgr = backup.NewManager(db, rdb, app.PluginMgr, log)
	log.Info("💾 Backup manager initialized")

	// 4.8 Helm Manager（通过 k8s secrets 列表 + helm CLI 安装/卸载/回滚）
	app.HelmMgr = helm.NewManager(app.ClusterMgr, log)
	log.Info("⎈ Helm manager initialized")

	// 4.8 WebSocket Hub（设计文档 9.2 节三通道：Pod 日志流 / 任务进度推送 / 集群事件通知）
	// Hub 自身订阅 Redis PubSub 3 个 channel，故仅在 API Server 进程内启动（Worker 端 Hub 无客户端）
	app.WSHub = ws.NewHub(rdb, log)
	log.Info("🌐 WebSocket Hub initialized")

	log.Info("✅ All modules initialized")

	// 仅在 API Server 进程启动 Hub（Hub 内部订阅 Redis，需要客户端连接才有意义）
	if role == RoleAPIServer {
		app.WSHub.Start()
		log.Info("🌐 WebSocket Hub started (subscribing 3 Redis PubSub channels)")
	}

	return app, nil
}

// RegisterAPIRoutes 注册所有路由（API Server 调用）
func (a *App) RegisterAPIRoutes() *gin.Engine {
	gin.SetMode(a.Cfg.Server.Mode)
	r := gin.New()

	// 全局中间件
	r.Use(middleware.CORS(&a.Cfg.Security))
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog(a.Log, &a.Cfg.Security))
	r.Use(middleware.Recovery(a.Log))

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "role": "api-server", "time": time.Now()})
	})
	r.GET("/readyz", func(c *gin.Context) {
		if err := a.pingDependencies(c); err != nil {
			c.JSON(503, gin.H{"status": "unready", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})

	// API v1 分组
	v1 := r.Group("/api/v1")
	{
		// 平台信息
		v1.GET("", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"name":    a.Cfg.Server.Name,
				"version": "0.2.0-mvp",
				"status":  "running",
				"modules": []string{"auth", "clusters", "resources", "versions", "backups", "plugins"},
			})
		})

		// ---- 公开路由（无需鉴权）+ 受保护路由 ----
		// auth/user/role/audit 路由函数内部自行挂载 AuthMiddleware + RBACMiddleware
		handlers := api.NewHandlers(a.AuthSvc, a.DB)
		api.RegisterAuthRoutes(v1, a.AuthSvc, handlers)
		api.RegisterUserRoutes(v1, a.AuthSvc, handlers)
		api.RegisterRoleRoutes(v1, a.AuthSvc, handlers)
		api.RegisterAuditRoutes(v1, a.AuthSvc, handlers)

		// ---- 集群/资源/版本/备份路由 ----
		// 这些路由文件只设置 RequirePermission(code)，需要外层先挂 Auth + RBAC 中间件
		authorized := v1.Group("")
		authorized.Use(middleware.AuthMiddleware(a.AuthSvc))
		authorized.Use(middleware.RBACMiddleware(a.AuthSvc))
		authorized.Use(middleware.AuditMiddleware(a.DB, a.Log, &a.Cfg.Security))
		{
			// 集群管理
			clusterHandler := handler.NewClusterHandler(a.ClusterMgr)
			api.RegisterClusterRoutes(authorized, clusterHandler)

			// 资源管理
			resourceHandler := handler.NewResourceHandler(a.ResourceMgr, a.VersionMgr)
			api.RegisterResourceRoutes(authorized, resourceHandler)

			// 版本历史
			versionHandler := handler.NewVersionHandler(a.VersionMgr)
			api.RegisterVersionRoutes(authorized, versionHandler)

			// 备份管理
			backupHandler := handler.NewBackupHandler(a.BackupMgr)
			api.RegisterBackupRoutes(authorized, backupHandler)

			// Helm 应用管理
			helmHandler := &handler.HelmHandler{Mgr: a.HelmMgr}
			api.RegisterHelmRoutes(authorized, helmHandler)
		}

		// ---- WebSocket 路由（设计文档 9.2 节三通道）----
		// GET /api/v1/ws                                WebSocket 握手（用 ?token=xxx 传 JWT）
		// GET /api/v1/clusters/:code/pods/:namespace/:pod/logs 触发 Pod 日志流
		wsHandler := ws.NewHandler(a.WSHub, a.AuthSvc)
		podLogSvc := ws.NewPodLogService(a.WSHub, a.ClusterMgr)
		podLogHandler := handler.NewPodLogHandler(podLogSvc)
		api.RegisterWebsocketRoutes(v1, a.AuthSvc, wsHandler, podLogHandler)
	}

	return r
}

// Shutdown 优雅关闭时释放资源
func (a *App) Shutdown() {
	// 先停 WebSocket Hub（关闭所有客户端连接 + Redis PubSub 订阅）
	if a.WSHub != nil {
		a.WSHub.Stop()
	}
	// 停 Informer（释放 Watch 连接）
	if a.InformerMgr != nil {
		a.InformerMgr.StopAll()
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	if a.DB != nil {
		if sqldb, err := a.DB.DB(); err == nil {
			_ = sqldb.Close()
		}
	}
}

func (a *App) pingDependencies(ctx context.Context) error {
	if sqldb, err := a.DB.DB(); err == nil {
		if err := sqldb.PingContext(ctx); err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
	}
	if err := a.Redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	return nil
}

// ---------- init helpers ----------

func initPostgres(cfg *config.Config, log *logger.Logger) (*gorm.DB, error) {
	var gl gormlogger.Interface
	{
		zapLvl := zap.WarnLevel
		switch cfg.Log.Level {
		case "debug":
			zapLvl = zap.InfoLevel
		case "error", "fatal", "panic":
			zapLvl = zap.ErrorLevel
		}
		gl = gormlogger.New(
			zap.NewStdLog(log.Zap()),
			gormlogger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  gormlogger.Warn,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		)
		_ = zapLvl
	}

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN()), &gorm.Config{
		Logger: gl,
	})
	if err != nil {
		return nil, err
	}
	if sqldb, err := db.DB(); err == nil {
		sqldb.SetMaxIdleConns(cfg.Database.MaxIdleConns)
		sqldb.SetMaxOpenConns(cfg.Database.MaxOpenConns)
		sqldb.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)
	}
	return db, nil
}

func initRedis(cfg *config.Config, _ *logger.Logger) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr(),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		MinIdleConns: cfg.Redis.MinIdleConns,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	})
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Redis.DialTimeout)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return rdb, nil
}
