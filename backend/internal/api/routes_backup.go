package api

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/handler"
)

func RegisterBackupRoutes(rg *gin.RouterGroup, h *handler.BackupHandler) {
	rg.POST("/clusters/:code/backups",
		RequirePermission("backup:create"),
		h.CreateBackup)

	backups := rg.Group("/backups")
	{
		backups.GET("",
			RequirePermission("backup:list"),
			h.ListBackups)

		backups.GET("/:id",
			RequirePermission("backup:list"),
			h.GetBackup)

		// 下载备份文件：统一通过 Manager.Download → storage.Download 处理
		backups.GET("/:id/download",
			RequirePermission("backup:list"),
			h.DownloadBackup)

		backups.POST("/:id/restore",
			RequirePermission("backup:restore"),
			h.RestoreBackup)

		backups.DELETE("/:id",
			RequirePermission("backup:delete"),
			h.DeleteBackup)
	}
}
