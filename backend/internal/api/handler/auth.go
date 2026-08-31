package handler

import (
	"context"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/api/middleware"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
	"gorm.io/gorm"
)

type AuthHandler struct {
	AuthSvc *auth.Service
	DB      *gorm.DB
}

func NewAuthHandler(authSvc *auth.Service, db *gorm.DB) *AuthHandler {
	return &AuthHandler{AuthSvc: authSvc, DB: db}
}

type loginReq struct {
	Username string `json:"username" binding:"required,min=1,max=64"`
	Password string `json:"password" binding:"required,min=1,max=128"`
}

func (h *AuthHandler) LoginHandler(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}

	clientIP := c.ClientIP()
	if host, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		clientIP = host
	}
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		clientIP = xff
	}

	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
	result, err := h.AuthSvc.Login(ctx, req.Username, req.Password, clientIP)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) RefreshHandler(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
	result, err := h.AuthSvc.Refresh(ctx, req.RefreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

type userRoleInfo struct {
	RoleID      uint64           `json:"role_id"`
	RoleCode    string           `json:"role_code"`
	RoleName    string           `json:"role_name"`
	ScopeType   models.ScopeType `json:"scope_type"`
	ClusterCode string           `json:"cluster_code,omitempty"`
	Namespace   string           `json:"namespace,omitempty"`
}

type currentUserResp struct {
	ID              uint64            `json:"id"`
	Username        string            `json:"username"`
	DisplayName     string            `json:"display_name"`
	Email           string            `json:"email,omitempty"`
	Phone           string            `json:"phone,omitempty"`
	AuthSource      string            `json:"auth_source"`
	Status          models.UserStatus `json:"status"`
	LastLoginAt     string            `json:"last_login_at,omitempty"`
	LastLoginIP     string            `json:"last_login_ip,omitempty"`
	Roles           []userRoleInfo    `json:"roles"`
	Perms           []string          `json:"perms"`
	IsPlatformAdmin bool              `json:"is_platform_admin"`
}

func (h *AuthHandler) CurrentUserHandler(c *gin.Context) {
	userID, ok := middleware.CurrentUserID(c)
	if !ok {
		response.Fail(c, errcode.New(errcode.Unauthenticated, "无法识别当前用户"))
		return
	}
	username, _ := middleware.CurrentUsername(c)

	var user models.SysUser
	if err := h.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}

	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
	pt, permErr := h.AuthSvc.GetUserPermissionTree(ctx, userID)
	if permErr != nil {
		response.Fail(c, permErr)
		return
	}

	type roleRow struct {
		RoleID         uint64
		RoleCode       string
		RoleName       string
		ScopeType      models.ScopeType
		ScopeCluster   string
		ScopeNamespace string
	}
	var rows []roleRow
	h.DB.Table("sys_user_role ur").
		Select("ur.role_id, r.code as role_code, r.name as role_name, ur.scope_type, ur.scope_cluster_code, ur.scope_namespace").
		Joins("LEFT JOIN sys_role r ON ur.role_id = r.id").
		Where("ur.user_id = ?", userID).
		Scan(&rows)

	roles := make([]userRoleInfo, 0, len(rows))
	for _, r := range rows {
		roles = append(roles, userRoleInfo{
			RoleID:      r.RoleID,
			RoleCode:    r.RoleCode,
			RoleName:    r.RoleName,
			ScopeType:   r.ScopeType,
			ClusterCode: r.ScopeCluster,
			Namespace:   r.ScopeNamespace,
		})
	}

	loginAt := ""
	if user.LastLoginAt != nil {
		loginAt = user.LastLoginAt.Format("2006-01-02 15:04:05")
	}

	resp := currentUserResp{
		ID:              user.ID,
		Username:        user.Username,
		DisplayName:     user.DisplayName,
		Email:           user.Email,
		Phone:           user.Phone,
		AuthSource:      user.AuthSource,
		Status:          user.Status,
		LastLoginAt:     loginAt,
		LastLoginIP:     user.LastLoginIP,
		Roles:           roles,
		Perms:           pt.PermsSlice(),
		IsPlatformAdmin: pt.IsPlatformAdmin,
	}
	_ = username
	response.OK(c, resp)
}

type logoutReq struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}

func (h *AuthHandler) LogoutHandler(c *gin.Context) {
	var req logoutReq
	_ = c.ShouldBindJSON(&req)
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))
	var access *auth.CustomClaims
	if v, ok := c.Get(middleware.CtxClaims); ok {
		access, _ = v.(*auth.CustomClaims)
	}
	h.AuthSvc.Logout(ctx, access, req.RefreshToken)
	response.OK(c, gin.H{"message": "logout success"})
}
