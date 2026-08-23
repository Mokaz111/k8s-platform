package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/k8s-platform/console/internal/config"
)

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(cfg *config.LocalBackupConfig) *LocalStorage {
	return &LocalStorage{baseDir: cfg.BaseDir}
}

func (s *LocalStorage) StorageType() string { return "local" }

func (s *LocalStorage) Ping(ctx context.Context) error {
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("mkdir base_dir: %w", err)
	}
	f, err := os.CreateTemp(s.baseDir, ".ping-*")
	if err != nil {
		return fmt.Errorf("create ping file: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader) (*UploadResult, error) {
	fullPath, err := safeJoin(s.baseDir, key)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	var buf bytes.Buffer
	tee := io.TeeReader(reader, &buf)

	data, err := io.ReadAll(tee)
	if err != nil {
		return nil, fmt.Errorf("read upload data: %w", err)
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write file %s: %w", fullPath, err)
	}

	// 统一语义：StoragePath 存储相对 key（不再是绝对路径），与 NFS/S3 保持一致
	return &UploadResult{
		SizeBytes:   int64(len(data)),
		StoragePath: key,
	}, nil
}

func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	var fullPath string
	if filepath.IsAbs(key) {
		fullPath = key
	} else {
		fullPath = filepath.Join(s.baseDir, key)
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", fullPath, err)
	}
	return f, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	var fullPath string
	if filepath.IsAbs(key) {
		fullPath = key
	} else {
		fullPath = filepath.Join(s.baseDir, key)
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file %s: %w", fullPath, err)
	}
	return nil
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]FileMeta, error) {
	prefixDir := filepath.Join(s.baseDir, prefix)
	var results []FileMeta

	err := filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.baseDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if prefix != "" {
			prefixSlash := filepath.ToSlash(prefix)
			if len(rel) < len(prefixSlash) || rel[:len(prefixSlash)] != prefixSlash {
				_ = prefixDir
				return nil
			}
		}
		results = append(results, FileMeta{
			Key:        rel,
			Size:       info.Size(),
			ModifiedAt: info.ModTime(),
		})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("walk dir: %w", err)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].ModifiedAt.After(results[j].ModifiedAt)
	})
	return results, nil
}

func BuildKey(clusterCode, dateStr, backupID, kind, name string) string {
	return fmt.Sprintf("%s/%s/%s_%s_%s.yaml", clusterCode, dateStr, backupID, kind, sanitizeName(name))
}

func sanitizeName(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			b = append(b, c)
		} else {
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "unnamed"
	}
	return string(b)
}

func DateStr(t time.Time) string {
	return t.Format("20060102")
}
