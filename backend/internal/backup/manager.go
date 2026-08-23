package backup

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/k8s-platform/console/internal/models"
	plugins "github.com/k8s-platform/console/internal/plugins"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	StreamKeyBackupTasks = "stream:backup:tasks"
)

type RestoreMode string

const (
	RestoreModeOverwrite RestoreMode = "overwrite"
	RestoreModeCreateNew RestoreMode = "create-new"
)

// BackupMode 备份模式
//   - single：单对象（原有语义）
//   - namespace_batch：命名空间级批量（namespaces[] × kind_filter[]）
type BackupMode string

const (
	BackupModeSingle          BackupMode = "single"
	BackupModeNamespaceBatch  BackupMode = "namespace_batch"
)

type SubmitBackupInput struct {
	ClusterCode string
	// Mode=single 时使用：
	Namespace  string
	TargetKind string
	TargetName string
	// Mode=namespace_batch 时使用：
	Namespaces []string
	KindFilter []string
	// 公共字段：
	Mode        BackupMode
	StorageType string
	Operator    string
	OperatorID  uint64
}

type SubmitRestoreInput struct {
	BackupID          uint64
	TargetClusterCode string
	Operator          string
	OperatorID        uint64
	Mode              RestoreMode
	// 恢复到指定命名空间（可选，留空沿用备份时的 namespace）
	TargetNamespace string
}

type ListInput struct {
	Page        int
	Size        int
	ClusterCode string
	Status      models.BackupTaskStatus
	Keyword     string
	BackupType  string

	// RBAC 数据范围过滤字段（由 handler 注入）
	ClusterCodes []string
	Namespaces   []string
}

type ListResult struct {
	Total int64
	Items []models.BackupTask
}

type DownloadResult struct {
	Reader      io.ReadCloser
	SizeBytes   int64
	SuggestName string
	ContentType string
}

type Manager struct {
	DB      *gorm.DB
	Redis   *redis.Client
	Plugins *plugins.Manager
	Log     *logger.Logger
}

func NewManager(db *gorm.DB, rdb *redis.Client, pm *plugins.Manager, log *logger.Logger) *Manager {
	return &Manager{DB: db, Redis: rdb, Plugins: pm, Log: log}
}

// normalizeStorageType 统一前后端 storage_type 大小写：
//   前端会传 'Local'/'NFS'/'S3' (首字母大写)，内部统一成 local/nfs/s3。
func normalizeStorageType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "local":
		return "local"
	case "nfs":
		return "nfs"
	case "s3":
		return "s3"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

func (m *Manager) SubmitBackupTask(ctx context.Context, in *SubmitBackupInput) (*models.BackupTask, error) {
	if in.ClusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster_code 不能为空")
	}
	mode := in.Mode
	if mode == "" {
		mode = BackupModeSingle
	}

	storType := normalizeStorageType(in.StorageType)
	if storType == "" {
		storType = m.Plugins.DefaultStorageType()
	}
	if _, err := m.Plugins.GetStorage(storType); err != nil {
		return nil, err
	}

	// ========== 参数校验 ==========
	switch mode {
	case BackupModeSingle:
		if in.TargetKind == "" || in.TargetName == "" {
			return nil, errcode.New(errcode.InvalidArgument, "target_kind 和 target_name 不能为空")
		}
	case BackupModeNamespaceBatch:
		if len(in.Namespaces) == 0 {
			return nil, errcode.New(errcode.InvalidArgument, "命名空间批量模式必须提供 namespaces 列表")
		}
		if len(in.KindFilter) == 0 {
			return nil, errcode.New(errcode.InvalidArgument, "命名空间批量模式必须提供 kind_filter 列表")
		}
	default:
		return nil, errcode.New(errcode.InvalidArgument, "未知备份模式: "+string(mode))
	}

	// ========== 构造任务记录 ==========
	var (
		nsPtr     *string
		kindPtr   *string
		namePtr   *string
		backupType = string(mode)
	)
	if mode == BackupModeSingle {
		kindPtr = sPtr(in.TargetKind)
		namePtr = sPtr(in.TargetName)
		if in.Namespace != "" {
			nsPtr = sPtr(in.Namespace)
		}
	} else {
		// namespace_batch：target_kind 存第一个 kind 方便展示；target_name 留空
		if len(in.KindFilter) > 0 {
			kindPtr = sPtr(in.KindFilter[0])
		}
		if len(in.Namespaces) == 1 {
			nsPtr = sPtr(in.Namespaces[0])
		}
		// 多命名空间时 namespace 列存 "multi:<count>" 文本做区分展示
		if len(in.Namespaces) > 1 {
			nsPtr = sPtr(fmt.Sprintf("multi:%d", len(in.Namespaces)))
		}
	}

	task := &models.BackupTask{
		ClusterCode: in.ClusterCode,
		Namespace:   nsPtr,
		TargetKind:  kindPtr,
		TargetName:  namePtr,
		BackupType:  backupType,
		StorageType: storType,
		Status:      models.BackupStatusPending,
		Operator:    in.Operator,
		OperatorID:  in.OperatorID,
	}
	if err := m.DB.WithContext(ctx).Create(task).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err, "创建备份任务失败")
	}

	// namespace_batch：把 namespaces/kind_filter 写一份到 Redis HASH（task_id:payload），Worker 端消费
	if mode == BackupModeNamespaceBatch {
		if err := m.writeBatchPayload(ctx, task.ID, in); err != nil {
			_ = m.setTaskError(task.ID, err.Error())
			return nil, err
		}
	}

	if err := m.pushTaskToStream(ctx, task.ID, "backup"); err != nil {
		_ = m.setTaskError(task.ID, err.Error())
		return nil, err
	}
	m.Log.Infof("📦 backup task submitted: id=%d cluster=%s mode=%s storage=%s",
		task.ID, in.ClusterCode, mode, storType)
	return task, nil
}

func (m *Manager) SubmitRestoreTask(ctx context.Context, in *SubmitRestoreInput) (*models.BackupTask, error) {
	if in.BackupID == 0 {
		return nil, errcode.New(errcode.InvalidArgument, "backup_id 不能为空")
	}
	mode := in.Mode
	if mode == "" {
		mode = RestoreModeCreateNew
	}
	if mode != RestoreModeOverwrite && mode != RestoreModeCreateNew {
		return nil, errcode.New(errcode.InvalidArgument, "mode 只能是 overwrite 或 create-new")
	}

	src, err := m.Get(ctx, in.BackupID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, errcode.New(errcode.BackupNotFound)
	}
	if src.Status != models.BackupStatusSuccess {
		return nil, errcode.New(errcode.BackupRestoreFail,
			fmt.Sprintf("源备份状态为 %s，仅 success 可恢复", src.Status))
	}

	targetCode := in.TargetClusterCode
	if targetCode == "" {
		targetCode = src.ClusterCode
	}
	if _, err := m.Plugins.GetStorage(src.StorageType); err != nil {
		return nil, err
	}

	modeStr := string(mode)
	restoreType := "restore"
	restoreName := fmt.Sprintf("restore-%d-%s", in.BackupID, uuid.New().String()[:8])

	// TargetNamespace 非空时覆盖恢复到该命名空间
	targetNS := src.Namespace
	if in.TargetNamespace != "" {
		targetNS = &in.TargetNamespace
	}

	task := &models.BackupTask{
		ClusterCode: targetCode,
		Namespace:   targetNS,
		TargetKind:  src.TargetKind,
		TargetName:  &restoreName,
		BackupType:  restoreType,
		StorageType: src.StorageType,
		StoragePath: src.StoragePath,
		Status:      models.BackupStatusPending,
		Operator:    in.Operator,
		OperatorID:  in.OperatorID,
	}
	_ = modeStr

	if err := m.DB.WithContext(ctx).Create(task).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err, "创建恢复任务失败")
	}

	if err := m.pushTaskToStream(ctx, task.ID, "restore"); err != nil {
		_ = m.setTaskError(task.ID, err.Error())
		return nil, err
	}
	m.Log.Infof("🔄 restore task submitted: id=%d from_backup=%d target_cluster=%s target_ns=%v mode=%s",
		task.ID, in.BackupID, targetCode, targetNS, mode)
	return task, nil
}

func (m *Manager) List(ctx context.Context, in *ListInput) (*ListResult, error) {
	page := in.Page
	if page < 1 {
		page = 1
	}
	size := in.Size
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}

	q := m.DB.WithContext(ctx).Model(&models.BackupTask{})
	if in.ClusterCode != "" {
		q = q.Where("cluster_code = ?", in.ClusterCode)
	}
	if len(in.ClusterCodes) > 0 {
		q = q.Where("cluster_code IN ?", in.ClusterCodes)
	}
	if len(in.Namespaces) > 0 {
		q = q.Where("namespace IN ?", in.Namespaces)
	}
	if in.Status != "" {
		q = q.Where("status = ?", in.Status)
	}
	if in.BackupType != "" {
		q = q.Where("backup_type = ?", in.BackupType)
	}
	if in.Keyword != "" {
		kw := "%" + strings.ToLower(in.Keyword) + "%"
		q = q.Where(
			"lower(COALESCE(target_kind,'')) LIKE ? OR lower(COALESCE(target_name,'')) LIKE ? OR lower(cluster_code) LIKE ? OR lower(COALESCE(operator,'')) LIKE ?",
			kw, kw, kw, kw,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	var items []models.BackupTask
	offset := (page - 1) * size
	if err := q.Order("id DESC").Offset(offset).Limit(size).Find(&items).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	return &ListResult{Total: total, Items: items}, nil
}

func (m *Manager) Get(ctx context.Context, id uint64) (*models.BackupTask, error) {
	if id == 0 {
		return nil, errcode.New(errcode.InvalidArgument, "id 不能为空")
	}
	var t models.BackupTask
	if err := m.DB.WithContext(ctx).Where("id = ?", id).First(&t).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.BackupNotFound)
		}
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	return &t, nil
}

// GetBatchPayload 供 Worker 读取批量备份的 namespaces / kind_filter（Redis HASH，TTL = 7 天）
func (m *Manager) GetBatchPayload(ctx context.Context, taskID uint64) (namespaces, kindFilter []string, err error) {
	key := batchPayloadKey(taskID)
	raw, gerr := m.Redis.HGetAll(ctx, key).Result()
	if gerr != nil {
		return nil, nil, errcode.Wrap(errcode.RedisError, gerr, "读取 batch payload")
	}
	if len(raw) == 0 {
		return nil, nil, errcode.New(errcode.InvalidArgument, "该任务没有 batch payload（可能不是 namespace_batch 模式）")
	}
	if nsStr, ok := raw["namespaces"]; ok && nsStr != "" {
		namespaces = strings.Split(nsStr, ",")
	}
	if kfStr, ok := raw["kind_filter"]; ok && kfStr != "" {
		kindFilter = strings.Split(kfStr, ",")
	}
	return namespaces, kindFilter, nil
}

func (m *Manager) Delete(ctx context.Context, id uint64) error {
	t, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.StoragePath != "" {
		s, getErr := m.Plugins.GetStorage(t.StorageType)
		if getErr == nil {
			key := t.StoragePath
			// 兼容 legacy：如果是 local 且存的是绝对路径，仍然直接按路径删
			if delErr := s.Delete(ctx, key); delErr != nil {
				m.Log.Warnf("delete backup storage failed: id=%d key=%s err=%v", id, key, delErr)
			} else {
				m.Log.Infof("🗑️ backup storage deleted: id=%d key=%s", id, key)
			}
		}
	}
	if err := m.DB.WithContext(ctx).Delete(&models.BackupTask{}, id).Error; err != nil {
		return errcode.Wrap(errcode.DatabaseError, err, "删除备份记录失败")
	}
	_ = m.Redis.Del(ctx, batchPayloadKey(id)).Err()
	return nil
}

// Download 返回备份文件 ReadCloser（Manager 层统一解 StoragePath 语义，兼容历史绝对路径）
func (m *Manager) Download(ctx context.Context, id uint64) (*DownloadResult, error) {
	t, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t.Status != models.BackupStatusSuccess {
		return nil, errcode.New(errcode.BackupRestoreFail,
			fmt.Sprintf("备份状态为 %s，仅 success 可下载", t.Status))
	}
	if t.StoragePath == "" {
		return nil, errcode.New(errcode.BackupRestoreFail, "该备份无存储文件（可能是批量备份未打包）")
	}
	stor, gerr := m.Plugins.GetStorage(t.StorageType)
	if gerr != nil {
		return nil, gerr
	}
	rc, dlerr := stor.Download(ctx, t.StoragePath)
	if dlerr != nil {
		return nil, errcode.Wrap(errcode.BackupStorageError, dlerr, "下载备份文件失败")
	}
	size := int64(0)
	if t.SizeBytes != nil {
		size = *t.SizeBytes
	}
	suggest := suggestDownloadName(t)
	return &DownloadResult{
		Reader:      rc,
		SizeBytes:   size,
		SuggestName: suggest,
		ContentType: "application/yaml",
	}, nil
}

func suggestDownloadName(t *models.BackupTask) string {
	kind := "backup"
	if t.TargetKind != nil && *t.TargetKind != "" {
		kind = *t.TargetKind
	}
	name := fmt.Sprintf("backup-%d-%s-%s.yaml", t.ID, t.ClusterCode, kind)
	if t.TargetName != nil && *t.TargetName != "" {
		name = fmt.Sprintf("backup-%d-%s-%s-%s.yaml", t.ID, t.ClusterCode, kind, *t.TargetName)
	}
	return name
}

func (m *Manager) writeBatchPayload(ctx context.Context, taskID uint64, in *SubmitBackupInput) error {
	key := batchPayloadKey(taskID)
	fields := map[string]interface{}{
		"namespaces":  strings.Join(in.Namespaces, ","),
		"kind_filter": strings.Join(in.KindFilter, ","),
		"cluster":     in.ClusterCode,
		"operator":    in.Operator,
	}
	if err := m.Redis.HSet(ctx, key, fields).Err(); err != nil {
		return errcode.Wrap(errcode.RedisError, err, "写入 batch payload")
	}
	_ = m.Redis.Expire(ctx, key, 7*24*time.Hour).Err()
	return nil
}

func batchPayloadKey(taskID uint64) string {
	return fmt.Sprintf("backup:batch:%d", taskID)
}

func (m *Manager) pushTaskToStream(ctx context.Context, taskID uint64, taskType string) error {
	values := map[string]interface{}{
		"task_id":    strconv.FormatUint(taskID, 10),
		"task_type":  taskType,
		"enqueue_at": fmt.Sprint(time.Now().Unix()),
	}
	args := &redis.XAddArgs{
		Stream: StreamKeyBackupTasks,
		ID:     "*",
		Values: values,
	}
	if _, err := m.Redis.XAdd(ctx, args).Result(); err != nil {
		return errcode.Wrap(errcode.RedisError, err, "写入 Redis Stream 失败")
	}
	return nil
}

func (m *Manager) setTaskError(id uint64, errMsg string) error {
	now := time.Now()
	msg := truncate(errMsg, 1024)
	return m.DB.Model(&models.BackupTask{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":         models.BackupStatusFailed,
		"error_message":  &msg,
		"completed_at":   &now,
	}).Error
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func sPtr(s string) *string {
	return &s
}
