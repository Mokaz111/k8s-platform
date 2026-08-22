package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
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
	req.Namespace = strings.TrimSpace(req.Namespace)

	// RBAC 数据范围校验：namespace 为空（集群级资源）时退化为 cluster scope 校验
	if err := middleware.RequireNamespaceScope(c, code, req.Namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
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

	in := &backup.ListInput{
		Page:        page,
		Size:        size,
		ClusterCode: clusterCode,
		Status:      status,
		Keyword:     keyword,
	}

	// RBAC 数据范围过滤：
	//   - 带 cluster_code（单集群场景）：按用户可访问的命名空间过滤
	//   - 不带 cluster_code（跨集群场景）：按用户可访问的集群列表过滤
	if clusterCode != "" {
		nsList, isFull := middleware.AllowedNamespaces(c, clusterCode)
		if !isFull {
			if len(nsList) == 0 {
				// namespace scope 用户对该集群无任何命名空间权，直接返回空
				response.OKList(c, 0, []models.BackupTask{})
				return
			}
			in.Namespaces = nsList
		}
	} else {
		codes, isFull := middleware.AllowedClusters(c)
		if !isFull {
			if len(codes) == 0 {
				response.OKList(c, 0, []models.BackupTask{})
				return
			}
			in.ClusterCodes = codes
		}
	}

	res, err := h.Mgr.List(context.Background(), in)
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
	// RBAC 数据范围校验：用户只能查看自己有权限的集群/命名空间的备份
	nsStr := ""
	if item.Namespace != nil {
		nsStr = *item.Namespace
	}
	if err := middleware.RequireNamespaceScope(c, item.ClusterCode, nsStr); err != nil {
		response.Fail(c, err.(*errcode.Error))
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

	// 先查源备份，做 RBAC 数据范围校验
	src, err := h.Mgr.Get(context.Background(), id)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	srcNS := ""
	if src.Namespace != nil {
		srcNS = *src.Namespace
	}
	// 校验对源备份所在集群/命名空间的读权限
	if err := middleware.RequireNamespaceScope(c, src.ClusterCode, srcNS); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}
	// 确定目标集群并校验写权限（恢复是集群级写操作）
	targetCode := strings.TrimSpace(req.TargetClusterCode)
	if targetCode == "" {
		targetCode = src.ClusterCode
	}
	if err := middleware.RequireClusterScope(c, targetCode); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
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
	// 先查备份，做 RBAC 数据范围校验，再删除
	item, err := h.Mgr.Get(context.Background(), id)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	nsStr := ""
	if item.Namespace != nil {
		nsStr = *item.Namespace
	}
	if err := middleware.RequireNamespaceScope(c, item.ClusterCode, nsStr); err != nil {
		response.Fail(c, err.(*errcode.Error))
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
