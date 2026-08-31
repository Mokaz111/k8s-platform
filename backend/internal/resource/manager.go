package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

type ListResult struct {
	Total int64                        `json:"total"`
	Items []*unstructured.Unstructured `json:"items"`
}

type Manager struct {
	clusterMgr  *cluster.Manager
	informerMgr *InformerManager
	log         *logger.Logger
}

func NewManager(clusterMgr *cluster.Manager, informerMgr *InformerManager, log *logger.Logger) *Manager {
	return &Manager{
		clusterMgr:  clusterMgr,
		informerMgr: informerMgr,
		log:         log,
	}
}

var MVP_GVKs = []schema.GroupVersionKind{
	{Group: "apps", Version: "v1", Kind: "Deployment"},
	{Group: "apps", Version: "v1", Kind: "StatefulSet"},
	{Group: "apps", Version: "v1", Kind: "DaemonSet"},
	{Group: "batch", Version: "v1", Kind: "Job"},
	{Group: "batch", Version: "v1", Kind: "CronJob"},
	{Group: "", Version: "v1", Kind: "ConfigMap"},
	{Group: "", Version: "v1", Kind: "Secret"},
	{Group: "", Version: "v1", Kind: "PersistentVolume"},
	{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
	{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"},
}

func (m *Manager) IsNamespaced(gvk schema.GroupVersionKind) bool {
	switch gvk.Kind {
	// 集群级资源：走带 namespace 的 REST 路径查询会 404
	case "PersistentVolume", "StorageClass", "Namespace", "Node", "VolumeAttachment", "ClusterRole",
		"ClusterRoleBinding", "ClusterIssuer", "CustomResourceDefinition", "APIService", "PriorityClass",
		"RuntimeClass", "IngressClass", "CSIDriver", "CSINode", "MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration", "ComponentStatus":
		return false
	default:
		return true
	}
}

func (m *Manager) ListResources(clusterCode string, gvk schema.GroupVersionKind, namespace, keyword string, page, size int) (*ListResult, error) {
	if clusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}

	gvr := gvkToGVR(gvk)
	namespaced := m.IsNamespaced(gvk)
	if namespaced && namespace == "" {
		namespace = metav1.NamespaceAll
	}

	indexer, ready, _ := m.informerMgr.GetIndexer(clusterCode, gvk)
	if ready && indexer != nil {
		return m.listFromIndexer(indexer, gvk, namespace, keyword, page, size)
	}

	return m.listFromAPI(clusterCode, gvr, gvk, namespace, keyword, page, size)
}

func (m *Manager) listFromIndexer(indexer interface{}, gvk schema.GroupVersionKind, namespace, keyword string, page, size int) (*ListResult, error) {
	idx, ok := indexer.(interface{ List() []interface{} })
	if !ok {
		return nil, errcode.New(errcode.Internal, "indexer type error")
	}

	namespaced := m.IsNamespaced(gvk)
	allObjs := idx.List()
	var filtered []*unstructured.Unstructured

	for _, o := range allObjs {
		u, ok := o.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		if namespaced && namespace != "" && namespace != metav1.NamespaceAll {
			if u.GetNamespace() != namespace {
				continue
			}
		}
		if keyword != "" {
			name := u.GetName()
			if !strings.Contains(strings.ToLower(name), strings.ToLower(keyword)) {
				labels := u.GetLabels()
				matched := false
				for k, v := range labels {
					if strings.Contains(strings.ToLower(k), strings.ToLower(keyword)) ||
						strings.Contains(strings.ToLower(v), strings.ToLower(keyword)) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
		}
		filtered = append(filtered, u)
	}

	total := int64(len(filtered))
	start := (page - 1) * size
	end := start + size
	if start >= len(filtered) {
		return &ListResult{Total: total, Items: []*unstructured.Unstructured{}}, nil
	}
	if end > len(filtered) {
		end = len(filtered)
	}

	return &ListResult{Total: total, Items: filtered[start:end]}, nil
}

func (m *Manager) listFromAPI(clusterCode string, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, namespace, keyword string, page, size int) (*ListResult, error) {
	dynCli, _, err := m.clusterMgr.GetDynamicClient(clusterCode)
	if err != nil {
		return nil, err
	}

	rl := m.informerMgr.getRateLimiter(clusterCode)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := rl.Wait(ctx); err != nil {
		return nil, errcode.Wrap(errcode.K8SAPIRateLimited, err, "rate limiter wait")
	}

	namespaced := m.IsNamespaced(gvk)
	limit := int64(100)

	listOpts := metav1.ListOptions{
		Limit: limit,
	}

	var list *unstructured.UnstructuredList
	var listErr error
	if namespaced && namespace != "" && namespace != metav1.NamespaceAll {
		list, listErr = dynCli.Resource(gvr).Namespace(namespace).List(ctx, listOpts)
	} else {
		list, listErr = dynCli.Resource(gvr).List(ctx, listOpts)
	}
	if listErr != nil {
		return nil, errcode.Wrap(errcode.K8SAPIError, listErr, fmt.Sprintf("list %s", gvk.String()))
	}

	var filtered []*unstructured.Unstructured
	for i := range list.Items {
		u := &list.Items[i]
		if keyword != "" {
			name := u.GetName()
			if !strings.Contains(strings.ToLower(name), strings.ToLower(keyword)) {
				labels := u.GetLabels()
				matched := false
				for k, v := range labels {
					if strings.Contains(strings.ToLower(k), strings.ToLower(keyword)) ||
						strings.Contains(strings.ToLower(v), strings.ToLower(keyword)) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
		}
		filtered = append(filtered, u)
	}

	total := int64(len(filtered))
	start := (page - 1) * size
	end := start + size
	if start >= len(filtered) {
		return &ListResult{Total: total, Items: []*unstructured.Unstructured{}}, nil
	}
	if end > len(filtered) {
		end = len(filtered)
	}

	return &ListResult{Total: total, Items: filtered[start:end]}, nil
}

func (m *Manager) GetResource(clusterCode string, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	if clusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}
	if name == "" {
		return nil, errcode.New(errcode.InvalidArgument, "resource name 不能为空")
	}

	gvr := gvkToGVR(gvk)
	namespaced := m.IsNamespaced(gvk)

	indexer, ready, _ := m.informerMgr.GetIndexer(clusterCode, gvk)
	if ready && indexer != nil {
		key := name
		if namespaced && namespace != "" {
			key = namespace + "/" + name
		}
		if idx, ok := indexer.(interface {
			GetByKey(string) (interface{}, bool, error)
		}); ok {
			obj, exists, err := idx.GetByKey(key)
			if err == nil && exists {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					return u.DeepCopy(), nil
				}
			}
		}
	}

	dynCli, _, err := m.clusterMgr.GetDynamicClient(clusterCode)
	if err != nil {
		return nil, err
	}

	rl := m.informerMgr.getRateLimiter(clusterCode)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := rl.Wait(ctx); err != nil {
		return nil, errcode.Wrap(errcode.K8SAPIRateLimited, err, "rate limiter wait")
	}

	var obj *unstructured.Unstructured
	var getErr error
	if namespaced && namespace != "" {
		obj, getErr = dynCli.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, getErr = dynCli.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	if getErr != nil {
		if strings.Contains(getErr.Error(), "not found") {
			return nil, errcode.New(errcode.NotFound, "资源不存在")
		}
		return nil, errcode.Wrap(errcode.K8SAPIError, getErr, fmt.Sprintf("get %s/%s", gvk.String(), name))
	}

	return obj, nil
}

type ObjectIdentity struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

func NormalizeNamespace(ns string) string {
	switch strings.TrimSpace(ns) {
	case "", "_", "-", "_all":
		return ""
	default:
		return ns
	}
}

func ParseYAMLIdentity(yamlBytes []byte) (*ObjectIdentity, error) {
	if len(yamlBytes) == 0 {
		return nil, errcode.New(errcode.InvalidArgument, "yaml 内容不能为空")
	}
	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "yaml 解析失败")
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "JSON 解析失败")
	}
	id := &ObjectIdentity{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
	if id.APIVersion == "" || id.Kind == "" {
		return nil, errcode.New(errcode.InvalidArgument, "yaml 缺少 apiVersion 或 kind")
	}
	if id.Name == "" {
		return nil, errcode.New(errcode.InvalidArgument, "yaml 缺少 metadata.name")
	}
	return id, nil
}

func (id *ObjectIdentity) MatchGVK(apiVersion, kind string) error {
	if id.APIVersion != apiVersion || id.Kind != kind {
		return errcode.New(errcode.InvalidArgument, "YAML 的 apiVersion/kind 与路径不一致")
	}
	return nil
}

func (id *ObjectIdentity) MatchNamespacedName(namespace, name string) error {
	if id.Name != name {
		return errcode.New(errcode.InvalidArgument, "YAML metadata.name 与路径不一致")
	}
	if NormalizeNamespace(id.Namespace) != NormalizeNamespace(namespace) {
		return errcode.New(errcode.InvalidArgument, "YAML metadata.namespace 与路径不一致")
	}
	return nil
}

func (m *Manager) ApplyResource(clusterCode string, yamlBytes []byte, patchType string) (*unstructured.Unstructured, error) {
	if clusterCode == "" {
		return nil, errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}
	if len(yamlBytes) == 0 {
		return nil, errcode.New(errcode.InvalidArgument, "yaml 内容不能为空")
	}

	jsonBytes, err := yaml.YAMLToJSON(yamlBytes)
	if err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "yaml 解析失败")
	}

	var obj unstructured.Unstructured
	if err := json.Unmarshal(jsonBytes, &obj); err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "JSON 解析失败")
	}

	apiVersion := obj.GetAPIVersion()
	kind := obj.GetKind()
	if apiVersion == "" || kind == "" {
		return nil, errcode.New(errcode.InvalidArgument, "yaml 缺少 apiVersion 或 kind")
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, errcode.Wrap(errcode.InvalidArgument, err, "apiVersion 解析失败")
	}
	gvk := gv.WithKind(kind)
	gvr := gvkToGVR(gvk)
	namespaced := m.IsNamespaced(gvk)

	dynCli, _, err := m.clusterMgr.GetDynamicClient(clusterCode)
	if err != nil {
		return nil, err
	}

	rl := m.informerMgr.getRateLimiter(clusterCode)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := rl.Wait(ctx); err != nil {
		return nil, errcode.Wrap(errcode.K8SAPIRateLimited, err, "rate limiter wait")
	}

	force := true
	opts := metav1.PatchOptions{
		FieldManager: "k8s-console",
		Force:        &force,
	}

	var pt types.PatchType
	switch strings.ToLower(patchType) {
	case "apply", "serversideapply", "":
		pt = types.ApplyPatchType
	case "merge":
		pt = types.MergePatchType
	case "strategic":
		pt = types.StrategicMergePatchType
	case "json":
		pt = types.JSONPatchType
	default:
		pt = types.ApplyPatchType
	}

	var patched *unstructured.Unstructured
	var patchErr error
	if namespaced && namespace != "" {
		patched, patchErr = dynCli.Resource(gvr).Namespace(namespace).Patch(ctx, name, pt, jsonBytes, opts)
	} else {
		patched, patchErr = dynCli.Resource(gvr).Patch(ctx, name, pt, jsonBytes, opts)
	}
	if patchErr != nil {
		return nil, errcode.Wrap(errcode.K8SAPIError, patchErr, fmt.Sprintf("apply %s/%s", gvk.String(), name))
	}

	return patched, nil
}

func (m *Manager) DeleteResource(clusterCode string, gvk schema.GroupVersionKind, namespace, name string) error {
	if clusterCode == "" {
		return errcode.New(errcode.InvalidArgument, "cluster code 不能为空")
	}
	if name == "" {
		return errcode.New(errcode.InvalidArgument, "resource name 不能为空")
	}

	gvr := gvkToGVR(gvk)
	namespaced := m.IsNamespaced(gvk)

	dynCli, _, err := m.clusterMgr.GetDynamicClient(clusterCode)
	if err != nil {
		return err
	}

	rl := m.informerMgr.getRateLimiter(clusterCode)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := rl.Wait(ctx); err != nil {
		return errcode.Wrap(errcode.K8SAPIRateLimited, err, "rate limiter wait")
	}

	propagation := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	}

	var delErr error
	if namespaced && namespace != "" {
		delErr = dynCli.Resource(gvr).Namespace(namespace).Delete(ctx, name, deleteOpts)
	} else {
		delErr = dynCli.Resource(gvr).Delete(ctx, name, deleteOpts)
	}
	if delErr != nil {
		if strings.Contains(delErr.Error(), "not found") {
			return errcode.New(errcode.NotFound, "资源不存在")
		}
		return errcode.Wrap(errcode.K8SAPIError, delErr, fmt.Sprintf("delete %s/%s", gvk.String(), name))
	}

	return nil
}
