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

// NFSStorage 基于 NFS 已挂载目录的备份存储实现
//
// 约定：由运维/Helm 确保 server:/remote_path 已挂载到 mount_point（或 base_path 就是 mount_point）。
// 本插件不负责在代码里执行 mount 命令（生产环境不建议在业务进程里 mount），
// 仅在 Ping/Upload 时做 stat 验证并给出友好错误提示。
type NFSStorage struct {
	cfg *config.NFSBackupConfig
}

func NewNFSStorage(cfg *config.NFSBackupConfig) *NFSStorage {
	return &NFSStorage{cfg: cfg}
}

func (s *NFSStorage) StorageType() string { return "nfs" }

// baseDir 返回实际读写的本地挂载点路径（绝对路径）
func (s *NFSStorage) baseDir() string {
	if s.cfg == nil {
		return ""
	}
	// 优先使用 mount_point；未指定则回退 base_path（认为 base_path 本身就是挂载点）
	p := s.cfg.MountPoint
	if p == "" {
		p = s.cfg.BasePath
	}
	return p
}

// checkMounted 检查 baseDir 是否处于预期挂载源上（尽力而为，避免误写到本地磁盘）
// 实现按平台拆分：
//   - Unix (check_unix.go)：对比 baseDir 与 /tmp 的 dev，不同才视为"已挂载"
//   - Windows (check_windows.go)：仅做目录存在性校验（不做强挂载校验）
func (s *NFSStorage) checkMounted(baseDir string) error {
	info, err := os.Stat(baseDir)
	if err != nil {
		return fmt.Errorf("stat base_dir %s: %w", baseDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("base_dir %s 不是目录", baseDir)
	}
	// 未配置 server 源，不做强校验（允许单机/开发模式直接当本地路径用）
	if s.cfg == nil || s.cfg.Server == "" {
		return nil
	}
	// 进入平台特定强校验（由 check_unix.go / check_windows.go 提供 checkMountedStrong）
	return s.checkMountedStrong(baseDir)
}

func (s *NFSStorage) Ping(ctx context.Context) error {
	if s.cfg == nil {
		return fmt.Errorf("nfs config is nil")
	}
	baseDir := s.baseDir()
	if baseDir == "" {
		return fmt.Errorf("nfs mount_point / base_path 未配置")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("mkdir base_dir: %w", err)
	}
	if err := s.checkMounted(baseDir); err != nil {
		return err
	}
	// 读写探针：创建临时文件 → 写 → 读 → 删除
	f, err := os.CreateTemp(baseDir, ".nfs-ping-*")
	if err != nil {
		return fmt.Errorf("create ping file: %w", err)
	}
	name := f.Name()
	content := []byte(fmt.Sprintf("k8s-platform-nfs-ping-%d", time.Now().UnixNano()))
	if _, werr := f.Write(content); werr != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write ping file: %w", werr)
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close ping file: %w", cerr)
	}
	got, rerr := os.ReadFile(name)
	if rerr != nil {
		_ = os.Remove(name)
		return fmt.Errorf("read ping file: %w", rerr)
	}
	if !bytes.Equal(got, content) {
		_ = os.Remove(name)
		return fmt.Errorf("nfs ping content mismatch, 可能存在 stale NFS handle")
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove ping file: %w", err)
	}
	return nil
}

// safeJoin 做路径拼接，同时阻止".."穿越 baseDir
func safeJoin(baseDir, key string) (string, error) {
	rel := filepath.Clean(key)
	if filepath.IsAbs(rel) {
		rel, _ = filepath.Rel(string(filepath.Separator), rel)
	}
	for rel == ".." || len(rel) >= 2 && rel[:2] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("storage key %q 非法：不允许 .. 路径穿越", key)
	}
	full := filepath.Join(baseDir, rel)
	baseAbs, _ := filepath.Abs(baseDir)
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	prefix := baseAbs
	if prefix[len(prefix)-1] != filepath.Separator {
		prefix += string(filepath.Separator)
	}
	if fullAbs != baseAbs && !(len(fullAbs) > len(prefix) && fullAbs[:len(prefix)] == prefix) {
		return "", fmt.Errorf("storage key %q 解析后路径 %s 越过 base_dir", key, fullAbs)
	}
	return fullAbs, nil
}

func (s *NFSStorage) Upload(ctx context.Context, key string, reader io.Reader) (*UploadResult, error) {
	baseDir := s.baseDir()
	if baseDir == "" {
		return nil, fmt.Errorf("nfs storage not configured (empty base_dir)")
	}
	if err := s.checkMounted(baseDir); err != nil {
		return nil, err
	}
	fullPath, err := safeJoin(baseDir, key)
	if err != nil {
		return nil, err
	}
	if mkErr := os.MkdirAll(filepath.Dir(fullPath), 0755); mkErr != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(fullPath), mkErr)
	}

	data, rerr := io.ReadAll(reader)
	if rerr != nil {
		return nil, fmt.Errorf("read upload data: %w", rerr)
	}

	// 原子写：先写同目录下 .tmp 文件，写入后 rename，避免 NFS 半写文件
	tmpPath := fullPath + ".tmp-" + fmt.Sprint(time.Now().UnixNano())
	if werr := os.WriteFile(tmpPath, data, 0644); werr != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("write nfs tmp file: %w", werr)
	}
	if rerr := os.Rename(tmpPath, fullPath); rerr != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("commit nfs file (rename tmp): %w", rerr)
	}
	return &UploadResult{
		SizeBytes:   int64(len(data)),
		StoragePath: key,
	}, nil
}

func (s *NFSStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	baseDir := s.baseDir()
	if baseDir == "" {
		return nil, fmt.Errorf("nfs storage not configured")
	}
	fullPath, err := safeJoin(baseDir, key)
	if err != nil {
		return nil, err
	}
	f, oerr := os.Open(fullPath)
	if oerr != nil {
		return nil, fmt.Errorf("open nfs file %s: %w", fullPath, oerr)
	}
	return f, nil
}

func (s *NFSStorage) Delete(ctx context.Context, key string) error {
	baseDir := s.baseDir()
	if baseDir == "" {
		return fmt.Errorf("nfs storage not configured")
	}
	fullPath, err := safeJoin(baseDir, key)
	if err != nil {
		return err
	}
	if rerr := os.Remove(fullPath); rerr != nil && !os.IsNotExist(rerr) {
		return fmt.Errorf("remove nfs file %s: %w", fullPath, rerr)
	}
	return nil
}

func (s *NFSStorage) List(ctx context.Context, prefix string) ([]FileMeta, error) {
	baseDir := s.baseDir()
	if baseDir == "" {
		return nil, fmt.Errorf("nfs storage not configured")
	}
	if err := s.checkMounted(baseDir); err != nil {
		return nil, err
	}
	var results []FileMeta
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if prefix != "" {
			prefixSlash := filepath.ToSlash(prefix)
			if len(rel) < len(prefixSlash) || rel[:len(prefixSlash)] != prefixSlash {
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
		return nil, fmt.Errorf("walk nfs dir: %w", err)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].ModifiedAt.After(results[j].ModifiedAt)
	})
	return results, nil
}
