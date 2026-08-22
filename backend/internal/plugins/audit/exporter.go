package audit

import (
	"context"
	"io"
	"time"
)

// MVP_TODO: IAuditExporter 审计导出插件接口（设计文档 Section 8.4）
// 用于将审计日志导出至外部系统：ELK、Splunk、Graylog、OSS、Kafka、Syslog 等
type IAuditExporter interface {
	ExporterType() string

	Name() string

	Init(ctx context.Context, config map[string]interface{}) error

	Export(ctx context.Context, records []*AuditRecord) error

	ExportStream(ctx context.Context, records <-chan *AuditRecord) error

	Query(ctx context.Context, q *AuditQuery) (*AuditQueryResult, error)

	ExportFile(ctx context.Context, q *AuditQuery, format string) (io.ReadCloser, string, error)

	Rotate(ctx context.Context, before time.Time) (rotated int64, err error)

	Close() error
}

type AuditRecord struct {
	ID           uint64
	TraceID      string
	Timestamp    time.Time
	UserID       uint64
	Username     string
	ClientIP     string
	UserAgent    string
	Module       string
	Action       string
	TargetType   string
	TargetID     string
	ClusterCode  string
	Namespace    string
	Status       string
	ErrorMsg     string
	RequestMethod string
	RequestURI   string
	RequestBody  string
	ResponseCode int
	CostMs       int
	ExtraFields  map[string]interface{}
}

type AuditQuery struct {
	StartTime   *time.Time
	EndTime     *time.Time
	Username    string
	Module      string
	Action      string
	ClusterCode string
	Namespace   string
	Status      string
	ClientIP    string
	Keyword     string
	Page        int
	Size        int
}

type AuditQueryResult struct {
	Total int64
	Items []*AuditRecord
}
