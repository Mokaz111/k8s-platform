package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/backup"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

type BackupHandler struct {
	Mgr *backup.Manager
}

func NewBackupHandler(mgr *backup.Manager) *BackupHandler {
	return &BackupHandler{Mgr: mgr}
}

type createBackupReq struct {
	Namespace   string `json:"namespace"`
	TargetKind  string `json:"target_kind" binding:"required"`
	TargetName  string `json:"target_name" binding:"required"`
	StorageType string `json:"storage_type"`
}

func (h *BackupHandler) CreateBackup(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "cluster code 不能为空"))
		return
	}
	var req createBackupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}
	req.TargetKind = strings.TrimSpace(req.TargetKind)
	req.TargetName = strings.TrimSpace(req.TargetName)
	if req.TargetKind == "" || req.TargetName == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "target_kind 和 target_name 不能为空"))
		return
	}

	operatorID := getCurrentUserID(c)
	operator := getCurrentUsername(c)

	task, err := h.Mgr.SubmitBackupTask(context.Background(), &backup.SubmitBackupInput{
		ClusterCode: code,
		Namespace:   strings.TrimSpace(req.Namespace),
		TargetKind:  req.TargetKind,
		TargetName:  req.TargetName,
		StorageType: strings.TrimSpace(req.StorageType),
		Operator:    operator,
		OperatorID:  operatorID,
	})
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, task)
}

func (h *BackupHandler) ListBackups(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	clusterCode := c.Query("cluster_code")
	statusStr := c.Query("status")
	keyword := c.Query("keyword")

	var status models.BackupTaskStatus
	switch statusStr {
	case string(models.BackupStatusPending),
		string(models.BackupStatusRunning),
		string(models.BackupStatusSuccess),
		string(models.BackupStatusFailed),
		string(models.BackupStatusCancelled):
		status = models.BackupTaskStatus(statusStr)
	}

	res, err := h.Mgr.List(context.Background(), &backup.ListInput{
		Page:        page,
		Size:        size,
		ClusterCode: clusterCode,
		Status:      status,
		Keyword:     keyword,
	})
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OKList(c, res.Total, res.Items)
}

func (h *BackupHandler) GetBackup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "id 无效"))
		return
	}
	item, err := h.Mgr.Get(context.Background(), id)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, item)
}

type restoreBackupReq struct {
	TargetClusterCode string `json:"target_cluster_code"`
	Mode              string `json:"mode"`
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "id 无效"))
		return
	}
	var req restoreBackupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}
	mode := backup.RestoreMode(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = backup.RestoreModeCreateNew
	}

	operatorID := getCurrentUserID(c)
	operator := getCurrentUsername(c)

	task, err := h.Mgr.SubmitRestoreTask(context.Background(), &backup.SubmitRestoreInput{
		BackupID:          id,
		TargetClusterCode: strings.TrimSpace(req.TargetClusterCode),
		Operator:          operator,
		OperatorID:        operatorID,
		Mode:              mode,
	})
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, task)
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "id 无效"))
		return
	}
	if err := h.Mgr.Delete(context.Background(), id); err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, gin.H{"deleted": true, "id": id})
}

func getCurrentUsername(c *gin.Context) string {
	if v, ok := c.Get("current_username"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if v := c.GetHeader("X-User-Name"); v != "" {
		return v
	}
	return ""
}
