package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/k8s-platform/console/internal/config"
)

type S3Storage struct {
	cfg *config.S3BackupConfig
}

func NewS3Storage(cfg *config.S3BackupConfig) *S3Storage {
	return &S3Storage{cfg: cfg}
}

func (s *S3Storage) StorageType() string { return "s3" }

func (s *S3Storage) Ping(ctx context.Context) error {
	// MVP_TODO: 接入 minio-go / aws-sdk-go-v2，调用 BucketExists / HeadBucket 探测连通性
	if s.cfg == nil {
		return fmt.Errorf("s3 config is nil")
	}
	if s.cfg.Bucket == "" {
		return fmt.Errorf("s3 bucket is empty")
	}
	return fmt.Errorf("MVP_TODO: S3Storage.Ping not implemented (need minio-go / aws-sdk-go-v2)")
}

func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader) (*UploadResult, error) {
	// MVP_TODO: 接入 minio-go / aws-sdk-go-v2 的 PutObject，支持流式上传、分块上传
	_ = key
	_ = reader
	return nil, fmt.Errorf("MVP_TODO: S3Storage.Upload not implemented (need minio-go / aws-sdk-go-v2)")
}

func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// MVP_TODO: 接入 minio-go / aws-sdk-go-v2 的 GetObject，返回 io.ReadCloser
	_ = key
	return nil, fmt.Errorf("MVP_TODO: S3Storage.Download not implemented (need minio-go / aws-sdk-go-v2)")
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	// MVP_TODO: 接入 minio-go / aws-sdk-go-v2 的 RemoveObject
	_ = key
	return fmt.Errorf("MVP_TODO: S3Storage.Delete not implemented (need minio-go / aws-sdk-go-v2)")
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]FileMeta, error) {
	// MVP_TODO: 接入 minio-go / aws-sdk-go-v2 的 ListObjectsV2 / ListObjects
	_ = prefix
	return nil, fmt.Errorf("MVP_TODO: S3Storage.List not implemented (need minio-go / aws-sdk-go-v2)")
}
