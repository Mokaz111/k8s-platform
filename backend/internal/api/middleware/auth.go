package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

const (
	CtxUserID   = "user_id"
	CtxUsername = "username"
	CtxPermTree = "perm_tree"
	CtxPermCode = "perm_code"
	CtxClaims   = "jwt_claims"
	// PermKey 由 RequirePermission 写入；Gin 中路由级 handler 晚于组中间件，
	// RBACMiddleware 以 FullPath 精确表为准，RequirePermission 做二次校验。
	PermKey = "required_permission"
)

// routePermExact 按 "METHOD + 空格 + Gin FullPath" 精确匹配。
// 禁止用请求路径前缀：/api/v1/clusters/:code/resources 会误命中 /api/v1/clusters。
var routePermExact = map[string]string{
	"GET /api/v1/clusters":             "cluster:list",
	"POST /api/v1/clusters":            "cluster:create",
	"GET /api/v1/clusters/:code":       "cluster:list",
	"PUT /api/v1/clusters/:code":       "cluster:update",
	"DELETE /api/v1/clusters/:code":    "cluster:delete",
	"POST /api/v1/clusters/:code/ping": "cluster:ping",

	"GET /api/v1/clusters/:code/resources/:apiVersion/:kind":                     "resource:list",
	"GET /api/v1/clusters/:code/resources/:apiVersion/:kind/:namespace/:name":    "resource:get",
	"POST /api/v1/clusters/:code/resources/:apiVersion/:kind":                    "resource:create",
	"PUT /api/v1/clusters/:code/resources/:apiVersion/:kind/:namespace/:name":    "resource:update",
	"DELETE /api/v1/clusters/:code/resources/:apiVersion/:kind/:namespace/:name": "resource:delete",

	"GET /api/v1/versions":           "version:list",
	"GET /api/v1/versions/diff":      "version:diff",
	"POST /api/v1/versions/diff":     "version:diff",
	"POST /api/v1/versions/rollback": "version:rollback",

	"POST /api/v1/clusters/:code/backups": "backup:create",
	"GET /api/v1/backups":                 "backup:list",
	"GET /api/v1/backups/:id":             "backup:list",
	"GET /api/v1/backups/:id/download":    "backup:list",
	"POST /api/v1/backups/:id/restore":    "backup:restore",
	"DELETE /api/v1/backups/:id":          "backup:delete",

	"GET /api/v1/clusters/:code/helm/releases":                            "helm:view",
	"POST /api/v1/clusters/:code/helm/releases":                           "helm:install",
	"DELETE /api/v1/clusters/:code/helm/releases/:namespace/:name":        "helm:uninstall",
	"POST /api/v1/clusters/:code/helm/releases/:namespace/:name/rollback": "helm:rollback",
	"GET /api/v1/clusters/:code/helm/releases/:namespace/:name/history":   "helm:view",
	"GET /api/v1/clusters/:code/helm/check":                               "helm:view",

	"GET /api/v1/helm/repos":          "helm:view",
	"POST /api/v1/helm/repos":         "helm:install",
	"DELETE /api/v1/helm/repos/:name": "helm:install",
	"POST /api/v1/helm/repos/update":  "helm:install",
	"GET /api/v1/helm/charts":         "helm:view",

	"GET /api/v1/clusters/:code/pods/:namespace/:pod/logs": "cluster:list",

	"GET /api/v1/users":                     "user:manage",
	"POST /api/v1/users":                    "user:manage",
	"PUT /api/v1/users/:id":                 "user:manage",
	"PATCH /api/v1/users/:id/status":        "user:manage",
	"POST /api/v1/users/:id/reset-password": "user:manage",
	"GET /api/v1/users/:id/roles":           "user:manage",
	"POST /api/v1/users/:id/roles":          "user:manage",
	"DELETE /api/v1/users/:id/roles":        "user:manage",

	"GET /api/v1/roles":                  "role:manage",
	"POST /api/v1/roles":                 "role:manage",
	"PUT /api/v1/roles/:id":              "role:manage",
	"DELETE /api/v1/roles/:id":           "role:manage",
	"GET /api/v1/roles/:id/permissions":  "role:manage",
	"POST /api/v1/roles/:id/permissions": "role:manage",
	"GET /api/v1/permissions":            "role:manage",

	"GET /api/v1/audit-logs": "audit:list",
}

func resolvePermCode(method, fullPath string) string {
	if fullPath == "" {
		return ""
	}
	return routePermExact[method+" "+fullPath]
}

// RequirePermission 声明并二次校验权限点。组中间件先按 FullPath 表校验；
// 此处防止漏登记路由在 handler 执行前再拦一层。
func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(PermKey, code)
		if err := enforcePerm(c, code); err != nil {
			response.Fail(c, err)
			return
		}
		c.Next()
	}
}

func enforcePerm(c *gin.Context, permCode string) *errcode.Error {
	if permCode == "" {
		return errcode.New(errcode.PermissionDenied, "未声明访问权限")
	}
	pt := GetPermTree(c)
	if pt == nil {
		return errcode.New(errcode.PermissionDenied, "权限未加载")
	}
	if !pt.HasPerm(permCode) {
		return errcode.New(errcode.PermissionDenied, "缺少权限: "+permCode)
	}
	return nil
}

func AuthMiddleware(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := authSvc.JWT.ExtractToken(c)
		if err != nil {
			response.Fail(c, errcode.New(errcode.Unauthenticated, "缺少有效的 Authorization Header"))
			return
		}
		claims, parseErr := authSvc.JWT.ParseToken(tokenStr)
		if parseErr != nil {
			switch {
			case strings.Contains(parseErr.Error(), jwt.ErrTokenExpired.Error()):
				response.Fail(c, errcode.New(errcode.TokenExpired))
			default:
				response.Fail(c, errcode.New(errcode.Unauthenticated, "Token 解析失败或无效"))
			}
			return
		}
		if claims.Type != auth.AccessToken {
			response.Fail(c, errcode.New(errcode.Unauthenticated, "Token 类型错误，请使用 Access Token"))
			return
		}
		if authSvc.IsRevoked(c.Request.Context(), claims) {
			response.Fail(c, errcode.New(errcode.Unauthenticated, "Token 已失效"))
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxClaims, claims)
		c.Next()
	}
}

func RBACMiddleware(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get(CtxUserID)
		if !ok {
			response.Fail(c, errcode.New(errcode.Unauthenticated))
			return
		}
		uid, ok := userID.(uint64)
		if !ok || uid == 0 {
			response.Fail(c, errcode.New(errcode.Unauthenticated))
			return
		}

		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
		pt, err := authSvc.GetUserPermissionTree(ctx, uid)
		if err != nil {
			response.Fail(c, err)
			return
		}
		c.Set(CtxPermTree, pt)

		permCode := resolvePermCode(c.Request.Method, c.FullPath())
		if permCode == "" {
			response.Fail(c, errcode.New(errcode.PermissionDenied, "未授权的接口"))
			return
		}
		c.Set(CtxPermCode, permCode)
		if !pt.HasPerm(permCode) {
			response.Fail(c, errcode.New(errcode.PermissionDenied, "缺少权限: "+permCode))
			return
		}

		clusterCode, mismatch := requestClusterCode(c)
		if mismatch {
			response.Fail(c, errcode.New(errcode.ScopeDenied, "X-Cluster-Code 与路径不一致"))
			return
		}
		if clusterCode != "" {
			if err := RequireAnyClusterAccess(c, clusterCode); err != nil {
				if ec, ok := err.(*errcode.Error); ok {
					response.Fail(c, ec)
					return
				}
				response.Fail(c, errcode.New(errcode.ScopeDenied, err.Error()))
				return
			}
		}

		c.Next()
	}
}

// requestClusterCode 优先路径参数 :code（临时 ping 的 "_" 不参与 scope），
// 否则回落 X-Cluster-Code。两者都有且不一致时拒绝。
func requestClusterCode(c *gin.Context) (code string, mismatch bool) {
	fromPath := strings.TrimSpace(c.Param("code"))
	fromHeader := strings.TrimSpace(c.GetHeader("X-Cluster-Code"))
	if fromPath == "_" {
		fromPath = ""
	}
	if fromPath != "" && fromHeader != "" && fromPath != fromHeader {
		return fromPath, true
	}
	if fromPath != "" {
		return fromPath, false
	}
	return fromHeader, false
}

// CurrentUserID 只信 Auth 中间件写入的 JWT claims，不接受客户端 Header。
func CurrentUserID(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint64)
	return id, ok && id > 0
}

// CurrentUsername 只信 JWT claims。
func CurrentUsername(c *gin.Context) (string, bool) {
	v, ok := c.Get(CtxUsername)
	if !ok {
		return "", false
	}
	name, ok := v.(string)
	return name, ok && name != ""
}
