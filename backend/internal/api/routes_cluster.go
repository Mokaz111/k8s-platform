package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
	"github.com/k8s-platform/console/internal/api/middleware"
)

func RegisterClusterRoutes(rg *gin.RouterGroup, h *handler.ClusterHandler) {
	clusters := rg.Group("/clusters")
	{
		clusters.POST("",
			middleware.RequirePermission("cluster:create"),
			h.ImportCluster)

		clusters.GET("",
			middleware.RequirePermission("cluster:list"),
			h.ListClusters)

		clusters.GET("/:code",
			middleware.RequirePermission("cluster:list"),
			h.GetCluster)

		clusters.PUT("/:code",
			middleware.RequirePermission("cluster:update"),
			h.UpdateCluster)

		clusters.DELETE("/:code",
			middleware.RequirePermission("cluster:delete"),
			h.DeleteCluster)

		clusters.POST("/:code/ping",
			middleware.RequirePermission("cluster:ping"),
			h.PingCluster)
	}
}
