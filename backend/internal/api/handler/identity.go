package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
)

// mustCurrentUser 从 JWT context 读取当前用户，失败时已写入 401 响应。
func mustCurrentUser(c *gin.Context) (userID uint64, username string, ok bool) {
	userID, ok = middleware.CurrentUserID(c)
	if !ok {
		response.Fail(c, errcode.New(errcode.Unauthenticated, "无法识别当前用户"))
		return 0, "", false
	}
	username, _ = middleware.CurrentUsername(c)
	return userID, username, true
}
