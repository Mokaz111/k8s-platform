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
	CtxUserID        = "user_id"
	CtxUsername      = "username"
	CtxPermTree      = "perm_tree"
	CtxPermCode      = "perm_code"
)

type routePermMapEntry struct {
	Method  string
	Prefix  string
	Perm    string
}

var defaultRoutePermMap = []routePermMapEntry{
	{"GET", "/api/v1/clusters", "cluster:list"},
	{"POST", "/api/v1/clusters", "cluster:create"},
	{"PUT", "/api/v1/clusters", "cluster:update"},
	{"DELETE", "/api/v1/clusters", "cluster:delete"},
	{"POST", "/api/v1/clusters/ping", "cluster:ping"},

	{"GET", "/api/v1/resources", "resource:list"},
	{"GET", "/api/v1/resource", "resource:get"},
	{"POST", "/api/v1/resources", "resource:create"},
	{"PUT", "/api/v1/resources", "resource:update"},
	{"PATCH", "/api/v1/resources", "resource:update"},
	{"DELETE", "/api/v1/resources", "resource:delete"},

	{"GET", "/api/v1/versions", "version:list"},
	{"POST", "/api/v1/versions/diff", "version:diff"},
	{"POST", "/api/v1/versions/rollback", "version:rollback"},

	{"GET", "/api/v1/backups", "backup:list"},
	{"POST", "/api/v1/backups", "backup:create"},
	{"POST", "/api/v1/backups/restore", "backup:restore"},
	{"DELETE", "/api/v1/backups", "backup:delete"},

	{"GET", "/api/v1/plugins", "plugin:list"},
	{"POST", "/api/v1/plugins/enable", "plugin:enable"},

	{"GET", "/api/v1/users", "user:manage"},
	{"POST", "/api/v1/users", "user:manage"},
	{"PUT", "/api/v1/users", "user:manage"},
	{"PATCH", "/api/v1/users", "user:manage"},
	{"POST", "/api/v1/users/reset-password", "user:manage"},
	{"POST", "/api/v1/users/roles", "user:manage"},
	{"DELETE", "/api/v1/users/roles", "user:manage"},

	{"GET", "/api/v1/roles", "role:manage"},
	{"POST", "/api/v1/roles", "role:manage"},
	{"PUT", "/api/v1/roles", "role:manage"},
	{"DELETE", "/api/v1/roles", "role:manage"},
	{"GET", "/api/v1/roles/permissions", "role:manage"},
	{"POST", "/api/v1/roles/permissions", "role:manage"},
	{"GET", "/api/v1/permissions", "role:manage"},

	{"GET", "/api/v1/audit-logs", "audit:list"},

	{"GET", "/api/v1/system/config", "system:config"},
	{"PUT", "/api/v1/system/config", "system:config"},
}

func resolvePermCode(method, path string) string {
	for _, e := range defaultRoutePermMap {
		if e.Method == method && strings.HasPrefix(path, e.Prefix) {
			return e.Perm
		}
	}
	return ""
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

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
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
		uid, _ := userID.(uint64)

		permCode := resolvePermCode(c.Request.Method, c.Request.URL.Path)
		if permCode == "" {
			c.Next()
			return
		}
		c.Set(CtxPermCode, permCode)

		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
		pt, err := authSvc.GetUserPermissionTree(ctx, uid)
		if err != nil {
			response.Fail(c, err)
			return
		}

		c.Set(CtxPermTree, pt)

		if !pt.HasPerm(permCode) {
			response.Fail(c, errcode.New(errcode.PermissionDenied, "缺少权限: "+permCode))
			return
		}

		if !pt.IsPlatformAdmin {
			clusterCode := c.GetHeader("X-Cluster-Code")
			if clusterCode != "" {
				allowed := false
				for _, sc := range pt.Scopes {
					switch sc.ScopeType {
					case "cluster":
						if sc.ClusterCode == clusterCode {
							allowed = true
						}
					case "namespace":
						if sc.ClusterCode == clusterCode {
							allowed = true
						}
					}
				}
				if !allowed {
					response.Fail(c, errcode.New(errcode.ScopeDenied, "无权访问集群: "+clusterCode))
					return
				}
			}
		}

		c.Next()
	}
}
