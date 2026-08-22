package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
)

func RegisterVersionRoutes(rg *gin.RouterGroup, h *handler.VersionHandler) {
	versions := rg.Group("/versions")
	{
		versions.GET("",
			RequirePermission("version:list"),
			h.ListVersions)

		versions.GET("/diff",
			RequirePermission("version:diff"),
			h.DiffVersions)

		versions.POST("/diff",
			RequirePermission("version:diff"),
			h.DiffVersions)

		versions.POST("/rollback",
			RequirePermission("version:rollback"),
			h.Rollback)
	}
}
