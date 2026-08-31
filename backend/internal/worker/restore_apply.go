package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/k8s-platform/console/internal/backup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

func stripForCreate(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "status")
}

func uniqueRestoredName(original, yyyymmdd string, exists func(string) bool) string {
	base := original + "-restored-" + yyyymmdd
	if !exists(base) {
		return base
	}
	for i := 2; i < 100; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !exists(cand) {
			return cand
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func applyRestoredObject(
	ctx context.Context,
	dynCli dynamic.Interface,
	gvr schema.GroupVersionResource,
	ns string,
	obj *unstructured.Unstructured,
	mode backup.RestoreMode,
) error {
	get := func(name string) (*unstructured.Unstructured, error) {
		if ns != "" {
			return dynCli.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		}
		return dynCli.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}
	create := func(o *unstructured.Unstructured) error {
		stripForCreate(o)
		var err error
		if ns != "" {
			_, err = dynCli.Resource(gvr).Namespace(ns).Create(ctx, o, metav1.CreateOptions{})
		} else {
			_, err = dynCli.Resource(gvr).Create(ctx, o, metav1.CreateOptions{})
		}
		return err
	}
	update := func(o *unstructured.Unstructured) error {
		var err error
		if ns != "" {
			_, err = dynCli.Resource(gvr).Namespace(ns).Update(ctx, o, metav1.UpdateOptions{})
		} else {
			_, err = dynCli.Resource(gvr).Update(ctx, o, metav1.UpdateOptions{})
		}
		return err
	}

	existing, getErr := get(obj.GetName())
	exists := getErr == nil && existing != nil

	switch mode {
	case backup.RestoreModeCreateNew:
		if exists {
			name := uniqueRestoredName(obj.GetName(), time.Now().Format("20060102"), func(n string) bool {
				got, err := get(n)
				return err == nil && got != nil
			})
			obj.SetName(name)
		}
		return create(obj)
	default:
		if exists {
			obj.SetResourceVersion(existing.GetResourceVersion())
			return update(obj)
		}
		return create(obj)
	}
}
