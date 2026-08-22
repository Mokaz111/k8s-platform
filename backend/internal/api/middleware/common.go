package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/k8s-platform/console/pkg/response"
)

// CORS 跨域中间件
func CORS(cfg *config.SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := false
		for _, o := range cfg.CORSAllowOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}
		if allowed && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
			if cfg.CORSAllowCredentials {
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,PATCH,OPTIONS,HEAD")
			c.Writer.Header().Set("Access-Control-Allow-Headers",
				"Authorization,Content-Type,X-Requested-With,X-Trace-ID,X-Cluster-Code,Accept,Origin")
			c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Trace-ID,Content-Disposition")
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// RequestID 生成并注入 trace_id
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader("X-Trace-ID")
		if tid == "" {
			tid = strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		c.Set("trace_id", tid)
		c.Header("X-Trace-ID", tid)
		c.Next()
	}
}

// AccessLog 结构化访问日志 + 审计日志收集（请求体脱敏后存 context，Handler 返回后写审计表）
func AccessLog(log *logger.Logger, cfg *config.SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 读取请求体（需要重新写回去，否则后面 Handler 读不到）
		var reqBody []byte
		if c.Request.Body != nil {
			reqBody, _ = io.ReadAll(c.Request.Body)
			_ = c.Request.Body.Close()
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		log = log.With("trace_id", c.GetString("trace_id"))
		c.Set("logger", log)

		// 响应 Body 包装（记录状态码）
		blw := &bodyLogWriter{body: bytes.NewBuffer(nil), ResponseWriter: c.Writer}
		c.Writer = blw

		c.Next()

		cost := time.Since(start)
		status := c.Writer.Status()

		// 脱敏请求体
		safeBody := maskAndTruncateBody(c.ContentType(), reqBody, cfg.AuditBodyMaxBytes)

		// 收集审计上下文（后面 Audit 中间件或 Handler 写 DB 用）
		c.Set("audit_ctx", map[string]interface{}{
			"method":       c.Request.Method,
			"uri":          path + "?" + query,
			"request_body": safeBody,
			"status_code":  status,
			"cost_ms":      cost.Milliseconds(),
			"client_ip":    clientIP(c),
			"user_agent":   c.GetHeader("User-Agent"),
		})

		// 结构化日志
		fields := []interface{}{
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"cost_ms", cost.Milliseconds(),
			"client_ip", clientIP(c),
			"user_agent", c.GetHeader("User-Agent"),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, "errors", c.Errors.Errors())
		}
		if status >= 500 {
			log.Errorw("HTTP "+c.Request.Method+" "+path, fields...)
		} else if status >= 400 {
			log.Warnw("HTTP "+c.Request.Method+" "+path, fields...)
		} else {
			log.Infow("HTTP "+c.Request.Method+" "+path, fields...)
		}
	}
}

// Recovery 统一 panic 恢复
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.With("trace_id", c.GetString("trace_id")).
					Errorf("panic recovered: %v", r)
				response.Fail(c, errcode.New(errcode.Internal, "服务内部异常，请联系管理员"))
			}
		}()
		c.Next()
	}
}

// ---------- 辅助 ----------

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func clientIP(c *gin.Context) string {
	if ip := c.GetHeader("X-Forwarded-For"); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return ip
	}
	return c.ClientIP()
}

// maskAndTruncateBody: 截断 + 脱敏敏感字段
func maskAndTruncateBody(contentType string, body []byte, maxBytes int) json.RawMessage {
	if len(body) == 0 {
		return nil
	}
	// 只处理 JSON
	if !strings.Contains(contentType, "application/json") {
		if len(body) > maxBytes {
			body = body[:maxBytes]
		}
		return json.RawMessage(`"(non-json body truncated)"`)
	}

	var obj interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return json.RawMessage(`"(invalid json)"`)
	}
	maskSensitive(obj)

	out, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(`"(marshal error)"`)
	}
	if len(out) > maxBytes {
		out = out[:maxBytes]
		out = append(out, []byte(`...(truncated)`)...)
	}
	return json.RawMessage(out)
}

var sensitiveKeys = map[string]struct{}{
	"password": {}, "token": {}, "secret": {}, "key": {},
	"credential": {}, "bearer": {}, "kubeconfig": {},
	"client_cert": {}, "client_key": {}, "access_key": {}, "private_key": {},
}

func maskSensitive(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			lk := strings.ToLower(k)
			if _, hit := sensitiveKeys[lk]; hit {
				t[k] = "***"
				continue
			}
			// certificate key BEGIN...END 块（YAML 转 JSON 场景）
			if s, ok := val.(string); ok && strings.Contains(s, "-----BEGIN") {
				t[k] = "***"
				continue
			}
			maskSensitive(val)
		}
	case []interface{}:
		for i := range t {
			maskSensitive(t[i])
		}
	}
}
