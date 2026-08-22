package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/k8s-platform/console/internal/backup"
	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/models"
	plugins "github.com/k8s-platform/console/internal/plugins"
	"github.com/k8s-platform/console/internal/plugins/storage"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

const (
	consumerGroup       = "backup-workers"
	pubSubProgressChan  = "notifier:task_progress"
	streamBlockDuration = 5 * time.Second
	pendingClaimTimeout = 5 * time.Minute
	progressInterval    = 1 * time.Second
)

type BackupWorker struct {
	cfg     *config.Config
	log     *logger.Logger
	db      *gorm.DB
	redis   *redis.Client
	plugins *plugins.Manager
	cluster *cluster.Manager

	stopCh chan struct{}
	once   sync.Once
}

func NewBackupWorker(cfg *config.Config, log *logger.Logger, db *gorm.DB, rdb *redis.Client, pm *plugins.Manager, cm *cluster.Manager) *BackupWorker {
	return &BackupWorker{
		cfg:     cfg,
		log:     log,
		db:      db,
		redis:   rdb,
		plugins: pm,
		cluster: cm,
		stopCh:  make(chan struct{}),
	}
}

func (w *BackupWorker) Name() string { return "BackupWorker" }

func (w *BackupWorker) Start(ctx context.Context) error {
	w.log.Info("⚡ BackupWorker starting, stream=", backup.StreamKeyBackupTasks)

	if _, err := w.redis.XGroupCreateMkStream(ctx, backup.StreamKeyBackupTasks, consumerGroup, "0").Result(); err != nil {
		if !isBusyGroupErr(err) {
			w.log.Warnf("XGroupCreate (non-fatal): %v", err)
		}
	}

	autoClaimTicker := time.NewTicker(1 * time.Minute)
	defer autoClaimTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("BackupWorker context done, exiting")
			return nil
		case <-w.stopCh:
			w.log.Info("BackupWorker stopCh closed, exiting")
			return nil
		default:
		}

		select {
		case <-autoClaimTicker.C:
			w.tryAutoClaim(ctx)
		default:
		}

		streams, err := w.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: fmt.Sprintf("bw-%d", time.Now().UnixNano()),
			Streams:  []string{backup.StreamKeyBackupTasks, ">"},
			Count:    1,
			Block:    streamBlockDuration,
		}).Result()
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				continue
			}
			w.log.Warnf("XReadGroup error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, s := range streams {
			for _, msg := range s.Messages {
				w.log.Debugf("BackupWorker got msg: id=%s values=%v", msg.ID, msg.Values)
				if handleErr := w.handleStreamMsg(ctx, msg); handleErr != nil {
					w.log.Errorf("handle stream msg %s failed: %v", msg.ID, handleErr)
				}
				if ackErr := w.redis.XAck(ctx, backup.StreamKeyBackupTasks, consumerGroup, msg.ID).Err(); ackErr != nil {
					w.log.Warnf("XAck %s failed: %v", msg.ID, ackErr)
				}
			}
		}
	}
}

func (w *BackupWorker) Stop() error {
	w.once.Do(func() { close(w.stopCh) })
	return nil
}

func (w *BackupWorker) tryAutoClaim(ctx context.Context) {
	start := "-"
	for i := 0; i < 10; i++ {
		res, claimedStart, err := w.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   backup.StreamKeyBackupTasks,
			Group:    consumerGroup,
			Consumer: fmt.Sprintf("bw-autoclaim-%d", time.Now().UnixNano()),
			MinIdle:  pendingClaimTimeout,
			Start:    start,
			Count:    10,
		}).Result()
		if err != nil {
			w.log.Warnf("XAutoClaim error: %v", err)
			return
		}
		if len(res) == 0 {
			return
		}
		for _, msg := range res {
			w.log.Infof("AutoClaim msg: id=%s values=%v", msg.ID, msg.Values)
			if err := w.handleStreamMsg(ctx, msg); err != nil {
				w.log.Errorf("AutoClaim handle msg %s failed: %v", msg.ID, err)
			}
			if err := w.redis.XAck(ctx, backup.StreamKeyBackupTasks, consumerGroup, msg.ID).Err(); err != nil {
				w.log.Warnf("AutoClaim XAck %s failed: %v", msg.ID, err)
			}
		}
		start = claimedStart
	}
}

func (w *BackupWorker) handleStreamMsg(ctx context.Context, msg redis.XMessage) error {
	taskIDStr, _ := msg.Values["task_id"].(string)
	taskType, _ := msg.Values["task_type"].(string)
	if taskIDStr == "" {
		return fmt.Errorf("task_id missing in msg %s", msg.ID)
	}
	taskID, err := strconv.ParseUint(taskIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("parse task_id %q: %w", taskIDStr, err)
	}
	return w.HandleTask(ctx, taskID, taskType)
}

func (w *BackupWorker) HandleTask(ctx context.Context, taskID uint64, taskType string) error {
	var task models.BackupTask
	if err := w.db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err != nil {
		return fmt.Errorf("load task %d: %w", taskID, err)
	}

	if task.Status != models.BackupStatusPending {
		w.log.Infof("task %d already status=%s, skip", taskID, task.Status)
		return nil
	}

	now := time.Now()
	if err := w.db.Model(&models.BackupTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":     models.BackupStatusRunning,
		"started_at": &now,
	}).Error; err != nil {
		w.log.Warnf("mark running failed: %v", err)
	}

	w.publishProgress(ctx, taskID, 5, string(models.BackupStatusRunning), "任务开始执行")

	progressDone := make(chan struct{})
	go w.publishProgressLoop(ctx, taskID, progressDone)

	var execErr error
	switch taskType {
	case "backup":
		execErr = w.doBackup(ctx, &task)
	case "restore":
		execErr = w.doRestore(ctx, &task)
	default:
		execErr = fmt.Errorf("unknown task_type %q", taskType)
	}

	close(progressDone)

	finishNow := time.Now()
	if execErr != nil {
		errMsg := truncateStr(execErr.Error(), 1024)
		_ = w.db.Model(&models.BackupTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
			"status":        models.BackupStatusFailed,
			"error_message": &errMsg,
			"completed_at":  &finishNow,
		})
		w.publishProgress(ctx, taskID, 100, string(models.BackupStatusFailed), execErr.Error())
		w.log.Errorf("❌ task %d (%s) failed: %v", taskID, taskType, execErr)
		return nil
	}

	w.publishProgress(ctx, taskID, 100, string(models.BackupStatusSuccess), "任务完成")
	w.log.Infof("✅ task %d (%s) success", taskID, taskType)
	return nil
}

func (w *BackupWorker) publishProgressLoop(ctx context.Context, taskID uint64, done <-chan struct{}) {
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	percent := 10
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if percent < 90 {
				percent += 5
			}
			w.publishProgress(ctx, taskID, percent, string(models.BackupStatusRunning), fmt.Sprintf("处理中 %d%%", percent))
		}
	}
}

func (w *BackupWorker) publishProgress(ctx context.Context, taskID uint64, percent int, status, msg string) {
	payload := map[string]interface{}{
		"task_id": taskID,
		"percent": percent,
		"status":  status,
		"msg":     msg,
		"ts":      time.Now().UnixMilli(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = w.redis.Publish(ctx, pubSubProgressChan, string(b)).Err()
}

func (w *BackupWorker) doBackup(ctx context.Context, task *models.BackupTask) error {
	w.publishProgress(ctx, task.ID, 10, "running", "连接集群并读取资源...")

	dynCli, _, err := w.cluster.GetDynamicClient(task.ClusterCode)
	if err != nil {
		return fmt.Errorf("get cluster client: %w", err)
	}
	if task.TargetKind == nil || task.TargetName == nil {
		return errcode.New(errcode.InvalidArgument, "target_kind/target_name missing")
	}

	gvr, err := w.resolveGVR(ctx, dynCli, *task.TargetKind)
	if err != nil {
		return fmt.Errorf("resolve GVR: %w", err)
	}

	var ns string
	if task.Namespace != nil {
		ns = *task.Namespace
	}

	var obj *unstructured.Unstructured
	var rawYAML string
	var count int

	if ns != "" {
		obj, err = dynCli.Resource(gvr).Namespace(ns).Get(ctx, *task.TargetName, metav1.GetOptions{})
	} else {
		obj, err = dynCli.Resource(gvr).Get(ctx, *task.TargetName, metav1.GetOptions{})
	}
	if err != nil {
		return fmt.Errorf("get resource %s/%s: %w", *task.TargetKind, *task.TargetName, err)
	}
	rawYAMLBytes, err := yaml.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	rawYAML = string(rawYAMLBytes)
	count = 1

	w.publishProgress(ctx, task.ID, 40, "running", fmt.Sprintf("读取 %d 个对象，上传存储...", count))

	stor, err := w.plugins.GetStorage(task.StorageType)
	if err != nil {
		return fmt.Errorf("get storage %s: %w", task.StorageType, err)
	}

	key := storage.BuildKey(task.ClusterCode, storage.DateStr(time.Now()),
		fmt.Sprint(task.ID), *task.TargetKind, *task.TargetName)

	uploadRes, err := stor.Upload(ctx, key, bytes.NewBufferString(rawYAML))
	if err != nil {
		return errcode.Wrap(errcode.BackupStorageError, err, "上传备份失败")
	}

	now := time.Now()
	sizeBytes := uploadRes.SizeBytes
	if err := w.db.Model(&models.BackupTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":       models.BackupStatusSuccess,
		"size_bytes":   &sizeBytes,
		"object_count": count,
		"storage_path": uploadRes.StoragePath,
		"completed_at": &now,
	}).Error; err != nil {
		return fmt.Errorf("update backup result: %w", err)
	}
	w.publishProgress(ctx, task.ID, 95, "running", "备份已写入存储，更新记录完成")
	return nil
}

func (w *BackupWorker) doRestore(ctx context.Context, task *models.BackupTask) error {
	w.publishProgress(ctx, task.ID, 10, "running", "下载备份文件...")

	if task.StoragePath == "" {
		return fmt.Errorf("storage_path empty, backup not usable")
	}

	stor, err := w.plugins.GetStorage(task.StorageType)
	if err != nil {
		return fmt.Errorf("get storage %s: %w", task.StorageType, err)
	}

	rc, err := stor.Download(ctx, task.StoragePath)
	if err != nil {
		return errcode.Wrap(errcode.BackupStorageError, err, "下载备份失败")
	}
	defer rc.Close()

	data, err := readAll(rc)
	if err != nil {
		return fmt.Errorf("read backup content: %w", err)
	}

	w.publishProgress(ctx, task.ID, 40, "running", "解析 YAML 并应用到集群...")

	var obj unstructured.Unstructured
	if err := yaml.Unmarshal(data, &obj.Object); err != nil {
		return fmt.Errorf("unmarshal yaml: %w", err)
	}

	dynCli, _, err := w.cluster.GetDynamicClient(task.ClusterCode)
	if err != nil {
		return fmt.Errorf("get cluster client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    obj.GroupVersionKind().Group,
		Version:  obj.GroupVersionKind().Version,
		Resource: "",
	}
	if gvr.Group == "" && gvr.Version == "v1" {
		gvr.Version = "v1"
	}
	if gvr.GroupVersion().Empty() && obj.GetAPIVersion() != "" {
		gv, _ := schema.ParseGroupVersion(obj.GetAPIVersion())
		gvr.Group = gv.Group
		gvr.Version = gv.Version
	}
	resolved, err := w.resolveGVR(ctx, dynCli, obj.GetKind())
	if err == nil {
		gvr = resolved
	}

	ns := obj.GetNamespace()
	if task.Namespace != nil && *task.Namespace != "" {
		ns = *task.Namespace
	}

	var resultObj *unstructured.Unstructured
	count := 1

	if ns != "" {
		existing, getErr := dynCli.Resource(gvr).Namespace(ns).Get(ctx, obj.GetName(), metav1.GetOptions{})
		if getErr == nil && existing != nil {
			obj.SetResourceVersion(existing.GetResourceVersion())
			resultObj, err = dynCli.Resource(gvr).Namespace(ns).Update(ctx, &obj, metav1.UpdateOptions{})
		} else {
			resultObj, err = dynCli.Resource(gvr).Namespace(ns).Create(ctx, &obj, metav1.CreateOptions{})
		}
	} else {
		existing, getErr := dynCli.Resource(gvr).Get(ctx, obj.GetName(), metav1.GetOptions{})
		if getErr == nil && existing != nil {
			obj.SetResourceVersion(existing.GetResourceVersion())
			resultObj, err = dynCli.Resource(gvr).Update(ctx, &obj, metav1.UpdateOptions{})
		} else {
			resultObj, err = dynCli.Resource(gvr).Create(ctx, &obj, metav1.CreateOptions{})
		}
	}
	if err != nil {
		return errcode.Wrap(errcode.BackupRestoreFail, err, "应用资源失败")
	}
	_ = resultObj

	now := time.Now()
	size := int64(len(data))
	if err := w.db.Model(&models.BackupTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":       models.BackupStatusSuccess,
		"size_bytes":   &size,
		"object_count": count,
		"completed_at": &now,
	}).Error; err != nil {
		return fmt.Errorf("update restore result: %w", err)
	}

	w.publishProgress(ctx, task.ID, 95, "running", "恢复完成")
	return nil
}

func (w *BackupWorker) resolveGVR(ctx context.Context, dynCli interface{}, kind string) (schema.GroupVersionResource, error) {
	switch kind {
	case "Pod":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, nil
	case "Service":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, nil
	case "ConfigMap":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, nil
	case "Secret":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, nil
	case "Namespace":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, nil
	case "Deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
	case "StatefulSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, nil
	case "DaemonSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, nil
	case "ReplicaSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, nil
	case "Job":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, nil
	case "CronJob":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, nil
	case "Ingress":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, nil
	case "PersistentVolume":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, nil
	case "PersistentVolumeClaim":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, nil
	case "ServiceAccount":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, nil
	case "Role":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, nil
	case "ClusterRole":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, nil
	case "RoleBinding":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, nil
	case "ClusterRoleBinding":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unknown kind %q (MVP_TODO: dynamic discovery via APIResourceList)", kind)
	}
}

func isBusyGroupErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
