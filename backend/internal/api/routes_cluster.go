package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
)

const (
	PermKey = "required_permission"
)

func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(PermKey, code)
		c.Next()
	}
}

func RegisterClusterRoutes(rg *gin.RouterGroup, h *handler.ClusterHandler) {
	clusters := rg.Group("/clusters")
	{
		clusters.POST("",
			RequirePermission("cluster:create"),
			h.ImportCluster)

		clusters.GET("",
			RequirePermission("cluster:list"),
			h.ListClusters)

		clusters.GET("/:code",
			RequirePermission("cluster:list"),
			h.GetCluster)

		clusters.PUT("/:code",
			RequirePermission("cluster:update"),
			h.UpdateCluster)

		clusters.DELETE("/:code",
			RequirePermission("cluster:delete"),
			h.DeleteCluster)

		clusters.POST("/:code/ping",
			RequirePermission("cluster:ping"),
			h.PingCluster)
	}
}
