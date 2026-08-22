package storage

import (
	"context"
	"io"
	"time"
)

type UploadResult struct {
	SizeBytes   int64
	StoragePath string
}

type FileMeta struct {
	Key        string
	Size       int64
	ModifiedAt time.Time
}

type IBackupStorage interface {
	StorageType() string
	Upload(ctx context.Context, key string, reader io.Reader) (*UploadResult, error)
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]FileMeta, error)
	Ping(ctx context.Context) error
}
