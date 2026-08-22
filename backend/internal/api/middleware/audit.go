package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/logger"
	"gorm.io/gorm"
)

// AuditMiddleware 记录写操作（POST/PUT/PATCH/DELETE）到 audit_log 表。
//
// 挂载顺序：AuthMiddleware → RBACMiddleware → AuditMiddleware → Handler
// （依赖 Auth 注入 user_id/username、RBAC 注入 perm_code/perm_tree）
//
// 设计要点：
//   - GET / HEAD / OPTIONS 不产生副作用，不记录
//   - 请求体由前置 AccessLog 中间件暂存到 context（audit_body），c.Next() 后取用；
//     避免重复读取，也避免截断 handler 实际使用的 body
//   - c.Next() 返回后响应码已知，据此判定 success/fail
//   - module/action 优先从 RBAC 注入的 perm_code 解析（如 cluster:create）
//     无法解析时回退到 URL 首段 + HTTP method
//   - cluster_code/namespace 优先取路径参数（:code/:namespace），其次取请求头
//   - 写库失败仅记录日志，不破坏已写出的响应
func AuditMiddleware(db *gorm.DB, log *logger.Logger, cfg *config.SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if !isWriteMethod(method) {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		cost := time.Since(start)

		// 从 AccessLog 中间件缓存的原始 body 取（避免重复读取，也避免截断 handler 用的 body）
		// handler 已消费 c.Request.Body，此处无法再读，故依赖 AccessLog 在 c.Next() 前暂存的 audit_body
		var bodyBytes []byte
		if v, ok := c.Get("audit_body"); ok {
			bodyBytes, _ = v.([]byte)
		}

		// 同步写入；失败/panic 不影响已写出的响应
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warnf("audit log panic recovered: %v", r)
				}
			}()
			writeAuditLog(db, log, cfg, c, method, bodyBytes, cost)
		}()
	}
}

func isWriteMethod(m string) bool {
	switch m {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func writeAuditLog(db *gorm.DB, log *logger.Logger, cfg *config.SecurityConfig, c *gin.Context, method string, reqBody []byte, cost time.Duration) {
	module, action := parseModuleAction(c)
	if module == "" {
		// 无法识别模块（未匹配 perm code 且无法从 URL 推断），跳过
		return
	}

	userID, _ := c.Get(CtxUserID)
	uid, _ := userID.(uint64)
	username, _ := c.Get(CtxUsername)
	uname, _ := username.(string)

	clusterCode := pickClusterCode(c)
	namespace := pickNamespace(c)
	targetType, targetID := pickTarget(c)

	status := "success"
	if c.Writer.Status() >= 400 {
		status = "fail"
	}

	// 复用 AccessLog 的脱敏+截断逻辑
	safeBody := maskAndTruncateBody(c.ContentType(), reqBody, cfg.AuditBodyMaxBytes)

	entry := &models.AuditLog{
		TraceID:       c.GetString("trace_id"),
		UserID:        uid,
		Username:      uname,
		ClientIP:      clientIP(c),
		UserAgent:     c.GetHeader("User-Agent"),
		Module:        module,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		ClusterCode:   clusterCode,
		Namespace:     namespace,
		Status:        status,
		RequestMethod: method,
		RequestURI:    c.Request.URL.RequestURI(),
		RequestBody:   models.JSONB(safeBody),
		ResponseCode:  c.Writer.Status(),
		CostMs:        int(cost.Milliseconds()),
	}

	// 用独立 context，避免请求结束后 c.Request.Context() 已取消
	if err := db.WithContext(context.Background()).Create(entry).Error; err != nil {
		log.Warnf("write audit log failed: %v (module=%s action=%s uri=%s)",
			err, module, action, entry.RequestURI)
	}
}

// parseModuleAction 优先从 RBAC 注入的 perm_code 解析 module:action
// 回退策略：URL 首段作为 module，HTTP method 小写作为 action
func parseModuleAction(c *gin.Context) (module, action string) {
	if code, ok := c.Get(CtxPermCode); ok {
		if s, ok := code.(string); ok && s != "" {
			if idx := strings.Index(s, ":"); idx > 0 {
				return s[:idx], s[idx+1:]
			}
			return s, ""
		}
	}
	m := moduleFromPath(c.Request.URL.Path)
	if m == "" {
		return "", ""
	}
	return m, strings.ToLower(c.Request.Method)
}

// moduleFromPath 从 /api/v1/<module>/... 提取首段
func moduleFromPath(path string) string {
	p := strings.TrimPrefix(path, "/api/v1/")
	p = strings.TrimPrefix(p, "/")
	if idx := strings.Index(p, "/"); idx > 0 {
		p = p[:idx]
	}
	return p
}

// pickClusterCode 优先取路径参数 :code，其次取 X-Cluster-Code 请求头
func pickClusterCode(c *gin.Context) string {
	if v := c.Param("code"); v != "" {
		return v
	}
	return c.GetHeader("X-Cluster-Code")
}

// pickNamespace 优先取路径参数 :namespace，其次取 X-Namespace 请求头
func pickNamespace(c *gin.Context) string {
	if v := c.Param("namespace"); v != "" {
		return v
	}
	return c.GetHeader("X-Namespace")
}

// pickTarget 提取操作对象类型与标识：优先 :id，其次 :name，再次 :code
func pickTarget(c *gin.Context) (targetType, targetID string) {
	targetType = moduleFromPath(c.Request.URL.Path)
	if v := c.Param("id"); v != "" {
		return targetType, v
	}
	if v := c.Param("name"); v != "" {
		return targetType, v
	}
	if v := c.Param("code"); v != "" {
		return targetType, v
	}
	return targetType, ""
}
