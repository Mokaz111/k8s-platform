package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/version"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

type VersionHandler struct {
	VersionMgr *version.Manager
}

func NewVersionHandler(versionMgr *version.Manager) *VersionHandler {
	return &VersionHandler{VersionMgr: versionMgr}
}

func (h *VersionHandler) ListVersions(c *gin.Context) {
	clusterCode := c.Query("cluster_code")
	namespace := c.DefaultQuery("namespace", "")
	apiVersion := c.Query("api_version")
	kind := c.Query("kind")
	name := c.Query("name")

	if clusterCode == "" || apiVersion == "" || kind == "" || name == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument,
			"cluster_code, api_version, kind, name 为必填查询参数"))
		return
	}

	// RBAC 数据范围校验：namespace 为空（集群级资源）时退化为 cluster scope 校验
	if err := middleware.RequireNamespaceScope(c, clusterCode, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	items, err := h.VersionMgr.ListVersions(clusterCode, namespace, apiVersion, kind, name)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}

	response.OKList(c, int64(len(items)), items)
}

type diffVersionsReq struct {
	ClusterCode string `json:"cluster_code" binding:"required"`
	Namespace   string `json:"namespace"`
	APIVersion  string `json:"api_version" binding:"required"`
	Kind        string `json:"kind" binding:"required"`
	Name        string `json:"name" binding:"required"`
	// Seq 为 0 表示与集群中的当前对象对比，>0 为历史快照序号
	SeqA int `json:"seq_a"`
	SeqB int `json:"seq_b"`
}

func (h *VersionHandler) DiffVersions(c *gin.Context) {
	clusterCode := c.Query("cluster_code")
	namespace := c.DefaultQuery("namespace", "")
	apiVersion := c.Query("api_version")
	kind := c.Query("kind")
	name := c.Query("name")
	seqAStr := c.Query("seq_a")
	seqBStr := c.Query("seq_b")

	var req diffVersionsReq
	if seqAStr != "" && seqBStr != "" {
		seqA, err := strconv.Atoi(seqAStr)
		if err != nil {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "seq_a 格式错误"))
			return
		}
		seqB, err := strconv.Atoi(seqBStr)
		if err != nil {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "seq_b 格式错误"))
			return
		}
		req = diffVersionsReq{
			ClusterCode: clusterCode,
			Namespace:   namespace,
			APIVersion:  apiVersion,
			Kind:        kind,
			Name:        name,
			SeqA:        seqA,
			SeqB:        seqB,
		}
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
			return
		}
	}

	if req.ClusterCode == "" || req.APIVersion == "" || req.Kind == "" || req.Name == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument,
			"cluster_code, api_version, kind, name 为必填参数"))
		return
	}
	if req.SeqA < 0 || req.SeqB < 0 {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "seq_a / seq_b 不能为负数；0 表示当前版本"))
		return
	}
	if req.SeqA == req.SeqB {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "seq_a 和 seq_b 不能相同"))
		return
	}

	// RBAC 数据范围校验：namespace 为空（集群级资源）时退化为 cluster scope 校验
	if err := middleware.RequireNamespaceScope(c, req.ClusterCode, req.Namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	yamlA, yamlB, err := h.VersionMgr.DiffVersions(
		req.ClusterCode, req.Namespace, req.APIVersion, req.Kind, req.Name, req.SeqA, req.SeqB,
	)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}

	response.OK(c, gin.H{
		"yaml_a": yamlA,
		"yaml_b": yamlB,
		"seq_a":  req.SeqA,
		"seq_b":  req.SeqB,
	})
}

type rollbackReq struct {
	ClusterCode string `json:"cluster_code" binding:"required"`
	Namespace   string `json:"namespace"`
	APIVersion  string `json:"api_version" binding:"required"`
	Kind        string `json:"kind" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Seq         int    `json:"seq" binding:"required,min=1"`
}

func (h *VersionHandler) Rollback(c *gin.Context) {
	var req rollbackReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}

	// RBAC 数据范围校验：回滚前确认用户对该集群/命名空间有访问权
	// namespace 为空（集群级资源）时退化为 cluster scope 校验
	if err := middleware.RequireNamespaceScope(c, req.ClusterCode, req.Namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	operatorID, operator, ok := mustCurrentUser(c)
	if !ok {
		return
	}

	patched, err := h.VersionMgr.Rollback(
		req.ClusterCode, req.Namespace, req.APIVersion, req.Kind, req.Name, req.Seq,
		operator, operatorID,
	)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}

	response.OK(c, gin.H{
		"rolled_back": true,
		"seq":         req.Seq,
		"object":      patched,
	})
}
