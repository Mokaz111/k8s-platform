package handler

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

type ClusterHandler struct {
	Mgr *cluster.Manager
}

func NewClusterHandler(mgr *cluster.Manager) *ClusterHandler {
	return &ClusterHandler{Mgr: mgr}
}

type importClusterReq struct {
	Name             string `json:"name" binding:"required"`
	Code             string `json:"code" binding:"required"`
	KubeconfigBase64 string `json:"kubeconfig_base64"`
	KubeconfigText   string `json:"kubeconfig_text"`
	Description      string `json:"description"`
}

func (h *ClusterHandler) ImportCluster(c *gin.Context) {
	var req importClusterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if req.Name == "" || req.Code == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "name 和 code 不能为空"))
		return
	}

	var raw []byte
	if strings.TrimSpace(req.KubeconfigBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.KubeconfigBase64))
		if err != nil {
			response.Fail(c, errcode.Wrap(errcode.KubeconfigInvalid, err, "kubeconfig_base64 解码失败"))
			return
		}
		raw = decoded
	} else if strings.TrimSpace(req.KubeconfigText) != "" {
		raw = []byte(strings.TrimSpace(req.KubeconfigText))
	} else {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "kubeconfig_base64 或 kubeconfig_text 必须提供其中一个"))
		return
	}

	creatorID := getCurrentUserID(c)

	created, err := h.Mgr.ImportCluster(req.Name, req.Code, raw, creatorID)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	if req.Description != "" {
		desc := req.Description
		updated, _ := h.Mgr.Update(req.Code, &cluster.UpdateInput{Description: &desc}, creatorID)
		if updated != nil {
			created = updated
		}
	}

	response.OK(c, created)
}

func (h *ClusterHandler) ListClusters(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	keyword := c.Query("keyword")

	res, err := h.Mgr.List(page, size, keyword)
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

func (h *ClusterHandler) GetCluster(c *gin.Context) {
	code := c.Param("code")
	item, err := h.Mgr.GetByCode(code)
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

type updateClusterReq struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	KubeconfigBase64 *string `json:"kubeconfig_base64"`
	KubeconfigText   *string `json:"kubeconfig_text"`
	Labels           *string `json:"labels"`
}

func (h *ClusterHandler) UpdateCluster(c *gin.Context) {
	code := c.Param("code")

	var req updateClusterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}

	in := &cluster.UpdateInput{
		Name:        req.Name,
		Description: req.Description,
	}

	if req.KubeconfigBase64 != nil && strings.TrimSpace(*req.KubeconfigBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*req.KubeconfigBase64))
		if err != nil {
			response.Fail(c, errcode.Wrap(errcode.KubeconfigInvalid, err, "kubeconfig_base64 解码失败"))
			return
		}
		in.NewRawKubeconfig = decoded
	} else if req.KubeconfigText != nil && strings.TrimSpace(*req.KubeconfigText) != "" {
		in.NewRawKubeconfig = []byte(strings.TrimSpace(*req.KubeconfigText))
	}

	if req.Labels != nil && *req.Labels != "" {
		raw := []byte(*req.Labels)
		in.LabelsJSON = &raw
	}

	operatorID := getCurrentUserID(c)
	updated, err := h.Mgr.Update(code, in, operatorID)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, updated)
}

func (h *ClusterHandler) DeleteCluster(c *gin.Context) {
	code := c.Param("code")
	if err := h.Mgr.Delete(code); err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, gin.H{"deleted": true, "code": code})
}

// tempPingReq 导入集群前的临时连通性检测请求体（code 为 "_" 时生效）
type tempPingReq struct {
	KubeconfigText string `json:"kubeconfig_text"`
}

func (h *ClusterHandler) PingCluster(c *gin.Context) {
	code := c.Param("code")

	// code 为 "_" 时走「临时 ping」：用请求体里的 kubeconfig_text 直接检测，
	// 用于导入集群前的测试连接（kubeconfig 尚未入库）
	if code == "_" {
		var req tempPingReq
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.KubeconfigText) == "" {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "临时检测必须提供 kubeconfig_text"))
			return
		}
		result, err := h.Mgr.PingWithKubeconfig([]byte(req.KubeconfigText))
		if err != nil {
			if ec, ok := err.(*errcode.Error); ok {
				response.Fail(c, ec)
			} else {
				response.Fail(c, errcode.Wrap(errcode.Internal, err))
			}
			return
		}
		response.OK(c, result)
		return
	}

	result, err := h.Mgr.Ping(code)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok {
			response.Fail(c, ec)
		} else {
			response.Fail(c, errcode.Wrap(errcode.Internal, err))
		}
		return
	}
	response.OK(c, result)
}

func getCurrentUserID(c *gin.Context) uint64 {
	if v, ok := c.Get("current_user_id"); ok {
		switch t := v.(type) {
		case uint64:
			return t
		case uint:
			return uint64(t)
		case int:
			if t > 0 {
				return uint64(t)
			}
		case int64:
			if t > 0 {
				return uint64(t)
			}
		case float64:
			if t > 0 {
				return uint64(t)
			}
		case string:
			if id, err := strconv.ParseUint(t, 10, 64); err == nil {
				return id
			}
		}
	}
	if v := c.GetHeader("X-User-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			return id
		}
	}
	return 1
}
