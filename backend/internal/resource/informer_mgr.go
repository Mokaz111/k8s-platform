package resource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/k8s-platform/console/internal/cluster"
	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type gvrKey struct {
	clusterCode string
	gvr         schema.GroupVersionResource
}

type informerEntry struct {
	factory  dynamicinformer.DynamicSharedInformerFactory
	informer informers.GenericInformer
	stopCh   chan struct{}
	started  bool
	readyMu  sync.RWMutex
	ready    bool
}

type InformerManager struct {
	clusterMgr *cluster.Manager
	cfg        *config.InformerConfig
	log        *logger.Logger

	mu        sync.RWMutex
	informers map[gvrKey]*informerEntry

	rateMu       sync.RWMutex
	rateLimiters map[string]*rate.Limiter

	listSemaphore chan struct{}
}

func NewInformerManager(clusterMgr *cluster.Manager, cfg *config.InformerConfig, log *logger.Logger) *InformerManager {
	concurrency := cfg.StartupConcurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	return &InformerManager{
		clusterMgr:    clusterMgr,
		cfg:           cfg,
		log:           log,
		informers:     make(map[gvrKey]*informerEntry),
		rateLimiters:  make(map[string]*rate.Limiter),
		listSemaphore: make(chan struct{}, concurrency),
	}
}

func (im *InformerManager) getRateLimiter(clusterCode string) *rate.Limiter {
	im.rateMu.RLock()
	rl, ok := im.rateLimiters[clusterCode]
	im.rateMu.RUnlock()
	if ok {
		return rl
	}

	im.rateMu.Lock()
	defer im.rateMu.Unlock()
	if rl, ok := im.rateLimiters[clusterCode]; ok {
		return rl
	}
	qps := im.cfg.ListQPS
	if qps <= 0 {
		qps = 5
	}
	burst := im.cfg.ListBurst
	if burst <= 0 {
		burst = 20
	}
	rl = rate.NewLimiter(rate.Limit(qps), burst)
	im.rateLimiters[clusterCode] = rl
	return rl
}

func waitForCacheSyncWithTimeout(stopCh <-chan struct{}, timeout time.Duration, syncFuncs ...cache.InformerSynced) bool {
	syncCh := make(chan struct{})
	go func() {
		cache.WaitForCacheSync(stopCh, syncFuncs...)
		close(syncCh)
	}()
	select {
	case <-stopCh:
		return false
	case <-syncCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (im *InformerManager) buildListWatch(dynCli dynamic.Interface, gvr schema.GroupVersionResource, namespace string, rl *rate.Limiter) cache.ListerWatcher {
	pageSize := im.cfg.ListPageSize
	if pageSize <= 0 {
		pageSize = 500
	}
	pageInterval := im.cfg.ListPageInterval
	if pageInterval <= 0 {
		pageInterval = 1 * time.Second
	}
	listTimeout := im.cfg.ListTimeout
	if listTimeout <= 0 {
		listTimeout = 60 * time.Second
	}

	return &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			im.listSemaphore <- struct{}{}
			defer func() { <-im.listSemaphore }()

			ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
			defer cancel()

			var allItems []unstructured.Unstructured
			continueTok := options.Continue
			limit := pageSize
			if options.Limit > 0 && options.Limit < limit {
				limit = options.Limit
			}

			for {
				if err := rl.Wait(ctx); err != nil {
					return nil, errcode.Wrap(errcode.K8SAPIRateLimited, err, "rate limiter wait")
				}

				listOpts := metav1.ListOptions{
					Limit:           limit,
					Continue:        continueTok,
					LabelSelector:   options.LabelSelector,
					FieldSelector:   options.FieldSelector,
					ResourceVersion: options.ResourceVersion,
				}

				var list *unstructured.UnstructuredList
				var listErr error
				if namespace == "" {
					list, listErr = dynCli.Resource(gvr).List(ctx, listOpts)
				} else {
					list, listErr = dynCli.Resource(gvr).Namespace(namespace).List(ctx, listOpts)
				}
				if listErr != nil {
					return nil, errcode.Wrap(errcode.K8SAPIError, listErr, fmt.Sprintf("list %s", gvr.String()))
				}

				allItems = append(allItems, list.Items...)

				continueTok = list.GetContinue()
				if continueTok == "" {
					break
				}

				select {
				case <-ctx.Done():
					return nil, errcode.Wrap(errcode.K8SAPIError, ctx.Err(), "list timeout")
				case <-time.After(pageInterval):
				}
			}

			result := &unstructured.UnstructuredList{
				Object: map[string]interface{}{
					"apiVersion": gvr.GroupVersion().String(),
					"kind":       "List",
					"metadata": map[string]interface{}{
						"resourceVersion": "0",
					},
				},
				Items: allItems,
			}
			if len(allItems) > 0 {
				rv := allItems[len(allItems)-1].GetResourceVersion()
				if rv != "" {
					result.Object["metadata"].(map[string]interface{})["resourceVersion"] = rv
				}
			}
			return result, nil
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
			defer cancel()

			if err := rl.Wait(ctx); err != nil {
				return nil, errcode.Wrap(errcode.K8SAPIRateLimited, err, "rate limiter wait")
			}

			watchOpts := metav1.ListOptions{
				ResourceVersion:     options.ResourceVersion,
				LabelSelector:       options.LabelSelector,
				FieldSelector:       options.FieldSelector,
				AllowWatchBookmarks: true,
			}

			if namespace == "" {
				return dynCli.Resource(gvr).Watch(context.Background(), watchOpts)
			}
			return dynCli.Resource(gvr).Namespace(namespace).Watch(context.Background(), watchOpts)
		},
	}
}

func gvkToGVR(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	// 使用 apimachinery 的标准猜算规则做 GVK→GVR 转换：
	// - 自动小写（API server 资源路径全是小写，如 ServiceAccount→serviceaccounts）
	// - 正确复数（NetworkPolicy→networkpolicies）
	// - 处理已复数的不规则 Kind（Endpoints→endpoints，内置 irregular 表）
	// 原实现是 Kind+"s" 兜底，对 Endpoints 会生成 "Endpointss"，
	// 对 NetworkPolicy 生成 "NetworkPolicys"，导致 apiserver 404。
	plural, _ := meta.UnsafeGuessKindToResource(gvk)
	return plural
}

func (im *InformerManager) StartInformer(clusterCode string, gvk schema.GroupVersionKind) (cache.SharedIndexInformer, error) {
	gvr := gvkToGVR(gvk)
	key := gvrKey{clusterCode: clusterCode, gvr: gvr}

	im.mu.RLock()
	entry, ok := im.informers[key]
	im.mu.RUnlock()

	if ok && entry.started {
		return entry.informer.Informer(), nil
	}

	im.mu.Lock()
	if entry, ok := im.informers[key]; ok {
		if entry.started {
			im.mu.Unlock()
			return entry.informer.Informer(), nil
		}
	} else {
		dynCli, _, err := im.clusterMgr.GetDynamicClient(clusterCode)
		if err != nil {
			im.mu.Unlock()
			return nil, err
		}

		rl := im.getRateLimiter(clusterCode)

		factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynCli, 10*time.Minute, metav1.NamespaceAll, nil)

		entry = &informerEntry{
			factory: factory,
			stopCh:  make(chan struct{}),
		}

		lw := im.buildListWatch(dynCli, gvr, "", rl)
		newInf := cache.NewSharedIndexInformer(lw, &unstructured.Unstructured{}, 10*time.Minute, cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		})
		newInf.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
			im.log.Warnf("informer watch error cluster=%s gvr=%s: %v", clusterCode, gvr.String(), err)
		})
		entry.informer = &wrappedGenericInformer{inf: newInf}

		im.informers[key] = entry
	}
	im.mu.Unlock()

	entry.readyMu.Lock()
	defer entry.readyMu.Unlock()

	if !entry.started {
		go func() {
			im.log.Infof("starting informer cluster=%s gvk=%s", clusterCode, gvk.String())
			inf := entry.informer.Informer()
			inf.Run(entry.stopCh)
		}()

		readyTimeout := im.cfg.ListTimeout + 30*time.Second
		if !waitForCacheSyncWithTimeout(entry.stopCh, readyTimeout, entry.informer.Informer().HasSynced) {
			im.log.Warnf("informer sync timeout cluster=%s gvk=%s", clusterCode, gvk.String())
		} else {
			im.log.Infof("informer synced cluster=%s gvk=%s", clusterCode, gvk.String())
		}
		entry.started = true
		entry.ready = true
	}

	return entry.informer.Informer(), nil
}

func (im *InformerManager) GetIndexer(clusterCode string, gvk schema.GroupVersionKind) (cache.Indexer, bool, error) {
	gvr := gvkToGVR(gvk)
	key := gvrKey{clusterCode: clusterCode, gvr: gvr}

	im.mu.RLock()
	entry, ok := im.informers[key]
	im.mu.RUnlock()

	if !ok {
		return nil, false, nil
	}

	entry.readyMu.RLock()
	ready := entry.ready
	entry.readyMu.RUnlock()

	return entry.informer.Informer().GetIndexer(), ready, nil
}

func (im *InformerManager) StopInformer(clusterCode string, gvk schema.GroupVersionKind) {
	gvr := gvkToGVR(gvk)
	key := gvrKey{clusterCode: clusterCode, gvr: gvr}

	im.mu.Lock()
	defer im.mu.Unlock()

	if entry, ok := im.informers[key]; ok {
		select {
		case <-entry.stopCh:
		default:
			close(entry.stopCh)
		}
		delete(im.informers, key)
		im.log.Infof("stopped informer cluster=%s gvk=%s", clusterCode, gvk.String())
	}
}

func (im *InformerManager) StopAll() {
	im.mu.Lock()
	defer im.mu.Unlock()

	for key, entry := range im.informers {
		select {
		case <-entry.stopCh:
		default:
			close(entry.stopCh)
		}
		im.log.Infof("stopped informer cluster=%s gvr=%s", key.clusterCode, key.gvr.String())
	}
	im.informers = make(map[gvrKey]*informerEntry)
}

type wrappedGenericInformer struct {
	inf cache.SharedIndexInformer
}

func (w *wrappedGenericInformer) Informer() cache.SharedIndexInformer { return w.inf }
func (w *wrappedGenericInformer) Lister() cache.GenericLister {
	return &wrappedGenericLister{indexer: w.inf.GetIndexer()}
}

type wrappedGenericLister struct {
	indexer cache.Indexer
}

func (w *wrappedGenericLister) List(selector labels.Selector) ([]runtime.Object, error) {
	var result []runtime.Object
	for _, item := range w.indexer.List() {
		if u, ok := item.(runtime.Object); ok {
			result = append(result, u)
		}
	}
	return result, nil
}

func (w *wrappedGenericLister) Get(name string) (runtime.Object, error) {
	obj, exists, err := w.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	if ro, ok := obj.(runtime.Object); ok {
		return ro, nil
	}
	return nil, fmt.Errorf("not a runtime.Object")
}

func (w *wrappedGenericLister) ByNamespace(namespace string) cache.GenericNamespaceLister {
	return &wrappedGenericNamespaceLister{indexer: w.indexer, namespace: namespace}
}

type wrappedGenericNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

func (w *wrappedGenericNamespaceLister) List(selector labels.Selector) ([]runtime.Object, error) {
	keys, err := w.indexer.IndexKeys(cache.NamespaceIndex, w.namespace)
	if err != nil {
		return nil, err
	}
	var result []runtime.Object
	for _, k := range keys {
		if obj, exists, _ := w.indexer.GetByKey(k); exists {
			if ro, ok := obj.(runtime.Object); ok {
				result = append(result, ro)
			}
		}
	}
	return result, nil
}

func (w *wrappedGenericNamespaceLister) Get(name string) (runtime.Object, error) {
	key := w.namespace + "/" + name
	obj, exists, err := w.indexer.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	if ro, ok := obj.(runtime.Object); ok {
		return ro, nil
	}
	return nil, fmt.Errorf("not a runtime.Object")
}
