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

type activeTaskRecord struct {
	task     *models.BackupTask
	taskType string // backup | restore
}

type BackupWorker struct {
	cfg     *config.Config
	log     *logger.Logger
	db      *gorm.DB
	redis   *redis.Client
	plugins *plugins.Manager
	cluster *cluster.Manager

	activeTasks sync.Map // uint64(taskID) -> *activeTaskRecord
	stopCh      chan struct{}
	once        sync.Once
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

type dynClient interface {
	Resource(gvr schema.GroupVersionResource) namespaceableResource
}
type namespaceableResource interface {
	Namespace(string) namespacedClient
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*unstructured.Unstructured, error)
	List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error)
}
type namespacedClient interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*unstructured.Unstructured, error)
	List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error)
}

// dynamicClientWrap 包装 DynamicClient（无接口依赖），用于让 doBackupSingle 更方便地测 namespaceableResource
type dynamicClientWrap struct {
	dyn interface {
		Resource(gvr schema.GroupVersionResource) namespaceableResource
	}
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

	// 记录活跃 task，便于 publishProgress 从 taskID 反查完整元信息
	w.activeTasks.Store(taskID, &activeTaskRecord{task: &task, taskType: taskType})
	defer w.activeTasks.Delete(taskID)

	w.publishProgress(ctx, taskID, 5, string(models.BackupStatusRunning), "任务开始执行")

	progressDone := make(chan struct{})
	go w.publishProgressLoop(ctx, taskID, progressDone)

	var execErr error
	switch taskType {
	case "backup":
		if task.BackupType == "namespace_batch" || task.BackupType == string(backup.BackupModeNamespaceBatch) {
			execErr = w.doBackupNamespaceBatch(ctx, &task)
		} else {
			execErr = w.doBackupSingle(ctx, &task)
		}
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
		w.publishProgressWithError(ctx, taskID, 100, string(models.BackupStatusFailed), "任务失败", execErr.Error())
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
				percent += 2
			}
			w.publishProgress(ctx, taskID, percent, string(models.BackupStatusRunning), fmt.Sprintf("处理中 %d%%", percent))
		}
	}
}

// taskProgressPayload 与前端 wsSlice TaskProgressPayload 字段完全对齐
type taskProgressPayload struct {
	TaskID      string `json:"task_id"`
	BackupID    string `json:"backup_id,omitempty"`
	ClusterCode string `json:"cluster_code,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Stage       string `json:"stage"` // pending | exporting | uploading | completed | failed | restoring
	Progress    int    `json:"progress"`
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// resolveStage 把 status/percent/taskType 映射到前端约定的 stage 枚举
func resolveStage(status string, percent int, taskType string) string {
	switch status {
	case string(models.BackupStatusPending):
		return "pending"
	case string(models.BackupStatusSuccess):
		return "completed"
	case string(models.BackupStatusFailed):
		return "failed"
	case "running":
		// 兼容 running 状态，下面按 taskType + percent 细分
		break
	default:
		// 未知状态透传兜底
		return status
	}
	// running：根据 taskType + percent 细粒度划分
	if taskType == "restore" {
		return "restoring"
	}
	if percent < 50 {
		return "exporting"
	}
	return "uploading"
}

func (w *BackupWorker) buildProgressPayload(taskID uint64, percent int, status string, msg string, errMsg string) *taskProgressPayload {
	taskIDStr := strconv.FormatUint(taskID, 10)
	p := &taskProgressPayload{
		TaskID:    taskIDStr,
		BackupID:  taskIDStr,
		Stage:     "pending",
		Progress:  percent,
		Message:   msg,
		Error:     errMsg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	taskType := "backup"
	if v, ok := w.activeTasks.Load(taskID); ok {
		if rec, ok := v.(*activeTaskRecord); ok && rec != nil {
			taskType = rec.taskType
			if rec.task != nil {
				t := rec.task
				p.ClusterCode = t.ClusterCode
				if t.Namespace != nil {
					p.Namespace = *t.Namespace
				}
				if t.TargetKind != nil {
					p.Kind = *t.TargetKind
				}
				if t.TargetName != nil {
					p.Name = *t.TargetName
				}
			}
		}
	}
	p.Stage = resolveStage(status, percent, taskType)
	return p
}

// publishProgress 发送进度事件到 Redis PubSub（字段对齐前端 TaskProgressPayload）
func (w *BackupWorker) publishProgress(ctx context.Context, taskID uint64, percent int, status, msg string) {
	payload := w.buildProgressPayload(taskID, percent, status, msg, "")
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = w.redis.Publish(ctx, pubSubProgressChan, string(b)).Err()
}

// publishProgressWithError 发送带 error 字段的进度事件（用于任务失败场景）
func (w *BackupWorker) publishProgressWithError(ctx context.Context, taskID uint64, percent int, status, msg string, errMsg string) {
	payload := w.buildProgressPayload(taskID, percent, status, msg, errMsg)
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = w.redis.Publish(ctx, pubSubProgressChan, string(b)).Err()
}

// doBackupSingle 单对象备份（原有语义 + 规范化后的存储语义）
func (w *BackupWorker) doBackupSingle(ctx context.Context, task *models.BackupTask) error {
	w.publishProgress(ctx, task.ID, 10, "running", "连接集群并读取资源...")

	dyn, _, dynErr := w.cluster.GetDynamicClient(task.ClusterCode)
	if dynErr != nil {
		return fmt.Errorf("get cluster client: %w", dynErr)
	}
	dynCli := &dynamicClientWrap{dyn: dyn.(interface {
		Resource(gvr schema.GroupVersionResource) namespaceableResource
	})}
	if task.TargetKind == nil || task.TargetName == nil {
		return errcode.New(errcode.InvalidArgument, "target_kind/target_name missing")
	}

	gvr, err := w.resolveGVR(ctx, dyn, *task.TargetKind)
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
		obj, err = dynCli.dyn.Resource(gvr).Namespace(ns).Get(ctx, *task.TargetName, metav1.GetOptions{})
	} else {
		obj, err = dynCli.dyn.Resource(gvr).Get(ctx, *task.TargetName, metav1.GetOptions{})
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

// doBackupNamespaceBatch 命名空间批量备份：namespaces × kind_filter 组合遍历 List，逐个写存储。
// Progress 规则：
//   5%  准备（取 payload）
//   10% 集群连接
//   40% = 10 + (done/total)*80 （逐个对象推进）
//   95% 写 DB 完成
//   100% 由 HandleTask 统一推进
func (w *BackupWorker) doBackupNamespaceBatch(ctx context.Context, task *models.BackupTask) error {
	w.publishProgress(ctx, task.ID, 5, "running", "读取批量配置...")

	// 先从 manager 拿 batch payload。为避免循环依赖，直接在 worker 里复用同一个 key 读取 Redis HASH：
	batchKey := fmt.Sprintf("backup:batch:%d", task.ID)
	raw, hgerr := w.redis.HGetAll(ctx, batchKey).Result()
	if hgerr != nil {
		return fmt.Errorf("read batch payload from redis: %w", hgerr)
	}
	var namespaces []string
	var kindFilter []string
	if nsStr, ok := raw["namespaces"]; ok && nsStr != "" {
		namespaces = strings.Split(nsStr, ",")
	}
	if kfStr, ok := raw["kind_filter"]; ok && kfStr != "" {
		kindFilter = strings.Split(kfStr, ",")
	}
	if len(namespaces) == 0 || len(kindFilter) == 0 {
		return fmt.Errorf("batch payload empty (namespaces=%d kinds=%d)", len(namespaces), len(kindFilter))
	}

	w.publishProgress(ctx, task.ID, 10, "running", "连接集群并准备遍历...")
	dyn, _, dynErr := w.cluster.GetDynamicClient(task.ClusterCode)
	if dynErr != nil {
		return fmt.Errorf("get cluster client: %w", dynErr)
	}
	dynCli := &dynamicClientWrap{dyn: dyn.(interface {
		Resource(gvr schema.GroupVersionResource) namespaceableResource
	})}
	stor, err := w.plugins.GetStorage(task.StorageType)
	if err != nil {
		return fmt.Errorf("get storage %s: %w", task.StorageType, err)
	}

	// 预解析 GVR 表，kind → GVR
	gvrMap := make(map[string]schema.GroupVersionResource, len(kindFilter))
	for _, k := range kindFilter {
		g, err := w.resolveGVR(ctx, dyn, k)
		if err != nil {
			w.log.Warnf("skip unknown kind %s: %v", k, err)
			continue
		}
		gvrMap[k] = g
	}
	if len(gvrMap) == 0 {
		return fmt.Errorf("没有合法的 kind_filter，已全部跳过")
	}

	// 收集所有 (ns, kind, name, rawYAML) 到队列；边收集边上传，避免内存堆积超大 manifest
	type item struct {
		NS, Kind, Name, YAML string
	}

	w.publishProgress(ctx, task.ID, 15, "running",
		fmt.Sprintf("发现 %d 命名空间 × %d 资源类型，开始枚举对象...", len(namespaces), len(gvrMap)))

	var (
		doneItems   int64
		totalItems  int64
		failedItems int64
		totalSize   int64
		lastStorage string
	)

	// 第一阶段：估算总对象数（只 List，不读 full object）用于精确进度
	var plan []item
	for _, ns := range namespaces {
		for kind, gvr := range gvrMap {
			var ulist *unstructured.UnstructuredList
			var lerr error
			// 区分集群级资源（Namespace）—— 没有 Namespace 层，且 ns 为 "" 时直接全局 List
			isClusterScoped := !isNamespacedKind(kind)
			if isClusterScoped {
				ulist, lerr = dynCli.dyn.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 500})
			} else {
				ulist, lerr = dynCli.dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{Limit: 500})
			}
			if lerr != nil {
				w.log.Warnf("list %s ns=%s failed: %v", kind, ns, lerr)
				failedItems++
				continue
			}
			for i := range ulist.Items {
				o := &ulist.Items[i]
				name := o.GetName()
				// 跳过 empty / 非预期
				if name == "" {
					continue
				}
				plan = append(plan, item{NS: ns, Kind: kind, Name: name})
			}
		}
	}
	totalItems = int64(len(plan))
	w.log.Infof("📋 task %d namespace_batch plan: %d objects (failedItems=%d during list)",
		task.ID, totalItems, failedItems)
	if totalItems == 0 {
		msg := "枚举结果为 0：可能所有命名空间都没有对应类型的对象"
		_ = w.db.Model(&models.BackupTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
			"error_message": &msg,
		})
		w.publishProgressWithError(ctx, task.ID, 100, string(models.BackupStatusFailed), "任务失败", msg)
		return fmt.Errorf(msg)
	}

	// 第二阶段：逐个对象 Get → YAML → Upload
	for i, it := range plan {
		gvr := gvrMap[it.Kind]
		var obj *unstructured.Unstructured
		var gerr error
		if !isNamespacedKind(it.Kind) {
			obj, gerr = dynCli.dyn.Resource(gvr).Get(ctx, it.Name, metav1.GetOptions{})
		} else {
			obj, gerr = dynCli.dyn.Resource(gvr).Namespace(it.NS).Get(ctx, it.Name, metav1.GetOptions{})
		}
		if gerr != nil {
			w.log.Warnf("get %s/%s/%s: %v", it.Kind, it.NS, it.Name, gerr)
			failedItems++
			continue
		}
		bs, merr := yaml.Marshal(obj.Object)
		if merr != nil {
			w.log.Warnf("marshal %s/%s/%s: %v", it.Kind, it.NS, it.Name, merr)
			failedItems++
			continue
		}
		key := storage.BuildKey(task.ClusterCode, storage.DateStr(time.Now()),
			fmt.Sprintf("%d-%05d", task.ID, i+1), it.Kind, it.Name)
		res, uerr := stor.Upload(ctx, key, bytes.NewReader(bs))
		if uerr != nil {
			w.log.Warnf("upload %s: %v", key, uerr)
			failedItems++
			continue
		}
		totalSize += res.SizeBytes
		lastStorage = res.StoragePath
		doneItems++
		// 进度：15..95 区间
		percent := 15 + int(float64(doneItems)/float64(totalItems)*80)
		w.publishProgress(ctx, task.ID, percent, "running",
			fmt.Sprintf("进度 %d/%d（失败 %d）: %s/%s/%s", doneItems, totalItems, failedItems, it.NS, it.Kind, it.Name))
	}

	if doneItems == 0 {
		msg := fmt.Sprintf("所有 %d 对象均备份失败，任务整体标记失败", totalItems)
		now := time.Now()
		_ = w.db.Model(&models.BackupTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
			"status":        models.BackupStatusFailed,
			"error_message": &msg,
			"object_count":  0,
			"completed_at":  &now,
		})
		return fmt.Errorf(msg)
	}

	// 写完成记录：
	//  批量备份 DB.StoragePath 存“目录前缀”（cluster/YYYYMMDD/backup_id-），
	//  让下载/恢复后续可以用批量任务子表，当前版本只保留第一条 item 的 storage_path 便于 UI 展示"已落盘"。
	now := time.Now()
	summaryMsg := fmt.Sprintf("批量完成：成功 %d，失败 %d，总对象 %d", doneItems, failedItems, totalItems)
	objectCount := int(doneItems)
	updates := map[string]interface{}{
		"status":       models.BackupStatusSuccess,
		"size_bytes":   &totalSize,
		"object_count": objectCount,
		"storage_path": lastStorage,
		"error_message": &summaryMsg,
		"completed_at": &now,
	}
	if err := w.db.Model(&models.BackupTask{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update backup result: %w", err)
	}
	w.publishProgress(ctx, task.ID, 95, "running", fmt.Sprintf("批量备份写入完成，共成功 %d / 失败 %d", doneItems, failedItems))
	return nil
}

func isNamespacedKind(kind string) bool {
	switch kind {
	case "Namespace", "Node", "PersistentVolume", "ClusterRole", "ClusterRoleBinding",
		"StorageClass", "PriorityClass", "CustomResourceDefinition", "CSIDriver", "CSINode":
		return false
	}
	return true
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

	type namespaceable = interface {
		Namespace(string) interface {
			Get(context.Context, string, metav1.GetOptions) (*unstructured.Unstructured, error)
			Create(context.Context, *unstructured.Unstructured, metav1.CreateOptions) (*unstructured.Unstructured, error)
			Update(context.Context, *unstructured.Unstructured, metav1.UpdateOptions) (*unstructured.Unstructured, error)
		}
		Get(context.Context, string, metav1.GetOptions) (*unstructured.Unstructured, error)
		Create(context.Context, *unstructured.Unstructured, metav1.CreateOptions) (*unstructured.Unstructured, error)
		Update(context.Context, *unstructured.Unstructured, metav1.UpdateOptions) (*unstructured.Unstructured, error)
	}

	_ = dynCli
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
