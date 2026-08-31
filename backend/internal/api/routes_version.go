package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
	"github.com/k8s-platform/console/internal/api/middleware"
)

func RegisterVersionRoutes(rg *gin.RouterGroup, h *handler.VersionHandler) {
	versions := rg.Group("/versions")
	{
		versions.GET("",
			middleware.RequirePermission("version:list"),
			h.ListVersions)

		versions.GET("/diff",
			middleware.RequirePermission("version:diff"),
			h.DiffVersions)

		versions.POST("/diff",
			middleware.RequirePermission("version:diff"),
			h.DiffVersions)

		versions.POST("/rollback",
			middleware.RequirePermission("version:rollback"),
			h.Rollback)
	}
}
