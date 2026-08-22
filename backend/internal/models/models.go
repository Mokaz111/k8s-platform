package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/k8s-platform/console/pkg/logger"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ---------- 通用 JSONB 类型（PostgreSQL） ----------
type JSONB json.RawMessage

func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 || string(j) == "null" {
		return nil, nil
	}
	return []byte(j), nil
}
func (j *JSONB) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}
	result := json.RawMessage{}
	err := json.Unmarshal(bytes, &result)
	*j = JSONB(result)
	return err
}
func (j JSONB) MarshalJSON() ([]byte, error) { return []byte(j), nil }
func (j *JSONB) UnmarshalJSON(data []byte) error {
	*j = append((*j)[0:0], data...)
	return nil
}

// ---------- 时间基类 ----------
type Timestamps struct {
	CreatedAt time.Time      `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// ---------- Cluster ----------
type ClusterStatus int8

const (
	ClusterStatusOffline ClusterStatus = 0
	ClusterStatusOnline  ClusterStatus = 1
)

type Cluster struct {
	ID           uint64         `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name         string         `gorm:"column:name;type:varchar(64);not null;uniqueIndex" json:"name"`
	Code         string         `gorm:"column:code;type:varchar(64);not null;uniqueIndex" json:"code"`
	CreatorID    uint64         `gorm:"column:creator_id;not null;default:0" json:"creator_id,omitempty"`
	Kubeconfig   []byte         `gorm:"column:kubeconfig;type:bytea;not null" json:"-"` // 密文，JSON 永不可序列化
	APIServer    string         `gorm:"column:api_server;type:varchar(256)" json:"api_server,omitempty"`
	Status       ClusterStatus  `gorm:"column:status;not null;default:0;index" json:"status"`
	Version      string         `gorm:"column:version;type:varchar(32)" json:"version,omitempty"`
	NodeCount    int            `gorm:"column:node_count;not null;default:0" json:"node_count"`
	LastSyncAt   *time.Time     `gorm:"column:last_sync_at" json:"last_sync_at,omitempty"`
	Description  string         `gorm:"column:description;type:varchar(512)" json:"description,omitempty"`
	Labels       JSONB          `gorm:"column:labels;type:jsonb" json:"labels,omitempty"`
	Timestamps
}

func (Cluster) TableName() string { return "cluster" }

// ---------- ResourceSnapshot (版本快照) ----------
type ResourceSnapshot struct {
	ID            uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ClusterCode   string     `gorm:"column:cluster_code;type:varchar(64);not null;index:idx_cluster_kind_name" json:"cluster_code"`
	Namespace     string     `gorm:"column:namespace;type:varchar(64);not null;default:'_cluster_';index:idx_version_seq" json:"namespace"`
	APIVersion    string     `gorm:"column:api_version;type:varchar(128);not null;index:idx_cluster_kind_name" json:"api_version"`
	Kind          string     `gorm:"column:kind;type:varchar(64);not null;index:idx_cluster_kind_name;index:idx_version_seq" json:"kind"`
	Name          string     `gorm:"column:name;type:varchar(256);not null;index:idx_cluster_kind_name;index:idx_version_seq" json:"name"`
	VersionSeq    int        `gorm:"column:version_seq;not null;index:idx_version_seq" json:"version_seq"`
	RawYAML       string     `gorm:"column:raw_yaml;type:mediumtext;not null" json:"raw_yaml"`
	ChangeSummary string     `gorm:"column:change_summary;type:varchar(512)" json:"change_summary,omitempty"`
	Operator      string     `gorm:"column:operator;type:varchar(64);not null" json:"operator"`
	Source        string     `gorm:"column:source;type:varchar(32);not null;default:'ui'" json:"source"` // ui / rollback / backup
	OperatorID    uint64     `gorm:"column:operator_id" json:"operator_id,omitempty"`
	Timestamps
}

func (ResourceSnapshot) TableName() string { return "resource_snapshot" }

// ---------- BackupTask ----------
type BackupTaskStatus string

const (
	BackupStatusPending   BackupTaskStatus = "pending"
	BackupStatusRunning   BackupTaskStatus = "running"
	BackupStatusSuccess   BackupTaskStatus = "success"
	BackupStatusFailed    BackupTaskStatus = "failed"
	BackupStatusCancelled BackupTaskStatus = "cancelled"
)

type BackupTask struct {
	ID            uint64           `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ClusterCode   string           `gorm:"column:cluster_code;type:varchar(64);not null;index" json:"cluster_code"`
	Namespace     *string          `gorm:"column:namespace;type:varchar(64);index" json:"namespace,omitempty"`
	TargetKind    *string          `gorm:"column:target_kind;type:varchar(64)" json:"target_kind,omitempty"`
	TargetName    *string          `gorm:"column:target_name;type:varchar(256)" json:"target_name,omitempty"`
	BackupType    string           `gorm:"column:backup_type;type:varchar(16);not null" json:"backup_type"` // single / namespace
	StorageType   string           `gorm:"column:storage_type;type:varchar(32);not null;default:'local'" json:"storage_type"`
	StoragePath   string           `gorm:"column:storage_path;type:varchar(512)" json:"storage_path"`
	Status        BackupTaskStatus `gorm:"column:status;type:varchar(32);not null;default:'pending';index" json:"status"`
	SizeBytes     *int64           `gorm:"column:size_bytes" json:"size_bytes,omitempty"`
	Operator      string           `gorm:"column:operator;type:varchar(64)" json:"operator,omitempty"`
	OperatorID    uint64           `gorm:"column:operator_id" json:"operator_id,omitempty"`
	StartedAt     *time.Time       `gorm:"column:started_at;default:CURRENT_TIMESTAMP" json:"started_at,omitempty"`
	CompletedAt   *time.Time       `gorm:"column:completed_at" json:"completed_at,omitempty"`
	ErrorMessage  *string          `gorm:"column:error_message;type:varchar(1024)" json:"error_message,omitempty"`
	ObjectCount   int              `gorm:"column:object_count;not null;default:0" json:"object_count"`
	Timestamps
}

func (BackupTask) TableName() string { return "backup_task" }

// ---------- PluginRegistry ----------
type PluginRegistry struct {
	ID            uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PluginID      string `gorm:"column:plugin_id;type:varchar(64);not null;uniqueIndex" json:"plugin_id"`
	Name          string `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Version       string `gorm:"column:version;type:varchar(32);not null" json:"version"`
	Kind          string `gorm:"column:kind;type:varchar(32)" json:"kind"` // resource/backup_storage/auth/notifier/audit/kms
	Capabilities  JSONB  `gorm:"column:capabilities;type:jsonb" json:"capabilities,omitempty"`
	EntryPath     string `gorm:"column:entry_path;type:varchar(512)" json:"entry_path,omitempty"`
	GRPCPortRange string `gorm:"column:grpc_port_range;type:varchar(32)" json:"grpc_port_range,omitempty"`
	Config        JSONB  `gorm:"column:config;type:jsonb" json:"config,omitempty"`
	Status        int8   `gorm:"column:status;not null;default:1" json:"status"` // 0 禁用 1 启用
	Builtin       int8   `gorm:"column:builtin;not null;default:0" json:"builtin"` // 0 外置 1 内置
	Timestamps
}

func (PluginRegistry) TableName() string { return "plugin_registry" }

// ---------- SysUser ----------
type UserStatus int8

const (
	UserStatusDisabled UserStatus = 0
	UserStatusEnabled  UserStatus = 1
	UserStatusLocked   UserStatus = 2
)

type SysUser struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"column:username;type:varchar(64);not null;uniqueIndex" json:"username"`
	DisplayName  string     `gorm:"column:display_name;type:varchar(128)" json:"display_name"`
	Email        string     `gorm:"column:email;type:varchar(128);index" json:"email,omitempty"`
	Phone        string     `gorm:"column:phone;type:varchar(32)" json:"phone,omitempty"`
	PasswordHash string     `gorm:"column:password_hash;type:varchar(256);not null" json:"-"`
	AuthSource   string     `gorm:"column:auth_source;type:varchar(32);not null;default:'local'" json:"auth_source"`
	Status       UserStatus `gorm:"column:status;not null;default:1;index" json:"status"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	LastLoginIP  string     `gorm:"column:last_login_ip;type:varchar(64)" json:"last_login_ip,omitempty"`
	Timestamps
}

func (SysUser) TableName() string { return "sys_user" }

// SetPassword 设置密码（bcrypt hash）
func (u *SysUser) SetPassword(raw string, cost int) error {
	h, err := bcrypt.GenerateFromPassword([]byte(raw), cost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(h)
	return nil
}
func (u *SysUser) CheckPassword(raw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(raw)) == nil
}

// ---------- SysRole ----------
type SysRole struct {
	ID          uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Code        string `gorm:"column:code;type:varchar(64);not null;uniqueIndex" json:"code"`
	Name        string `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Description string `gorm:"column:description;type:varchar(512)" json:"description,omitempty"`
	Builtin     int8   `gorm:"column:builtin;not null;default:0" json:"builtin"` // 1 内置不可删
	Status      int8   `gorm:"column:status;not null;default:1" json:"status"`
	Timestamps
}

func (SysRole) TableName() string { return "sys_role" }

// ---------- SysPermission ----------
type SysPermission struct {
	ID          uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Code        string `gorm:"column:code;type:varchar(128);not null;uniqueIndex" json:"code"` // module:action 如 clusters:read
	Module      string `gorm:"column:module;type:varchar(64);not null;index" json:"module"`
	Action      string `gorm:"column:action;type:varchar(64);not null" json:"action"`
	Name        string `gorm:"column:name;type:varchar(128);not null" json:"name"`
	Description string `gorm:"column:description;type:varchar(512)" json:"description,omitempty"`
	Timestamps
}

func (SysPermission) TableName() string { return "sys_permission" }

// ---------- SysRolePermission ----------
type SysRolePermission struct {
	ID             uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	RoleID         uint64 `gorm:"column:role_id;not null;uniqueIndex:uk_role_perm"`
	PermissionCode string `gorm:"column:permission_code;type:varchar(128);not null;uniqueIndex:uk_role_perm"`
}

func (SysRolePermission) TableName() string { return "sys_role_permission" }

// ---------- SysUserRole ----------
type ScopeType string

const (
	ScopePlatform  ScopeType = "platform"
	ScopeCluster   ScopeType = "cluster"
	ScopeNamespace ScopeType = "namespace"
)

type SysUserRole struct {
	ID                uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID            uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_user_role_scope" json:"user_id"`
	RoleID            uint64    `gorm:"column:role_id;not null;uniqueIndex:uk_user_role_scope" json:"role_id"`
	ScopeType         ScopeType `gorm:"column:scope_type;type:varchar(32);not null;default:'platform';uniqueIndex:uk_user_role_scope" json:"scope_type"`
	ScopeClusterCode  string    `gorm:"column:scope_cluster_code;type:varchar(64);uniqueIndex:uk_user_role_scope" json:"scope_cluster_code,omitempty"`
	ScopeNamespace    string    `gorm:"column:scope_namespace;type:varchar(64);uniqueIndex:uk_user_role_scope" json:"scope_namespace,omitempty"`
	Timestamps
}

func (SysUserRole) TableName() string { return "sys_user_role" }

// ---------- AuditLog ----------
type AuditLog struct {
	ID           uint64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TraceID      string `gorm:"column:trace_id;type:varchar(64);index" json:"trace_id"`
	UserID       uint64 `gorm:"column:user_id;index" json:"user_id,omitempty"`
	Username     string `gorm:"column:username;type:varchar(64);index:idx_user_module" json:"username"`
	ClientIP     string `gorm:"column:client_ip;type:varchar(64)" json:"client_ip"`
	UserAgent    string `gorm:"column:user_agent;type:varchar(512)" json:"user_agent"`
	Module       string `gorm:"column:module;type:varchar(64);not null;index:idx_user_module" json:"module"`
	Action       string `gorm:"column:action;type:varchar(64);not null" json:"action"`
	TargetType   string `gorm:"column:target_type;type:varchar(64);index:idx_cluster_target" json:"target_type,omitempty"`
	TargetID     string `gorm:"column:target_id;type:varchar(256)" json:"target_id,omitempty"`
	ClusterCode  string `gorm:"column:cluster_code;type:varchar(64);index:idx_cluster_target" json:"cluster_code,omitempty"`
	Namespace    string `gorm:"column:namespace;type:varchar(64)" json:"namespace,omitempty"`
	Status       string `gorm:"column:status;type:varchar(16);not null;default:'success'" json:"status"` // success / fail
	ErrorMsg     string `gorm:"column:error_msg;type:varchar(1024)" json:"error_msg,omitempty"`
	RequestMethod string `gorm:"column:request_method;type:varchar(16)" json:"request_method"`
	RequestURI   string `gorm:"column:request_uri;type:varchar(512)" json:"request_uri"`
	RequestBody  JSONB  `gorm:"column:request_body;type:jsonb" json:"request_body,omitempty"`
	ResponseCode int    `gorm:"column:response_code" json:"response_code"`
	CostMs       int    `gorm:"column:cost_ms" json:"cost_ms"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;index:idx_created_at" json:"created_at"`
}

func (AuditLog) TableName() string { return "audit_log" }

// ---------- AutoMigrate & Seed ----------

// AutoMigrate 自动建表（开发环境使用）
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Cluster{},
		&ResourceSnapshot{},
		&BackupTask{},
		&PluginRegistry{},
		&SysUser{},
		&SysRole{},
		&SysPermission{},
		&SysRolePermission{},
		&SysUserRole{},
		&AuditLog{},
	)
}

// SeedInitialData 初始化种子数据：内置权限点、内置角色、默认管理员账号
func SeedInitialData(db *gorm.DB, log *logger.Logger) error {
	// 1. 权限点
	if err := seedPermissions(db); err != nil {
		return fmt.Errorf("seed permissions: %w", err)
	}
	// 2. 角色 + 绑定权限点
	if err := seedRoles(db); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}
	// 3. 默认管理员账号 admin / admin123
	if err := seedAdminUser(db, log); err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}
	return nil
}
