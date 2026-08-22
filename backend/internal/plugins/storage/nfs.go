package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/k8s-platform/console/internal/config"
)

type NFSStorage struct {
	cfg *config.NFSBackupConfig
}

func NewNFSStorage(cfg *config.NFSBackupConfig) *NFSStorage {
	return &NFSStorage{cfg: cfg}
}

func (s *NFSStorage) StorageType() string { return "nfs" }

func (s *NFSStorage) Ping(ctx context.Context) error {
	// MVP_TODO: 检查 NFS 挂载点是否可用（stat base_path + touch test file）
	// 可复用 LocalStorage 逻辑，但 baseDir 指向 NFS 挂载路径
	if s.cfg == nil {
		return fmt.Errorf("nfs config is nil")
	}
	if s.cfg.BasePath == "" {
		return fmt.Errorf("nfs base_path is empty")
	}
	return fmt.Errorf("MVP_TODO: NFSStorage.Ping not implemented (need mount check + read/write probe)")
}

func (s *NFSStorage) Upload(ctx context.Context, key string, reader io.Reader) (*UploadResult, error) {
	// MVP_TODO: 基于 cfg.BasePath 复用 LocalStorage 的文件操作逻辑
	// 注意：NFS 挂载需确保目录权限、锁、原子写（先写临时文件再 rename）
	_ = key
	_ = reader
	return nil, fmt.Errorf("MVP_TODO: NFSStorage.Upload not implemented")
}

func (s *NFSStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// MVP_TODO: 复用 LocalStorage.Download，基于 cfg.BasePath
	_ = key
	return nil, fmt.Errorf("MVP_TODO: NFSStorage.Download not implemented")
}

func (s *NFSStorage) Delete(ctx context.Context, key string) error {
	// MVP_TODO: 复用 LocalStorage.Delete，基于 cfg.BasePath
	_ = key
	return fmt.Errorf("MVP_TODO: NFSStorage.Delete not implemented")
}

func (s *NFSStorage) List(ctx context.Context, prefix string) ([]FileMeta, error) {
	// MVP_TODO: 复用 LocalStorage.List，基于 cfg.BasePath
	_ = prefix
	return nil, fmt.Errorf("MVP_TODO: NFSStorage.List not implemented")
}
