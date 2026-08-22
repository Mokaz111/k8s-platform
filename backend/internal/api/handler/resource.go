package handler

import (
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/resource"
	"github.com/k8s-platform/console/internal/version"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceHandler struct {
	ResourceMgr *resource.Manager
	VersionMgr  *version.Manager
}

func NewResourceHandler(resourceMgr *resource.Manager, versionMgr *version.Manager) *ResourceHandler {
	return &ResourceHandler{
		ResourceMgr: resourceMgr,
		VersionMgr:  versionMgr,
	}
}

func parseGVK(apiVersion, kind string) (schema.GroupVersionKind, error) {
	if apiVersion == "" || kind == "" {
		return schema.GroupVersionKind{}, errcode.New(errcode.InvalidArgument, "apiVersion 和 kind 不能为空")
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionKind{}, errcode.Wrap(errcode.InvalidArgument, err, "apiVersion 格式错误")
	}
	return gv.WithKind(kind), nil
}

// validateScope 做两件事：
//  1. 校验 X-Cluster-Code / X-Namespace header 与路径参数一致（防止伪造）
//  2. 校验当前用户对该 cluster/namespace 是否有 RBAC 数据范围权限
//
// namespaced 资源：namespace 必须在用户允许范围内
// 集群级资源（PV/StorageClass 等）：仅校验 cluster scope
func (h *ResourceHandler) validateScope(c *gin.Context, gvk schema.GroupVersionKind, namespace string) error {
	clusterCode := c.Param("code")
	headerCluster := c.GetHeader("X-Cluster-Code")
	if headerCluster != "" && headerCluster != clusterCode {
		return errcode.New(errcode.ScopeDenied, "X-Cluster-Code 与路径不一致")
	}

	namespaced := h.ResourceMgr.IsNamespaced(gvk)

	// 集群级资源（如 PV/StorageClass/Namespace/Node）：只需 cluster scope
	if !namespaced {
		if err := middleware.RequireClusterScope(c, clusterCode); err != nil {
			return err
		}
		return nil
	}

	// 命名空间级资源
	headerNS := c.GetHeader("X-Namespace")
	if headerNS != "" && headerNS != namespace {
		return errcode.New(errcode.ScopeDenied, "X-Namespace 与路径不一致")
	}

	// namespace 为空：仅 cluster scope 用户可以 list all namespaces；namespace scope 用户被拒
	if namespace == "" {
		nsList, isFull := middleware.AllowedNamespaces(c, clusterCode)
		if isFull {
			return nil
		}
		// 仅 namespace scope 用户且未指定 namespace
		if len(nsList) == 0 {
			return errcode.New(errcode.ScopeDenied, "无权访问集群: "+clusterCode)
		}
		return errcode.New(errcode.InvalidArgument,
			"请指定 namespace 参数；您仅可访问: "+strings.Join(nsList, ", "))
	}

	// namespace 非空：精确校验
	if err := middleware.RequireNamespaceScope(c, clusterCode, namespace); err != nil {
		return err
	}
	return nil
}

func (h *ResourceHandler) ListResources(c *gin.Context) {
	code := c.Param("code")
	apiVersion := c.Param("apiVersion")
	kind := c.Param("kind")
	namespace := c.DefaultQuery("namespace", "")
	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))

	gvk, err := parseGVK(apiVersion, kind)
	if err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	if err := h.validateScope(c, gvk, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	res, err := h.ResourceMgr.ListResources(code, gvk, namespace, keyword, page, size)
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

func (h *ResourceHandler) GetResource(c *gin.Context) {
	code := c.Param("code")
	apiVersion := c.Param("apiVersion")
	kind := c.Param("kind")
	namespace := c.Param("namespace")
	name := c.Param("name")

	gvk, err := parseGVK(apiVersion, kind)
	if err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	if err := h.validateScope(c, gvk, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	obj, err := h.ResourceMgr.GetResource(code, gvk, namespace, name)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, obj)
}

type createResourceReq struct {
	YAML string `json:"yaml" binding:"required"`
}

func (h *ResourceHandler) CreateResource(c *gin.Context) {
	code := c.Param("code")
	apiVersion := c.Param("apiVersion")
	kind := c.Param("kind")

	gvk, err := parseGVK(apiVersion, kind)
	if err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	var rawBody []byte
	contentType := c.ContentType()

	if strings.Contains(contentType, "application/json") {
		var req createResourceReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
			return
		}
		rawBody = []byte(strings.TrimSpace(req.YAML))
	} else {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "读取请求体失败"))
			return
		}
		rawBody = body
	}

	if len(rawBody) == 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "YAML 内容不能为空"))
		return
	}

	operator := getCurrentUsername(c)
	operatorID := getCurrentUserID(c)
	_ = gvk
	_ = operator
	_ = operatorID

	patched, err := h.ResourceMgr.ApplyResource(code, rawBody, "apply")
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}

	err = h.VersionMgr.SnapshotAfter(patched, operator, "ui", "创建/更新资源", operatorID, code)
	if err != nil {
		h.VersionMgr.Log.Warnf("SnapshotAfter failed: %v", err)
	}

	response.OK(c, patched)
}

type updateResourceReq struct {
	YAML string `json:"yaml" binding:"required"`
}

func (h *ResourceHandler) UpdateResource(c *gin.Context) {
	code := c.Param("code")
	apiVersion := c.Param("apiVersion")
	kind := c.Param("kind")
	namespace := c.Param("namespace")
	name := c.Param("name")

	gvk, err := parseGVK(apiVersion, kind)
	if err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	if err := h.validateScope(c, gvk, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	operator := getCurrentUsername(c)
	operatorID := getCurrentUserID(c)

	err = h.VersionMgr.BeforeSave(code, gvk, namespace, name, operator, operatorID, "UI 更新资源前快照")
	if err != nil {
		h.VersionMgr.Log.Warnf("BeforeSave snapshot failed: %v", err)
	}

	var rawBody []byte
	contentType := c.ContentType()

	if strings.Contains(contentType, "application/json") {
		var req updateResourceReq
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
			return
		}
		rawBody = []byte(strings.TrimSpace(req.YAML))
	} else {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "读取请求体失败"))
			return
		}
		rawBody = body
	}

	if len(rawBody) == 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "YAML 内容不能为空"))
		return
	}

	patched, err := h.ResourceMgr.ApplyResource(code, rawBody, "apply")
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}

	err = h.VersionMgr.SnapshotAfter(patched, operator, "ui", "UI 更新资源后快照", operatorID, code)
	if err != nil {
		h.VersionMgr.Log.Warnf("SnapshotAfter failed: %v", err)
	}

	response.OK(c, patched)
}

func (h *ResourceHandler) DeleteResource(c *gin.Context) {
	code := c.Param("code")
	apiVersion := c.Param("apiVersion")
	kind := c.Param("kind")
	namespace := c.Param("namespace")
	name := c.Param("name")

	gvk, err := parseGVK(apiVersion, kind)
	if err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	if err := h.validateScope(c, gvk, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	operator := getCurrentUsername(c)
	operatorID := getCurrentUserID(c)

	_, err = h.VersionMgr.SnapshotBefore(code, namespace, apiVersion, kind, name, operator, "ui", "删除资源前快照", operatorID)
	if err != nil {
		h.VersionMgr.Log.Warnf("SnapshotBefore failed: %v", err)
	}

	if err := h.ResourceMgr.DeleteResource(code, gvk, namespace, name); err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}

	response.OK(c, gin.H{"deleted": true, "name": name})
}
