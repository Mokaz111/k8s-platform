package version

import (
	"encoding/json"
	"fmt"

	"github.com/k8s-platform/console/internal/models"
	"github.com/k8s-platform/console/internal/resource"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"gorm.io/gorm"
	"sigs.k8s.io/yaml"
)

type Manager struct {
	DB          *gorm.DB
	ResourceMgr *resource.Manager
	Log         *logger.Logger

	MaxVersions int
}

func NewManager(db *gorm.DB, resourceMgr *resource.Manager, log *logger.Logger) *Manager {
	return &Manager{
		DB:          db,
		ResourceMgr: resourceMgr,
		Log:         log,
		MaxVersions: 5,
	}
}

func (m *Manager) SetMaxVersions(n int) {
	if n > 0 {
		m.MaxVersions = n
	}
}

func (m *Manager) nextVersionSeq(tx *gorm.DB, clusterCode, ns, apiVersion, kind, name string) (int, error) {
	var maxSeq int
	err := tx.Model(&models.ResourceSnapshot{}).
		Where("cluster_code = ? AND namespace = ? AND api_version = ? AND kind = ? AND name = ?",
			clusterCode, ns, apiVersion, kind, name).
		Select("COALESCE(MAX(version_seq), 0)").
		Scan(&maxSeq).Error
	if err != nil {
		return 0, errcode.Wrap(errcode.DatabaseError, err)
	}
	return maxSeq + 1, nil
}

func objToYAML(obj *unstructured.Unstructured) (string, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

func (m *Manager) SnapshotBefore(clusterCode, ns, apiVersion, kind, name, operator, source, changeSummary string, operatorID uint64) (int, error) {
	if clusterCode == "" || apiVersion == "" || kind == "" || name == "" {
		return 0, errcode.New(errcode.InvalidArgument, "cluster_code, api_version, kind, name 不能为空")
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return 0, errcode.Wrap(errcode.InvalidArgument, err, "apiVersion 解析失败")
	}
	gvk := gv.WithKind(kind)

	namespaced := m.ResourceMgr.IsNamespaced(gvk)
	effectiveNS := ns
	if !namespaced {
		effectiveNS = "_cluster_"
	}

	obj, err := m.ResourceMgr.GetResource(clusterCode, gvk, ns, name)
	if err != nil {
		if ec, ok := err.(*errcode.Error); ok && ec.Code == errcode.NotFound {
			return 0, nil
		}
		return 0, err
	}

	rawYAML, err := objToYAML(obj)
	if err != nil {
		return 0, errcode.Wrap(errcode.Internal, err, "YAML 序列化失败")
	}

	tx := m.DB.Begin()
	if tx.Error != nil {
		return 0, errcode.Wrap(errcode.DatabaseError, tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	seq, err := m.nextVersionSeq(tx, clusterCode, effectiveNS, apiVersion, kind, name)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	snapshot := &models.ResourceSnapshot{
		ClusterCode:   clusterCode,
		Namespace:     effectiveNS,
		APIVersion:    apiVersion,
		Kind:          kind,
		Name:          name,
		VersionSeq:    seq,
		RawYAML:       rawYAML,
		ChangeSummary: changeSummary,
		Operator:      operator,
		Source:        source,
		OperatorID:    operatorID,
	}

	if err := tx.Create(snapshot).Error; err != nil {
		tx.Rollback()
		return 0, errcode.Wrap(errcode.DatabaseError, err, "保存快照失败")
	}

	if err := tx.Commit().Error; err != nil {
		return 0, errcode.Wrap(errcode.DatabaseError, err)
	}

	return seq, nil
}

func (m *Manager) SnapshotAfter(obj *unstructured.Unstructured, operator, source, changeSummary string, operatorID uint64, clusterCode string) error {
	if obj == nil || clusterCode == "" {
		return errcode.New(errcode.InvalidArgument, "obj 和 cluster_code 不能为空")
	}

	apiVersion := obj.GetAPIVersion()
	kind := obj.GetKind()
	name := obj.GetName()
	ns := obj.GetNamespace()

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return errcode.Wrap(errcode.InvalidArgument, err, "apiVersion 解析失败")
	}
	gvk := gv.WithKind(kind)
	namespaced := m.ResourceMgr.IsNamespaced(gvk)
	effectiveNS := ns
	if !namespaced {
		effectiveNS = "_cluster_"
	}

	rawYAML, err := objToYAML(obj)
	if err != nil {
		return errcode.Wrap(errcode.Internal, err, "YAML 序列化失败")
	}

	tx := m.DB.Begin()
	if tx.Error != nil {
		return errcode.Wrap(errcode.DatabaseError, tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	seq, err := m.nextVersionSeq(tx, clusterCode, effectiveNS, apiVersion, kind, name)
	if err != nil {
		tx.Rollback()
		return err
	}

	snapshot := &models.ResourceSnapshot{
		ClusterCode:   clusterCode,
		Namespace:     effectiveNS,
		APIVersion:    apiVersion,
		Kind:          kind,
		Name:          name,
		VersionSeq:    seq,
		RawYAML:       rawYAML,
		ChangeSummary: changeSummary,
		Operator:      operator,
		Source:        source,
		OperatorID:    operatorID,
	}

	if err := tx.Create(snapshot).Error; err != nil {
		tx.Rollback()
		return errcode.Wrap(errcode.DatabaseError, err, "保存快照失败")
	}

	keepVersion := seq - m.MaxVersions + 1
	if keepVersion > 1 {
		deleteErr := tx.Where(
			"cluster_code = ? AND namespace = ? AND api_version = ? AND kind = ? AND name = ? AND version_seq < ?",
			clusterCode, effectiveNS, apiVersion, kind, name, keepVersion,
		).Delete(&models.ResourceSnapshot{}).Error
		if deleteErr != nil {
			m.Log.Warnf("cleanup old snapshots failed: %v", deleteErr)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return errcode.Wrap(errcode.DatabaseError, err)
	}

	return nil
}

func (m *Manager) ListVersions(clusterCode, ns, apiVersion, kind, name string) ([]models.ResourceSnapshot, error) {
	if clusterCode == "" || apiVersion == "" || kind == "" || name == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster_code, api_version, kind, name 不能为空")
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "apiVersion 解析失败")
	}
	gvk := gv.WithKind(kind)
	namespaced := m.ResourceMgr.IsNamespaced(gvk)
	effectiveNS := ns
	if !namespaced {
		effectiveNS = "_cluster_"
	}

	var items []models.ResourceSnapshot
	err = m.DB.Where(
		"cluster_code = ? AND namespace = ? AND api_version = ? AND kind = ? AND name = ?",
		clusterCode, effectiveNS, apiVersion, kind, name,
	).Order("version_seq DESC").Limit(m.MaxVersions).Find(&items).Error

	if err != nil {
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	return items, nil
}

func (m *Manager) GetVersion(clusterCode, ns, apiVersion, kind, name string, seq int) (*models.ResourceSnapshot, error) {
	if clusterCode == "" || apiVersion == "" || kind == "" || name == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster_code, api_version, kind, name 不能为空")
	}

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "apiVersion 解析失败")
	}
	gvk := gv.WithKind(kind)
	namespaced := m.ResourceMgr.IsNamespaced(gvk)
	effectiveNS := ns
	if !namespaced {
		effectiveNS = "_cluster_"
	}

	var snap models.ResourceSnapshot
	err = m.DB.Where(
		"cluster_code = ? AND namespace = ? AND api_version = ? AND kind = ? AND name = ? AND version_seq = ?",
		clusterCode, effectiveNS, apiVersion, kind, name, seq,
	).First(&snap).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errcode.New(errcode.VersionNotFound)
		}
		return nil, errcode.Wrap(errcode.DatabaseError, err)
	}

	return &snap, nil
}

func (m *Manager) DiffVersions(clusterCode, ns, apiVersion, kind, name string, seqA, seqB int) (string, string, error) {
	if seqA == seqB {
		return "", "", errcode.New(errcode.InvalidArgument, "seqA 和 seqB 不能相同")
	}

	snapA, err := m.GetVersion(clusterCode, ns, apiVersion, kind, name, seqA)
	if err != nil {
		return "", "", err
	}
	snapB, err := m.GetVersion(clusterCode, ns, apiVersion, kind, name, seqB)
	if err != nil {
		return "", "", err
	}

	return snapA.RawYAML, snapB.RawYAML, nil
}

func (m *Manager) Rollback(clusterCode, ns, apiVersion, kind, name string, seq int, operator string, operatorID uint64) (*unstructured.Unstructured, error) {
	if clusterCode == "" || apiVersion == "" || kind == "" || name == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster_code, api_version, kind, name 不能为空")
	}

	snap, err := m.GetVersion(clusterCode, ns, apiVersion, kind, name, seq)
	if err != nil {
		return nil, err
	}

	_, err = m.SnapshotBefore(clusterCode, ns, apiVersion, kind, name, operator, "rollback",
		fmt.Sprintf("回滚到版本 %d", seq), operatorID)
	if err != nil {
		return nil, errcode.Wrap(errcode.VersionRollbackFail, err, "SnapshotBefore 失败")
	}

	yamlBytes := []byte(snap.RawYAML)
	patched, err := m.ResourceMgr.ApplyResource(clusterCode, yamlBytes, "apply")
	if err != nil {
		return nil, errcode.Wrap(errcode.VersionRollbackFail, err, "ServerSideApply 失败")
	}

	_ = m.SnapshotAfter(patched, operator, "rollback",
		fmt.Sprintf("回滚到版本 %d 成功", seq), operatorID, clusterCode)

	return patched, nil
}

func (m *Manager) BeforeSave(clusterCode string, gvk schema.GroupVersionKind, ns, name, operator string, operatorID uint64, changeSummary string) error {
	if clusterCode == "" || name == "" {
		return nil
	}
	_, err := m.SnapshotBefore(
		clusterCode,
		ns,
		gvk.GroupVersion().String(),
		gvk.Kind,
		name,
		operator,
		"ui",
		changeSummary,
		operatorID,
	)
	return err
}
