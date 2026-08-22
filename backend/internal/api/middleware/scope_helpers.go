package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/pkg/errcode"
)

// GetPermTree 从 gin context 中取出当前用户的权限树
// 注意：返回值可能为 nil（例如路由未挂载 RBACMiddleware 时）
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

// RequireClusterScope 校验当前用户对指定集群是否有任意访问范围
// 失败时返回 ScopeDenied 错误，由 handler 通过 response.Fail 返回
func RequireClusterScope(c *gin.Context, clusterCode string) error {
	pt := GetPermTree(c)
	if pt == nil {
		// 未挂 RBACMiddleware：放行（权限由其它层兜底）
		return nil
	}
	if pt.HasClusterScope(clusterCode) {
		return nil
	}
	return errcode.New(errcode.ScopeDenied, "无权访问集群: "+clusterCode)
}

// RequireNamespaceScope 校验当前用户对指定集群的指定命名空间是否有访问权
// 命名空间为空（集群级资源）时退化为 cluster scope 校验
func RequireNamespaceScope(c *gin.Context, clusterCode, namespace string) error {
	pt := GetPermTree(c)
	if pt == nil {
		return nil
	}
	if namespace == "" {
		return RequireClusterScope(c, clusterCode)
	}
	if pt.HasNamespaceScope(clusterCode, namespace) {
		return nil
	}
	return errcode.New(errcode.ScopeDenied, "无权访问命名空间: "+clusterCode+"/"+namespace)
}

// AllowedNamespaces 从 context 中取出当前用户对指定集群可访问的命名空间
// 返回 (namespaces, isFullCluster)：
//   - isFullCluster=true 表示有全集群范围（cluster 级或 platform 级），namespaces 此时为 nil
//   - isFullCluster=false 表示仅命名空间级范围，namespaces 为具体允许列表
//
// 该函数供 list 类 handler 在 SQL 或内存过滤时使用
func AllowedNamespaces(c *gin.Context, clusterCode string) ([]string, bool) {
	pt := GetPermTree(c)
	if pt == nil {
		// 未挂 RBAC：保守放行全部
		return nil, true
	}
	return pt.AllowedNamespaces(clusterCode)
}

// AllowedClusters 从 context 中取出当前用户可访问的所有集群 code 列表
// 返回 (clusterCodes, isFullPlatform)：
//   - isFullPlatform=true 表示平台级范围，可访问全部集群（clusterCodes 为 nil）
//   - isFullPlatform=false 表示仅部分集群范围，clusterCodes 为具体允许列表（可能为空）
//
// 用于跨集群列表场景（如顶层 /backups 不带 cluster_code 时按集群维度过滤）
func AllowedClusters(c *gin.Context) ([]string, bool) {
	pt := GetPermTree(c)
	if pt == nil {
		// 未挂 RBAC：保守放行全部
		return nil, true
	}
	return pt.AllowedClusters()
}
