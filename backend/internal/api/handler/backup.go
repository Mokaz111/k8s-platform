package handler

import (
	"context"
	"fmt"
	"io"
	"mime"
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
	Mode        string   `json:"mode"` // single / namespace_batch (兼容前端传 object/namespace)
	Namespace   string   `json:"namespace"`
	Namespaces  []string `json:"namespaces"`
	TargetKind  string   `json:"target_kind"`
	KindFilter  []string `json:"kind_filter"`
	TargetName  string   `json:"target_name"`
	StorageType string   `json:"storage_type"`
	// legacy 前端兼容字段：
	Scope string `json:"scope"` // "object" => single / "namespace" => namespace_batch
}

func modeFromRequest(reqMode, scope string) backup.BackupMode {
	switch strings.ToLower(strings.TrimSpace(reqMode)) {
	case "single", "object":
		return backup.BackupModeSingle
	case "namespace_batch", "namespace-batch", "batch", "namespace":
		return backup.BackupModeNamespaceBatch
	}
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "object", "single":
		return backup.BackupModeSingle
	case "namespace", "batch":
		return backup.BackupModeNamespaceBatch
	}
	return backup.BackupModeSingle
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
	req.Namespace = strings.TrimSpace(req.Namespace)
	mode := modeFromRequest(req.Mode, req.Scope)

	operatorID, operator, ok := mustCurrentUser(c)
	if !ok {
		return
	}

	in := &backup.SubmitBackupInput{
		ClusterCode: code,
		Mode:        mode,
		StorageType: req.StorageType,
		Operator:    operator,
		OperatorID:  operatorID,
	}
	switch mode {
	case backup.BackupModeSingle:
		in.Namespace = req.Namespace
		in.TargetKind = req.TargetKind
		in.TargetName = req.TargetName
		if in.TargetKind == "" || in.TargetName == "" {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "target_kind 和 target_name 不能为空"))
			return
		}
		// RBAC 数据范围校验：namespace 为空（集群级资源）时退化为 cluster scope 校验
		if err := middleware.RequireNamespaceScope(c, code, in.Namespace); err != nil {
			response.Fail(c, err.(*errcode.Error))
			return
		}
	case backup.BackupModeNamespaceBatch:
		// 合并 namespaces：兼容旧前端用 scope=namespace 且 namespace 单值
		in.Namespaces = req.Namespaces
		if len(in.Namespaces) == 0 && req.Namespace != "" {
			in.Namespaces = []string{req.Namespace}
		}
		in.KindFilter = req.KindFilter
		if len(in.KindFilter) == 0 && req.TargetKind != "" {
			in.KindFilter = []string{req.TargetKind}
		}
		if len(in.Namespaces) == 0 {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "命名空间批量模式必须至少选择一个命名空间"))
			return
		}
		if len(in.KindFilter) == 0 {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "命名空间批量模式必须至少选择一种资源类型"))
			return
		}
		// 批量模式：RBAC 对每个 namespaces 做范围校验；若有任一个命名空间无权限则失败
		for _, ns := range in.Namespaces {
			if err := middleware.RequireNamespaceScope(c, code, ns); err != nil {
				response.Fail(c, err.(*errcode.Error))
				return
			}
		}
		for _, kind := range in.KindFilter {
			if kindNeedsClusterScope(kind) {
				if err := middleware.RequireClusterScope(c, code); err != nil {
					response.Fail(c, errcode.New(errcode.ScopeDenied, "批量备份包含集群级资源，需要集群范围权限"))
					return
				}
				break
			}
		}
	}

	task, err := h.Mgr.SubmitBackupTask(context.Background(), in)
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
	backupType := c.Query("backup_type")

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
		BackupType:  backupType,
	}

	// RBAC 数据范围过滤：
	if clusterCode != "" {
		nsList, isFull := middleware.AllowedNamespaces(c, clusterCode)
		if !isFull {
			if len(nsList) == 0 {
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
	if err := requireBackupRecordScope(c, item); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}
	response.OK(c, item)
}

// DownloadBackup 下载单个备份 YAML（支持 local/nfs 任意存储类型，Manager 内部做 storage_path 语义兼容）
func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "id 无效"))
		return
	}
	// 先 Get 一次：顺便做 RBAC 数据范围校验
	item, gerr := h.Mgr.Get(context.Background(), id)
	if gerr != nil {
		if ec, ok := gerr.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, gerr))
		}
		return
	}
	if err := requireBackupRecordScope(c, item); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	res, derr := h.Mgr.Download(context.Background(), id)
	if derr != nil {
		if ec, ok := derr.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, derr))
		}
		return
	}
	defer res.Reader.Close()

	contentType := res.ContentType
	if contentType == "" {
		contentType = "application/yaml"
	}
	// 将中文名做 MIME 编码，避免浏览器下载文件名乱码
	encodedName := mime.QEncoding.Encode("UTF-8", res.SuggestName)
	c.Header("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", res.SuggestName, encodedName))
	c.Header("Content-Type", contentType)
	if res.SizeBytes > 0 {
		c.Header("Content-Length", fmt.Sprintf("%d", res.SizeBytes))
	}
	c.Status(200)
	// 流式传输：边读边写 + 定期 flush，避免大文件把内存撑爆
	buf := make([]byte, 256*1024)
	wr := c.Writer
	for {
		n, rerr := res.Reader.Read(buf)
		if n > 0 {
			if _, werr := wr.Write(buf[:n]); werr != nil {
				break
			}
			wr.Flush()
		}
		if rerr != nil {
			if rerr != io.EOF {
				_ = rerr
			}
			break
		}
	}
}

type restoreBackupReq struct {
	TargetCluster   string `json:"target_cluster_code"`
	TargetClusterV2 string `json:"target_cluster"` // 兼容前端两种 key
	Mode            string `json:"mode"`
	TargetNamespace string `json:"target_namespace"` // 恢复到指定命名空间（可选）
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

	src, err := h.Mgr.Get(context.Background(), id)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	if err := requireBackupRecordScope(c, src); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}
	if backup.IsNamespaceBatch(src.BackupType) {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "批量备份暂不支持恢复，请按对象分别备份后再恢复"))
		return
	}
	targetCode := strings.TrimSpace(req.TargetCluster)
	if targetCode == "" {
		targetCode = strings.TrimSpace(req.TargetClusterV2)
	}
	if targetCode == "" {
		targetCode = src.ClusterCode
	}
	srcNS := ""
	if src.Namespace != nil {
		srcNS = *src.Namespace
	}
	targetNS := strings.TrimSpace(req.TargetNamespace)
	if targetNS == "" {
		targetNS = srcNS
	}
	if targetNS != "" {
		if err := middleware.RequireNamespaceScope(c, targetCode, targetNS); err != nil {
			response.Fail(c, err.(*errcode.Error))
			return
		}
	} else if err := middleware.RequireClusterScope(c, targetCode); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	operatorID, operator, ok := mustCurrentUser(c)
	if !ok {
		return
	}

	task, err := h.Mgr.SubmitRestoreTask(context.Background(), &backup.SubmitRestoreInput{
		BackupID:          id,
		TargetClusterCode: targetCode,
		TargetNamespace:   strings.TrimSpace(req.TargetNamespace),
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
	item, err := h.Mgr.Get(context.Background(), id)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	if err := requireBackupRecordScope(c, item); err != nil {
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

func kindNeedsClusterScope(kind string) bool {
	switch kind {
	case "Namespace", "Node", "PersistentVolume", "ClusterRole", "ClusterRoleBinding",
		"StorageClass", "PriorityClass", "CustomResourceDefinition", "CSIDriver", "CSINode":
		return true
	}
	return false
}

func requireBackupRecordScope(c *gin.Context, item *models.BackupTask) error {
	if item == nil {
		return errcode.New(errcode.BackupNotFound)
	}
	nsStr := ""
	if item.Namespace != nil {
		nsStr = *item.Namespace
	}
	if backup.IsNamespaceBatch(item.BackupType) || strings.HasPrefix(nsStr, "multi:") {
		pt := middleware.GetPermTree(c)
		if pt == nil {
			return errcode.New(errcode.ScopeDenied, "权限未加载，拒绝访问备份")
		}
		if pt.HasClusterScope(item.ClusterCode) {
			return nil
		}
		if uid, ok := middleware.CurrentUserID(c); ok && item.OperatorID == uid {
			return nil
		}
		if nsStr != "" && !strings.HasPrefix(nsStr, "multi:") {
			return middleware.RequireNamespaceScope(c, item.ClusterCode, nsStr)
		}
		return errcode.New(errcode.ScopeDenied, "无权访问该批量备份")
	}
	return middleware.RequireNamespaceScope(c, item.ClusterCode, nsStr)
}
