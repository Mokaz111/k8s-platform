package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/auth"
	ws "github.com/k8s-platform/console/internal/websocket"
)

// RegisterWebsocketRoutes 注册 WebSocket 与 Pod 日志流相关路由
//
// GET /api/v1/ws                              WebSocket 握手（用 ?token=xxx 传 JWT，仅 Auth 中间件）
// GET /api/v1/clusters/:code/pods/:namespace/:pod/logs
//   触发 Pod 日志流（Auth + RBAC 中间件）
func RegisterWebsocketRoutes(r *gin.RouterGroup, authSvc *auth.Service, wsHandler *ws.Handler, podLogHandler *handler.PodLogHandler) {
	// WebSocket 握手路由：仅鉴权（不需要 RBAC，因为后续按 channel 订阅实现细粒度过滤）
	// 用单独的子分组挂 AuthMiddleware，避免影响其他路由的 RBAC
	wsGroup := r.Group("")
	wsGroup.Use(middleware.AuthMiddleware(authSvc))
	{
		wsGroup.GET("/ws", wsHandler.WsHandler)
	}

	// Pod 日志流触发路由：Auth + RBAC
	authorized := r.Group("")
	authorized.Use(middleware.AuthMiddleware(authSvc))
	authorized.Use(middleware.RBACMiddleware(authSvc))
	{
		authorized.GET("/clusters/:code/pods/:namespace/:pod/logs",
			RequirePermission("cluster:list"),
			podLogHandler.StreamPodLogsHandler)
	}
}
