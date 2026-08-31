package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/pkg/errcode"
)

// GetPermTree 从 gin context 中取出当前用户的权限树。
// 未挂 RBAC 或尚未注入时返回 nil，调用方必须按拒绝处理。
func GetPermTree(c *gin.Context) *auth.PermissionTree {
	v, ok := c.Get(CtxPermTree)
	if !ok || v == nil {
		return nil
	}
	if pt, ok := v.(*auth.PermissionTree); ok {
		return pt
	}
	return nil
}

func RequireClusterScope(c *gin.Context, clusterCode string) error {
	pt := GetPermTree(c)
	if pt == nil {
		return errcode.New(errcode.ScopeDenied, "权限未加载，拒绝访问集群")
	}
	if pt.HasClusterScope(clusterCode) {
		return nil
	}
	return errcode.New(errcode.ScopeDenied, "无权以集群范围访问: "+clusterCode)
}

// RequireAnyClusterAccess 允许 cluster 或 namespace 级授权进入该集群的路由。
func RequireAnyClusterAccess(c *gin.Context, clusterCode string) error {
	pt := GetPermTree(c)
	if pt == nil {
		return errcode.New(errcode.ScopeDenied, "权限未加载，拒绝访问集群")
	}
	if pt.HasAnyAccessToCluster(clusterCode) {
		return nil
	}
	return errcode.New(errcode.ScopeDenied, "无权访问集群: "+clusterCode)
}

func RequireNamespaceScope(c *gin.Context, clusterCode, namespace string) error {
	pt := GetPermTree(c)
	if pt == nil {
		return errcode.New(errcode.ScopeDenied, "权限未加载，拒绝访问命名空间")
	}
	if namespace == "" {
		return RequireClusterScope(c, clusterCode)
	}
	if pt.HasNamespaceScope(clusterCode, namespace) {
		return nil
	}
	return errcode.New(errcode.ScopeDenied, "无权访问命名空间: "+clusterCode+"/"+namespace)
}

func AllowedNamespaces(c *gin.Context, clusterCode string) ([]string, bool) {
	pt := GetPermTree(c)
	if pt == nil {
		return nil, false
	}
	return pt.AllowedNamespaces(clusterCode)
}

func AllowedClusters(c *gin.Context) ([]string, bool) {
	pt := GetPermTree(c)
	if pt == nil {
		return nil, false
	}
	return pt.AllowedClusters()
}
