package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
	"gorm.io/gorm"
)

type UserHandler struct {
	DB      *gorm.DB
	AuthSvc *auth.Service
}

func NewUserHandler(db *gorm.DB, authSvc *auth.Service) *UserHandler {
	return &UserHandler{DB: db, AuthSvc: authSvc}
}

type userListQuery struct {
	Page      int               `form:"page,default=1"`
	PageSize  int               `form:"page_size,default=20"`
	Keyword   string            `form:"keyword"`
	Status    *models.UserStatus `form:"status"`
}

type userItem struct {
	ID           uint64            `json:"id"`
	Username     string            `json:"username"`
	DisplayName  string            `json:"display_name"`
	Email        string            `json:"email,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	AuthSource   string            `json:"auth_source"`
	Status       models.UserStatus `json:"status"`
	LastLoginAt  *time.Time        `json:"last_login_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	RoleCount    int64             `json:"role_count"`
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	var q userListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 20
	}

	db := h.DB.Model(&models.SysUser{})
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		db = db.Where("username ILIKE ? OR display_name ILIKE ? OR email ILIKE ?", kw, kw, kw)
	}
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}

	var users []models.SysUser
	offset := (q.Page - 1) * q.PageSize
	if err := db.Order("id DESC").Limit(q.PageSize).Offset(offset).Find(&users).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}

	userIDs := make([]uint64, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}
	type cntRow struct {
		UserID uint64
		Cnt    int64
	}
	var cnts []cntRow
	if len(userIDs) > 0 {
		h.DB.Model(&models.SysUserRole{}).
			Select("user_id, count(*) as cnt").
			Where("user_id IN ?", userIDs).
			Group("user_id").
			Scan(&cnts)
	}
	cntMap := make(map[uint64]int64, len(cnts))
	for _, r := range cnts {
		cntMap[r.UserID] = r.Cnt
	}

	items := make([]userItem, 0, len(users))
	for _, u := range users {
		items = append(items, userItem{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Email:       u.Email,
			Phone:       u.Phone,
			AuthSource:  u.AuthSource,
			Status:      u.Status,
			LastLoginAt: u.LastLoginAt,
			CreatedAt:   u.CreatedAt,
			RoleCount:   cntMap[u.ID],
		})
	}
	response.OKList(c, total, items)
}

type createUserReq struct {
	Username    string            `json:"username" binding:"required,min=2,max=64"`
	DisplayName string            `json:"display_name" binding:"max=128"`
	Email       string            `json:"email" binding:"max=128"`
	Phone       string            `json:"phone" binding:"max=32"`
	Password    string            `json:"password" binding:"required,min=6,max=64"`
	AuthSource  string            `json:"auth_source"`
	Status      models.UserStatus `json:"status"`
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	if req.AuthSource == "" {
		req.AuthSource = "local"
	}
	if req.Status == 0 {
		req.Status = models.UserStatusEnabled
	}

	var exists int64
	h.DB.Model(&models.SysUser{}).Where("username = ?", req.Username).Count(&exists)
	if exists > 0 {
		response.Fail(c, errcode.New(errcode.AlreadyExists, "用户名已存在"))
		return
	}

	user := &models.SysUser{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Phone:       req.Phone,
		AuthSource:  req.AuthSource,
		Status:      req.Status,
	}
	if err := user.SetPassword(req.Password, h.AuthSvc.Config.BcryptCost); err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err))
		return
	}
	if err := h.DB.Create(user).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	response.OK(c, gin.H{"id": user.ID, "username": user.Username})
}

type updateUserReq struct {
	DisplayName *string            `json:"display_name"`
	Email       *string            `json:"email"`
	Phone       *string            `json:"phone"`
	Status      *models.UserStatus `json:"status"`
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的用户 ID"))
		return
	}
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	var user models.SysUser
	if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.Fail(c, errcode.New(errcode.NotFound, "用户不存在"))
			return
		}
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	updates := make(map[string]interface{})
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if len(updates) > 0 {
		if err := h.DB.Model(&user).Updates(updates).Error; err != nil {
			response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
			return
		}
		ctx := context.WithValue(c.Request.Context(), "trace_id", c.GetString("trace_id"))
		_ = h.AuthSvc.InvalidateUserPermissionCache(ctx, id)
	}
	response.OK(c, gin.H{"id": id})
}

type updateUserStatusReq struct {
	Status models.UserStatus `json:"status" binding:"required"`
}

func (h *UserHandler) UpdateUserStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的用户 ID"))
		return
	}
	var req updateUserStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	res := h.DB.Model(&models.SysUser{}).Where("id = ?", id).Update("status", req.Status)
	if res.Error != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, res.Error))
		return
	}
	if res.RowsAffected == 0 {
		response.Fail(c, errcode.New(errcode.NotFound, "用户不存在"))
		return
	}
	ctx := context.WithValue(c.Request.Context(), "trace_id", c.GetString("trace_id"))
	_ = h.AuthSvc.InvalidateUserPermissionCache(ctx, id)
	response.OK(c, gin.H{"id": id, "status": req.Status})
}

type resetPasswordReq struct {
	Password string `json:"password" binding:"required,min=6,max=64"`
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的用户 ID"))
		return
	}
	var req resetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	var user models.SysUser
	if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.Fail(c, errcode.New(errcode.NotFound, "用户不存在"))
			return
		}
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	if err := user.SetPassword(req.Password, h.AuthSvc.Config.BcryptCost); err != nil {
		response.Fail(c, errcode.Wrap(errcode.Internal, err))
		return
	}
	if err := h.DB.Model(&user).Update("password_hash", user.PasswordHash).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	response.OK(c, gin.H{"id": id})
}

type bindUserRoleReq struct {
	RoleID       uint64            `json:"role_id" binding:"required"`
	ScopeType    models.ScopeType  `json:"scope_type" binding:"required"`
	ClusterCode  string            `json:"cluster_code,omitempty"`
	Namespace    string            `json:"namespace,omitempty"`
}

func (h *UserHandler) BindUserRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的用户 ID"))
		return
	}
	var req bindUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	switch req.ScopeType {
	case models.ScopeCluster:
		if req.ClusterCode == "" {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "集群范围必须指定 cluster_code"))
			return
		}
	case models.ScopeNamespace:
		if req.ClusterCode == "" || req.Namespace == "" {
			response.Fail(c, errcode.New(errcode.InvalidArgument, "命名空间范围必须指定 cluster_code 和 namespace"))
			return
		}
	}

	var userCnt int64
	h.DB.Model(&models.SysUser{}).Where("id=?", id).Count(&userCnt)
	if userCnt == 0 {
		response.Fail(c, errcode.New(errcode.NotFound, "用户不存在"))
		return
	}
	var roleCnt int64
	h.DB.Model(&models.SysRole{}).Where("id=?", req.RoleID).Count(&roleCnt)
	if roleCnt == 0 {
		response.Fail(c, errcode.New(errcode.NotFound, "角色不存在"))
		return
	}

	bind := models.SysUserRole{
		UserID:           id,
		RoleID:           req.RoleID,
		ScopeType:        req.ScopeType,
		ScopeClusterCode: req.ClusterCode,
		ScopeNamespace:   req.Namespace,
	}
	if err := h.DB.Create(&bind).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err, "绑定关系已存在或创建失败"))
		return
	}
	ctx := context.WithValue(c.Request.Context(), "trace_id", c.GetString("trace_id"))
	_ = h.AuthSvc.InvalidateUserPermissionCache(ctx, id)
	response.OK(c, gin.H{"id": bind.ID})
}

type unbindUserRoleReq struct {
	RoleID       uint64            `json:"role_id" binding:"required"`
	ScopeType    models.ScopeType  `json:"scope_type" binding:"required"`
	ClusterCode  string            `json:"cluster_code,omitempty"`
	Namespace    string            `json:"namespace,omitempty"`
}

func (h *UserHandler) UnbindUserRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的用户 ID"))
		return
	}
	var req unbindUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	q := h.DB.Where("user_id=? AND role_id=? AND scope_type=?", id, req.RoleID, req.ScopeType)
	switch req.ScopeType {
	case models.ScopeCluster:
		q = q.Where("scope_cluster_code=?", req.ClusterCode)
	case models.ScopeNamespace:
		q = q.Where("scope_cluster_code=? AND scope_namespace=?", req.ClusterCode, req.Namespace)
	}
	res := q.Delete(&models.SysUserRole{})
	if res.Error != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, res.Error))
		return
	}
	ctx := context.WithValue(c.Request.Context(), "trace_id", c.GetString("trace_id"))
	_ = h.AuthSvc.InvalidateUserPermissionCache(ctx, id)
	response.OK(c, gin.H{"deleted": res.RowsAffected})
}

type listUserRolesItem struct {
	ID          uint64            `json:"id"`
	UserID      uint64            `json:"user_id"`
	RoleID      uint64            `json:"role_id"`
	RoleCode    string            `json:"role_code"`
	RoleName    string            `json:"role_name"`
	ScopeType   models.ScopeType  `json:"scope_type"`
	ClusterCode string            `json:"cluster_code,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

func (h *UserHandler) ListUserRoles(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的用户 ID"))
		return
	}
	var rows []listUserRolesItem
	err = h.DB.Table("sys_user_role ur").
		Select("ur.id, ur.user_id, ur.role_id, r.code as role_code, r.name as role_name, ur.scope_type, ur.scope_cluster_code as cluster_code, ur.scope_namespace as namespace, ur.created_at").
		Joins("LEFT JOIN sys_role r ON ur.role_id = r.id").
		Where("ur.user_id = ?", id).
		Order("ur.id DESC").
		Scan(&rows).Error
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	response.OK(c, rows)
}
