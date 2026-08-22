package backup

import (
	"context"
	"fmt"
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
	RestoreModeOverwrite  RestoreMode = "overwrite"
	RestoreModeCreateNew  RestoreMode = "create-new"
)

type SubmitBackupInput struct {
	ClusterCode string
	Namespace   string
	TargetKind  string
	TargetName  string
	StorageType string
	Operator    string
	OperatorID  uint64
}

type SubmitRestoreInput struct {
	BackupID        uint64
	TargetClusterCode string
	Operator        string
	OperatorID      uint64
	Mode            RestoreMode
}

type ListInput struct {
	Page        int
	Size        int
	ClusterCode string
	Status      models.BackupTaskStatus
	Keyword     string
}

type ListResult struct {
	Total int64
	Items []models.BackupTask
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

func (m *Manager) SubmitBackupTask(ctx context.Context, in *SubmitBackupInput) (*models.BackupTask, error) {
	if in.ClusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster_code 不能为空")
	}
	if in.TargetKind == "" || in.TargetName == "" {
		return nil, errcode.New(errcode.InvalidArgument, "target_kind 和 target_name 不能为空")
	}
	storType := in.StorageType
	if storType == "" {
		storType = m.Plugins.DefaultStorageType()
	}
	if _, err := m.Plugins.GetStorage(storType); err != nil {
		return nil, err
	}

	backupType := "single"
	var nsPtr *string
	if in.Namespace != "" {
		ns := in.Namespace
		nsPtr = &ns
	}
	kind := in.TargetKind
	name := in.TargetName
	task := &models.BackupTask{
		ClusterCode: in.ClusterCode,
		Namespace:   nsPtr,
		TargetKind:  &kind,
		TargetName:  &name,
		BackupType:  backupType,
		StorageType: storType,
		Status:      models.BackupStatusPending,
		Operator:    in.Operator,
		OperatorID:  in.OperatorID,
	}
	if err := m.DB.WithContext(ctx).Create(task).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err, "创建备份任务失败")
	}

	if err := m.pushTaskToStream(ctx, task.ID, "backup"); err != nil {
		_ = m.setTaskError(task.ID, err.Error())
		return nil, err
	}
	m.Log.Infof("📦 backup task submitted: id=%d cluster=%s kind=%s name=%s storage=%s",
		task.ID, in.ClusterCode, in.TargetKind, in.TargetName, storType)
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
	task := &models.BackupTask{
		ClusterCode: targetCode,
		Namespace:   src.Namespace,
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
	m.Log.Infof("🔄 restore task submitted: id=%d from_backup=%d target_cluster=%s mode=%s",
		task.ID, in.BackupID, targetCode, mode)
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
	if in.Status != "" {
		q = q.Where("status = ?", in.Status)
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

func (m *Manager) Delete(ctx context.Context, id uint64) error {
	t, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.StoragePath != "" {
		s, getErr := m.Plugins.GetStorage(t.StorageType)
		if getErr == nil {
			key := extractKeyFromPath(t.StoragePath, s.StorageType())
			if key != "" {
				if delErr := s.Delete(ctx, key); delErr != nil {
					m.Log.Warnf("delete backup storage failed: id=%d key=%s err=%v", id, key, delErr)
				} else {
					m.Log.Infof("🗑️ backup storage deleted: id=%d key=%s", id, key)
				}
			} else if s.StorageType() == "local" {
				if delErr := s.Delete(ctx, t.StoragePath); delErr != nil {
					m.Log.Warnf("delete local backup file failed: id=%d path=%s err=%v", id, t.StoragePath, delErr)
				}
			}
		}
	}
	if err := m.DB.WithContext(ctx).Delete(&models.BackupTask{}, id).Error; err != nil {
		return errcode.Wrap(errcode.DatabaseError, err, "删除备份记录失败")
	}
	return nil
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
		"status":        models.BackupStatusFailed,
		"error_message": &msg,
		"completed_at":  &now,
	}).Error
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func extractKeyFromPath(storagePath, storageType string) string {
	if storagePath == "" {
		return ""
	}
	if storageType == "local" {
		return ""
	}
	return storagePath
}
