package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
)

func RegisterHelmRoutes(rg *gin.RouterGroup, h *handler.HelmHandler) {
	helm := rg.Group("/clusters/:code/helm")
	{
		// 列出 Helm releases
		helm.GET("/releases",
			RequirePermission("helm:view"),
			h.ListReleases)

		// 安装/升级 release
		helm.POST("/releases",
			RequirePermission("helm:install"),
			h.InstallRelease)

		// 卸载 release
		helm.DELETE("/releases/:namespace/:name",
			RequirePermission("helm:uninstall"),
			h.UninstallRelease)

		// 回滚 release
		helm.POST("/releases/:namespace/:name/rollback",
			RequirePermission("helm:rollback"),
			h.RollbackRelease)

		// 查看修订历史
		helm.GET("/releases/:namespace/:name/history",
			RequirePermission("helm:view"),
			h.ListHistory)

		// 检查 helm CLI 是否可用
		helm.GET("/check",
			h.CheckHelmCLI)
	}
}
