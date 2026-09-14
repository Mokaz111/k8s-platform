package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/helm"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

func failHelm(c *gin.Context, err error, fallback string) {
	if ec, ok := err.(*errcode.Error); ok {
		response.Fail(c, ec)
		return
	}
	response.Fail(c, errcode.Wrap(errcode.Internal, err, fallback))
}

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
		failHelm(c, err, "Helm install/upgrade 失败")
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
	ok, versionOrErr := h.Mgr.CheckCLI(c.Request.Context())
	if !ok {
		response.OK(c, gin.H{"available": false, "error": versionOrErr})
		return
	}
	response.OK(c, gin.H{"available": true, "version": versionOrErr})
}

func (h *HelmHandler) ListRepos(c *gin.Context) {
	repos, err := h.Mgr.ListRepos(c.Request.Context())
	if err != nil {
		failHelm(c, err, "查询 Helm 仓库失败")
		return
	}
	response.OK(c, gin.H{"items": repos, "total": len(repos)})
}

func (h *HelmHandler) AddRepo(c *gin.Context) {
	var req helm.AddRepoInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.InvalidArgument, err, "请求体解析失败"))
		return
	}
	if err := h.Mgr.AddRepo(c.Request.Context(), req); err != nil {
		failHelm(c, err, "添加 Helm 仓库失败")
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (h *HelmHandler) RemoveRepo(c *gin.Context) {
	name := c.Param("name")
	if err := h.Mgr.RemoveRepo(c.Request.Context(), name); err != nil {
		failHelm(c, err, "删除 Helm 仓库失败")
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (h *HelmHandler) UpdateRepos(c *gin.Context) {
	name := strings.TrimSpace(c.Query("name"))
	if err := h.Mgr.UpdateRepos(c.Request.Context(), name); err != nil {
		failHelm(c, err, "更新 Helm 仓库索引失败")
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (h *HelmHandler) SearchCharts(c *gin.Context) {
	repo := strings.TrimSpace(c.Query("repo"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	charts, err := h.Mgr.SearchCharts(c.Request.Context(), repo, keyword)
	if err != nil {
		failHelm(c, err, "搜索 Chart 失败")
		return
	}
	response.OK(c, gin.H{"items": charts, "total": len(charts)})
}
