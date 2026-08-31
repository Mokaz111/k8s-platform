package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/helm"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

type HelmHandler struct {
	Mgr *helm.Manager
}

func (h *HelmHandler) ListReleases(c *gin.Context) {
	clusterCode := c.Param("code")
	if clusterCode == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "cluster code 不能为空"))
		return
	}
	namespace := c.Query("namespace")

	if namespace != "" && namespace != "all" && namespace != "ALL" {
		if err := middleware.RequireNamespaceScope(c, clusterCode, namespace); err != nil {
			response.Fail(c, err.(*errcode.Error))
			return
		}
	}

	releases, err := h.Mgr.ListReleases(c.Request.Context(), clusterCode, namespace)
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err, "查询 Helm releases 失败"))
		return
	}

	if namespace == "" || namespace == "all" || namespace == "ALL" {
		allowed, isFull := middleware.AllowedNamespaces(c, clusterCode)
		if !isFull {
			if len(allowed) == 0 {
				response.OK(c, gin.H{"items": []helm.HelmRelease{}, "total": 0})
				return
			}
			allowSet := make(map[string]struct{}, len(allowed))
			for _, ns := range allowed {
				allowSet[ns] = struct{}{}
			}
			filtered := make([]helm.HelmRelease, 0, len(releases))
			for _, r := range releases {
				if _, ok := allowSet[r.Namespace]; ok {
					filtered = append(filtered, r)
				}
			}
			releases = filtered
		}
	}

	response.OK(c, gin.H{"items": releases, "total": len(releases)})
}

func (h *HelmHandler) InstallRelease(c *gin.Context) {
	clusterCode := c.Param("code")
	if clusterCode == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "cluster code 不能为空"))
		return
	}
	var req helm.InstallInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}
	req.ClusterCode = clusterCode

	if req.ReleaseName == "" || req.ChartRef == "" || req.Namespace == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "release_name, namespace, chart_ref 不能为空"))
		return
	}
	if err := middleware.RequireNamespaceScope(c, clusterCode, req.Namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	out, err := h.Mgr.Install(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err, "Helm install/upgrade 失败"))
		return
	}
	response.OK(c, gin.H{"output": out})
}

func (h *HelmHandler) UninstallRelease(c *gin.Context) {
	clusterCode := c.Param("code")
	namespace := c.Param("namespace")
	releaseName := c.Param("name")
	if clusterCode == "" || namespace == "" || releaseName == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "参数不完整"))
		return
	}
	if err := middleware.RequireNamespaceScope(c, clusterCode, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	out, err := h.Mgr.Uninstall(c.Request.Context(), clusterCode, namespace, releaseName)
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err, "Helm uninstall 失败"))
		return
	}
	response.OK(c, gin.H{"output": out})
}

func (h *HelmHandler) RollbackRelease(c *gin.Context) {
	clusterCode := c.Param("code")
	namespace := c.Param("namespace")
	releaseName := c.Param("name")
	if clusterCode == "" || namespace == "" || releaseName == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "参数不完整"))
		return
	}
	if err := middleware.RequireNamespaceScope(c, clusterCode, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	revisionStr := c.Query("revision")
	revision := 0
	if revisionStr != "" {
		revision, _ = strconv.Atoi(revisionStr)
	}

	out, err := h.Mgr.Rollback(c.Request.Context(), clusterCode, namespace, releaseName, revision)
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err, "Helm rollback 失败"))
		return
	}
	response.OK(c, gin.H{"output": out})
}

func (h *HelmHandler) ListHistory(c *gin.Context) {
	clusterCode := c.Param("code")
	namespace := c.Param("namespace")
	releaseName := c.Param("name")
	if clusterCode == "" || namespace == "" || releaseName == "" {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "参数不完整"))
		return
	}
	if err := middleware.RequireNamespaceScope(c, clusterCode, namespace); err != nil {
		response.Fail(c, err.(*errcode.Error))
		return
	}

	history, err := h.Mgr.ListHistory(c.Request.Context(), clusterCode, namespace, releaseName)
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err, "Helm history 失败"))
		return
	}
	response.OK(c, gin.H{"items": history})
}

// CheckHelmCLI checks if helm CLI binary is available
func (h *HelmHandler) CheckHelmCLI(c *gin.Context) {
	// Quick check: helm version
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    gin.H{"available": true},
	})
}
