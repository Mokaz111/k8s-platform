package handler

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/auth"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
	"gorm.io/gorm"
)

type RoleHandler struct {
	DB      *gorm.DB
	AuthSvc *auth.Service
}

func NewRoleHandler(db *gorm.DB, authSvc *auth.Service) *RoleHandler {
	return &RoleHandler{DB: db, AuthSvc: authSvc}
}

type roleListQuery struct {
	Page     int    `form:"page,default=1"`
	PageSize int    `form:"page_size,default=50"`
	Keyword  string `form:"keyword"`
	Status   *int8  `form:"status"`
}

type roleItem struct {
	ID          uint64 `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Builtin     int8   `json:"builtin"`
	Status      int8   `json:"status"`
	PermCount   int64  `json:"perm_count"`
	UserCount   int64  `json:"user_count"`
	CreatedAt   string `json:"created_at"`
}

func (h *RoleHandler) ListRoles(c *gin.Context) {
	var q roleListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 500 {
		q.PageSize = 50
	}

	db := h.DB.Model(&models.SysRole{})
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		db = db.Where("code ILIKE ? OR name ILIKE ? OR description ILIKE ?", kw, kw, kw)
	}
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}

	var roles []models.SysRole
	offset := (q.Page - 1) * q.PageSize
	if err := db.Order("id ASC").Limit(q.PageSize).Offset(offset).Find(&roles).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}

	roleIDs := make([]uint64, 0, len(roles))
	for _, r := range roles {
		roleIDs = append(roleIDs, r.ID)
	}
	type cntRow struct {
		RoleID uint64
		Cnt    int64
	}
	var permCnts, userCnts []cntRow
	if len(roleIDs) > 0 {
		h.DB.Model(&models.SysRolePermission{}).
			Select("role_id, count(*) as cnt").
			Where("role_id IN ?", roleIDs).
			Group("role_id").Scan(&permCnts)
		h.DB.Model(&models.SysUserRole{}).
			Select("role_id, count(*) as cnt").
			Where("role_id IN ?", roleIDs).
			Group("role_id").Scan(&userCnts)
	}
	pMap := make(map[uint64]int64)
	for _, r := range permCnts {
		pMap[r.RoleID] = r.Cnt
	}
	uMap := make(map[uint64]int64)
	for _, r := range userCnts {
		uMap[r.RoleID] = r.Cnt
	}

	items := make([]roleItem, 0, len(roles))
	for _, r := range roles {
		items = append(items, roleItem{
			ID:          r.ID,
			Code:        r.Code,
			Name:        r.Name,
			Description: r.Description,
			Builtin:     r.Builtin,
			Status:      r.Status,
			PermCount:   pMap[r.ID],
			UserCount:   uMap[r.ID],
			CreatedAt:   r.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	response.OKList(c, total, items)
}

type createRoleReq struct {
	Code        string   `json:"code" binding:"required,min=2,max=64"`
	Name        string   `json:"name" binding:"required,min=1,max=128"`
	Description string   `json:"description"`
	Perms       []string `json:"perms"`
}

func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req createRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}

	var exists int64
	h.DB.Model(&models.SysRole{}).Where("code = ?", req.Code).Count(&exists)
	if exists > 0 {
		response.Fail(c, errcode.New(errcode.AlreadyExists, "角色 code 已存在"))
		return
	}

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		role := models.SysRole{
			Code:        req.Code,
			Name:        req.Name,
			Description: req.Description,
			Builtin:     0,
			Status:      1,
		}
		if err := tx.Create(&role).Error; err != nil {
			return err
		}
		if len(req.Perms) > 0 {
			var validPerms []string
			if err := tx.Model(&models.SysPermission{}).
				Where("code IN ?", req.Perms).
				Pluck("code", &validPerms).Error; err != nil {
				return err
			}
			var binds []models.SysRolePermission
			for _, code := range validPerms {
				binds = append(binds, models.SysRolePermission{RoleID: role.ID, PermissionCode: code})
			}
			if len(binds) > 0 {
				if err := tx.Create(&binds).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	response.OK(c, gin.H{"code": req.Code})
}

type updateRoleReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *int8   `json:"status"`
}

func (h *RoleHandler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的角色 ID"))
		return
	}
	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}
	var role models.SysRole
	if err := h.DB.Where("id = ?", id).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.Fail(c, errcode.New(errcode.NotFound, "角色不存在"))
			return
		}
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	if role.Builtin == 1 && req.Status != nil && *req.Status == 0 {
		response.Fail(c, errcode.New(errcode.PermissionDenied, "内置角色不可禁用"))
		return
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if len(updates) > 0 {
		if err := h.DB.Model(&role).Updates(updates).Error; err != nil {
			response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
			return
		}
		h.invalidateRoleBindUsersCache(c, id)
	}
	response.OK(c, gin.H{"id": id})
}

func (h *RoleHandler) DeleteRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的角色 ID"))
		return
	}
	var role models.SysRole
	if err := h.DB.Where("id = ?", id).First(&role).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.Fail(c, errcode.New(errcode.NotFound, "角色不存在"))
			return
		}
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	if role.Builtin == 1 {
		response.Fail(c, errcode.New(errcode.PermissionDenied, "内置角色不可删除"))
		return
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&models.SysRolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", id).Delete(&models.SysUserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&role).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	h.invalidateRoleBindUsersCache(c, id)
	response.OK(c, gin.H{"id": id})
}

type setRolePermsReq struct {
	Perms []string `json:"perms"`
}

func (h *RoleHandler) SetRolePermissions(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的角色 ID"))
		return
	}
	var req setRolePermsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, err.Error()))
		return
	}

	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&models.SysRolePermission{}).Error; err != nil {
			return err
		}
		if len(req.Perms) == 0 {
			return nil
		}
		var validPerms []string
		if err := tx.Model(&models.SysPermission{}).
			Where("code IN ?", req.Perms).
			Pluck("code", &validPerms).Error; err != nil {
			return err
		}
		var binds []models.SysRolePermission
		for _, code := range validPerms {
			binds = append(binds, models.SysRolePermission{RoleID: id, PermissionCode: code})
		}
		if len(binds) > 0 {
			if err := tx.Create(&binds).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	h.invalidateRoleBindUsersCache(c, id)
	response.OK(c, gin.H{"id": id, "perm_count": len(req.Perms)})
}

func (h *RoleHandler) GetRolePermissions(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.Fail(c, errcode.New(errcode.InvalidArgument, "无效的角色 ID"))
		return
	}
	var codes []string
	err = h.DB.Model(&models.SysRolePermission{}).
		Where("role_id = ?", id).
		Pluck("permission_code", &codes).Error
	if err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	if codes == nil {
		codes = []string{}
	}
	response.OK(c, gin.H{"role_id": id, "perms": codes})
}

type permListQuery struct {
	Module string `form:"module"`
}

func (h *RoleHandler) ListPermissions(c *gin.Context) {
	var q permListQuery
	_ = c.ShouldBindQuery(&q)
	db := h.DB.Model(&models.SysPermission{}).Order("module ASC, id ASC")
	if q.Module != "" {
		db = db.Where("module = ?", q.Module)
	}
	var perms []models.SysPermission
	if err := db.Find(&perms).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	response.OK(c, perms)
}

func (h *RoleHandler) invalidateRoleBindUsersCache(c *gin.Context, roleID uint64) {
	var userIDs []uint64
	h.DB.Model(&models.SysUserRole{}).
		Where("role_id = ?", roleID).
		Distinct().
		Pluck("user_id", &userIDs)
	ctx := context.WithValue(c.Request.Context(), "trace_id", c.GetString("trace_id"))
	for _, uid := range userIDs {
		_ = h.AuthSvc.InvalidateUserPermissionCache(ctx, uid)
	}
}
