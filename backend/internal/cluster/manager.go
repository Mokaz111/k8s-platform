package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/pkg/crypto"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/kubeconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"gorm.io/gorm"
)

type PingResult struct {
	ServerVersion string        `json:"server_version"`
	NodeCount     int           `json:"node_count"`
	CostMs        int64         `json:"cost_ms"`
}

type ListResult struct {
	Total int64            `json:"total"`
	Items []models.Cluster `json:"items"`
}

type clientEntry struct {
	restCfg *rest.Config
	dynCli  dynamic.Interface
}

type Manager struct {
	DB  *gorm.DB
	KMS *crypto.AESGCM
	Cfg *config.Config

	mu      sync.RWMutex
	clients map[string]*clientEntry
}

func NewManager(db *gorm.DB, kms *crypto.AESGCM, cfg *config.Config) *Manager {
	return &Manager{
		DB:      db,
		KMS:     kms,
		Cfg:     cfg,
		clients: make(map[string]*clientEntry),
	}
}

func (m *Manager) ImportCluster(name, code string, rawKubeconfig []byte, creatorID uint64) (*models.Cluster, error) {
	if code == "" || name == "" {
		return nil, errcode.New(errcode.InvalidArgument, "name 和 code 不能为空")
	}
	if len(rawKubeconfig) == 0 {
		return nil, errcode.New(errcode.InvalidArgument, "kubeconfig 不能为空")
	}

	var existing int64
	if err := m.DB.Model(&models.Cluster{}).Where("code = ?", code).Count(&existing).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	if existing > 0 {
		return nil, errcode.New(errcode.AlreadyExists, fmt.Sprintf("集群 code %q 已存在", code))
	}
	var existingName int64
	if err := m.DB.Model(&models.Cluster{}).Where("name = ?", name).Count(&existingName).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	if existingName > 0 {
		return nil, errcode.New(errcode.AlreadyExists, fmt.Sprintf("集群 name %q 已存在", name))
	}

	parsed, err := kubeconfig.Parse(rawKubeconfig)
	if err != nil {
		return nil, err
	}

	aad := []byte(code + fmt.Sprint(creatorID))
	ciphertext, err := m.KMS.Encrypt(rawKubeconfig, aad)
	if err != nil {
		crypto.ZeroBuffer(aad)
		return nil, errcode.Wrap(errcode.Internal, err, "kubeconfig 加密失败")
	}
	crypto.ZeroBuffer(aad)

	cluster := &models.Cluster{
		Name:       name,
		Code:       code,
		CreatorID:  creatorID,
		Kubeconfig: ciphertext,
		APIServer:  parsed.APIServer,
		Status:     models.ClusterStatusOffline,
		Version:    "",
		NodeCount:  0,
		Labels:     models.JSONB(`{}`),
	}

	if err := m.DB.Create(cluster).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err, "保存集群记录失败")
	}

	cluster.Kubeconfig = nil
	return cluster, nil
}

func (m *Manager) Ping(clusterCode string) (*PingResult, error) {
	if clusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}

	restCfg, plainCfg, err := m.decryptAndBuildConfig(clusterCode)
	if err != nil {
		return nil, err
	}
	if restCfg == nil {
		return nil, errcode.New(errcode.NotFound, "集群不存在")
	}
	defer crypto.ZeroBuffer(plainCfg)

	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, errcode.Wrap(errcode.ClusterConnectFail, err, "创建 kubernetes client 失败")
	}

	versionInfo, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, errcode.Wrap(errcode.ClusterConnectFail, err, "调用 /version 失败")
	}

	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 5000})
	if err != nil {
		return nil, errcode.Wrap(errcode.ClusterConnectFail, err, "获取节点列表失败")
	}
	nodeCount := len(nodeList.Items)

	cost := time.Since(start)
	serverVer := versionInfo.GitVersion
	if serverVer == "" {
		serverVer = fmt.Sprintf("v%s.%s", versionInfo.Major, versionInfo.Minor)
	}

	now := time.Now()
	_ = m.DB.Model(&models.Cluster{}).Where("code = ?", clusterCode).Updates(map[string]interface{}{
		"status":       models.ClusterStatusOnline,
		"version":      serverVer,
		"node_count":   nodeCount,
		"last_sync_at": now,
	}).Error

	return &PingResult{
		ServerVersion: serverVer,
		NodeCount:     nodeCount,
		CostMs:        cost.Milliseconds(),
	}, nil
}

func (m *Manager) GetByCode(clusterCode string) (*models.Cluster, error) {
	if clusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}
	var c models.Cluster
	if err := m.DB.Where("code = ?", clusterCode).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.NotFound, "集群不存在")
		}
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	c.Kubeconfig = nil
	return &c, nil
}

func (m *Manager) List(page, size int, keyword string) (*ListResult, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}

	query := m.DB.Model(&models.Cluster{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		query = query.Where("name ILIKE ? OR code ILIKE ? OR COALESCE(description,'') ILIKE ?", kw, kw, kw)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	var items []models.Cluster
	offset := (page - 1) * size
	if err := query.Order("id DESC").Offset(offset).Limit(size).Find(&items).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	for i := range items {
		items[i].Kubeconfig = nil
	}

	return &ListResult{Total: total, Items: items}, nil
}

type UpdateInput struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	NewRawKubeconfig []byte  `json:"-"`
	LabelsJSON       *[]byte `json:"-"`
}

func (m *Manager) Update(clusterCode string, in *UpdateInput, operatorID uint64) (*models.Cluster, error) {
	if clusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}
	if in == nil {
		return nil, errcode.New(errcode.InvalidArgument, "更新内容为空")
	}

	var c models.Cluster
	if err := m.DB.Where("code = ?", clusterCode).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.NotFound, "集群不存在")
		}
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	updates := make(map[string]interface{})

	if in.Name != nil && *in.Name != "" {
		var dup int64
		if err := m.DB.Model(&models.Cluster{}).Where("name = ? AND code != ?", *in.Name, clusterCode).Count(&dup).Error; err != nil {
			return nil, errcode.Wrap(errcode.DatabaseError, err)
		}
		if dup > 0 {
			return nil, errcode.New(errcode.AlreadyExists, fmt.Sprintf("集群 name %q 已被占用", *in.Name))
		}
		updates["name"] = *in.Name
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if in.LabelsJSON != nil {
		var j models.JSONB
		if err := json.Unmarshal(*in.LabelsJSON, &j); err == nil {
			updates["labels"] = j
		}
	}

	if len(in.NewRawKubeconfig) > 0 {
		parsed, err := kubeconfig.Parse(in.NewRawKubeconfig)
		if err != nil {
			return nil, err
		}

		aad := []byte(clusterCode + fmt.Sprint(operatorID))
		ciphertext, err := m.KMS.Encrypt(in.NewRawKubeconfig, aad)
		if err != nil {
			crypto.ZeroBuffer(aad)
			return nil, errcode.Wrap(errcode.Internal, err, "kubeconfig 加密失败")
		}
		crypto.ZeroBuffer(aad)

		updates["kubeconfig"] = ciphertext
		updates["api_server"] = parsed.APIServer
		updates["creator_id"] = operatorID
	}

	if len(updates) == 0 {
		return nil, errcode.New(errcode.InvalidArgument, "没有任何需要更新的字段")
	}

	if err := m.DB.Model(&models.Cluster{}).Where("code = ?", clusterCode).Updates(updates).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	if len(in.NewRawKubeconfig) > 0 {
		m.invalidateClient(clusterCode)
	}

	var updated models.Cluster
	if err := m.DB.Where("code = ?", clusterCode).First(&updated).Error; err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	updated.Kubeconfig = nil
	return &updated, nil
}

func (m *Manager) Delete(clusterCode string) error {
	if clusterCode == "" {
		return errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}

	res := m.DB.Where("code = ?", clusterCode).Delete(&models.Cluster{})
	if res.Error != nil {
		return errcode.Wrap(errcode.DatabaseError, res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.NotFound, "集群不存在")
	}

	m.invalidateClient(clusterCode)
	return nil
}

func (m *Manager) GetDynamicClient(clusterCode string) (dynamic.Interface, *rest.Config, error) {
	if clusterCode == "" {
		return nil, nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}

	m.mu.RLock()
	if e, ok := m.clients[clusterCode]; ok && e.dynCli != nil {
		m.mu.RUnlock()
		return e.dynCli, e.restCfg, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.clients[clusterCode]; ok && e.dynCli != nil {
		return e.dynCli, e.restCfg, nil
	}

	restCfg, plain, err := m.decryptAndBuildConfig(clusterCode)
	if err != nil {
		return nil, nil, err
	}
	defer crypto.ZeroBuffer(plain)

	dynCli, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, errcode.Wrap(errcode.ClusterConnectFail, err, "创建 dynamic client 失败")
	}

	m.clients[clusterCode] = &clientEntry{restCfg: restCfg, dynCli: dynCli}
	return dynCli, restCfg, nil
}

func (m *Manager) invalidateClient(clusterCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, clusterCode)
}

func (m *Manager) decryptAndBuildConfig(clusterCode string) (*rest.Config, []byte, error) {
	var c models.Cluster
	if err := m.DB.Select("code", "kubeconfig", "creator_id").Where("code = ?", clusterCode).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, errcode.New(errcode.NotFound, "集群不存在")
		}
		return nil, nil, errcode.Wrap(errcode.DatabaseError, err)
	}
	if len(c.Kubeconfig) == 0 {
		return nil, nil, errcode.New(errcode.KubeconfigInvalid, "集群无已存储 kubeconfig")
	}

	aad := []byte(c.Code + fmt.Sprint(c.CreatorID))
	plain, err := m.KMS.Decrypt(c.Kubeconfig, aad)
	crypto.ZeroBuffer(aad)
	if err != nil {
		return nil, nil, err
	}

	parsed, err := kubeconfig.Parse(plain)
	if err != nil {
		crypto.ZeroBuffer(plain)
		return nil, nil, err
	}

	return parsed.RestConfig, plain, nil
}
