package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/response"
	"gorm.io/gorm"
)

type AuditHandler struct {
	DB *gorm.DB
}

func NewAuditHandler(db *gorm.DB) *AuditHandler {
	return &AuditHandler{DB: db}
}

type auditListQuery struct {
	Page        int    `form:"page,default=1"`
	PageSize    int    `form:"page_size,default=20"`
	Username    string `form:"username"`
	Module      string `form:"module"`
	Action      string `form:"action"`
	Status      string `form:"status"`
	ClusterCode string `form:"cluster_code"`
	Namespace   string `form:"namespace"`
	TargetType  string `form:"target_type"`
	TargetID    string `form:"target_id"`
	TraceID     string `form:"trace_id"`
	StartTime   string `form:"start_time"`
	EndTime     string `form:"end_time"`
}

func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	var q auditListQuery
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

	db := h.DB.Model(&models.AuditLog{})
	if q.Username != "" {
		db = db.Where("username = ?", q.Username)
	}
	if q.Module != "" {
		db = db.Where("module = ?", q.Module)
	}
	if q.Action != "" {
		db = db.Where("action = ?", q.Action)
	}
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}
	if q.ClusterCode != "" {
		db = db.Where("cluster_code = ?", q.ClusterCode)
	}
	if q.Namespace != "" {
		db = db.Where("namespace = ?", q.Namespace)
	}
	if q.TargetType != "" {
		db = db.Where("target_type = ?", q.TargetType)
	}
	if q.TargetID != "" {
		db = db.Where("target_id = ?", q.TargetID)
	}
	if q.TraceID != "" {
		db = db.Where("trace_id = ?", q.TraceID)
	}
	if q.StartTime != "" {
		db = db.Where("created_at >= ?", q.StartTime)
	}
	if q.EndTime != "" {
		db = db.Where("created_at <= ?", q.EndTime)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}

	var logs []models.AuditLog
	offset := (q.Page - 1) * q.PageSize
	if err := db.Order("id DESC").Limit(q.PageSize).Offset(offset).Find(&logs).Error; err != nil {
		response.Fail(c, errcode.Wrap(errcode.DatabaseError, err))
		return
	}
	if logs == nil {
		logs = []models.AuditLog{}
	}
	response.OKList(c, total, logs)
}
