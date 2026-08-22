package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/pkg/errcode"
)

// 统一响应结构
type Body struct {
	Code    int         `json:"code"`              // 0 成功，其他参考 errcode
	Message string      `json:"message"`           // 可读文案
	Data    interface{} `json:"data,omitempty"`    // 成功负载
	Details interface{} `json:"details,omitempty"` // 错误时的详情
	TraceID string      `json:"trace_id,omitempty"`// 请求追踪 ID
}

// OK 成功返回
func OK(c *gin.Context, data ...interface{}) {
	b := Body{Code: 0, Message: "OK"}
	if len(data) > 0 {
		b.Data = data[0]
	}
	if tid, ok := c.Get("trace_id"); ok {
		b.TraceID = tid.(string)
	}
	c.JSON(http.StatusOK, b)
}

// OKList 分页列表返回统一格式
func OKList(c *gin.Context, total int64, items interface{}) {
	OK(c, gin.H{"total": total, "items": items})
}

// Fail 根据错误码返回
func Fail(c *gin.Context, err *errcode.Error) {
	b := Body{Code: int(err.Code), Message: err.Message, Details: err.Details}
	if tid, ok := c.Get("trace_id"); ok {
		b.TraceID = tid.(string)
	}
	c.AbortWithStatusJSON(err.Code.HTTPStatus(), b)
}

// FailWithStatus 自定义 HTTP 状态码
func FailWithStatus(c *gin.Context, httpStatus int, err *errcode.Error) {
	b := Body{Code: int(err.Code), Message: err.Message, Details: err.Details}
	if tid, ok := c.Get("trace_id"); ok {
		b.TraceID = tid.(string)
	}
	c.AbortWithStatusJSON(httpStatus, b)
}
