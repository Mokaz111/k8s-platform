package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
	"github.com/k8s-platform/console/internal/api/middleware"
)

func RegisterHelmRoutes(rg *gin.RouterGroup, h *handler.HelmHandler) {
	// 平台级制品仓库（helm repo），不绑定具体集群
	repos := rg.Group("/helm")
	{
		repos.GET("/repos",
			middleware.RequirePermission("helm:view"),
			h.ListRepos)
		repos.POST("/repos",
			middleware.RequirePermission("helm:install"),
			h.AddRepo)
		repos.DELETE("/repos/:name",
			middleware.RequirePermission("helm:install"),
			h.RemoveRepo)
		repos.POST("/repos/update",
			middleware.RequirePermission("helm:install"),
			h.UpdateRepos)
		repos.GET("/charts",
			middleware.RequirePermission("helm:view"),
			h.SearchCharts)
	}

	helm := rg.Group("/clusters/:code/helm")
	{
		// 列出 Helm releases
		helm.GET("/releases",
			middleware.RequirePermission("helm:view"),
			h.ListReleases)

		// 安装/升级 release
		helm.POST("/releases",
			middleware.RequirePermission("helm:install"),
			h.InstallRelease)

		// 卸载 release
		helm.DELETE("/releases/:namespace/:name",
			middleware.RequirePermission("helm:uninstall"),
			h.UninstallRelease)

		// 回滚 release
		helm.POST("/releases/:namespace/:name/rollback",
			middleware.RequirePermission("helm:rollback"),
			h.RollbackRelease)

		// 查看修订历史
		helm.GET("/releases/:namespace/:name/history",
			middleware.RequirePermission("helm:view"),
			h.ListHistory)

		// 检查 helm CLI 是否可用
		helm.GET("/check",
			middleware.RequirePermission("helm:view"),
			h.CheckHelmCLI)
	}
}
