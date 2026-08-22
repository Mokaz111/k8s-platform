package resource

import (
	"context"

	"github.com/k8s-platform/console/internal/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MVP_TODO: IResourceHandler 资源处理插件接口（设计文档 Section 8.4）
// 用于扩展自定义资源的 CRUD 前置/后置处理、校验、转换逻辑
type IResourceHandler interface {
	Name() string

	Supports(apiVersion, kind string) bool

	BeforeCreate(ctx context.Context, clusterCode string, obj *unstructured.Unstructured) error
	AfterCreate(ctx context.Context, clusterCode string, obj *unstructured.Unstructured, snapshot *models.ResourceSnapshot) error

	BeforeUpdate(ctx context.Context, clusterCode string, oldObj, newObj *unstructured.Unstructured) error
	AfterUpdate(ctx context.Context, clusterCode string, oldObj, newObj *unstructured.Unstructured, snapshot *models.ResourceSnapshot) error

	BeforeDelete(ctx context.Context, clusterCode string, obj *unstructured.Unstructured) error
	AfterDelete(ctx context.Context, clusterCode string, obj *unstructured.Unstructured) error

	Validate(ctx context.Context, clusterCode string, obj *unstructured.Unstructured) error

	Mutate(ctx context.Context, clusterCode string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
}
