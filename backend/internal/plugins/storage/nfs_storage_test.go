package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/k8s-platform/console/internal/config"
)

// newTestNFSStorage 用临时目录创建一个可在本地单测运行的 NFSStorage。
// cfg.Server 留空 → checkMountedStrong 不会触发，仅做目录存在性校验。
func newTestNFSStorage(t *testing.T) (*NFSStorage, string) {
	t.Helper()
	base := t.TempDir()
	cfg := &config.NFSBackupConfig{
		BasePath:   base,
		MountPoint: base,
	}
	return NewNFSStorage(cfg), base
}

func TestNFSStorage_StorageType(t *testing.T) {
	s, _ := newTestNFSStorage(t)
	if s.StorageType() != "nfs" {
		t.Fatalf("StorageType=%q, want nfs", s.StorageType())
	}
}

func TestNFSStorage_Ping(t *testing.T) {
	t.Run("OK: 空配置 server（仅本地路径）", func(t *testing.T) {
		s, _ := newTestNFSStorage(t)
		if err := s.Ping(context.Background()); err != nil {
			t.Fatalf("Ping err=%v, want nil", err)
		}
	})

	t.Run("FAIL: base_dir 未配置", func(t *testing.T) {
		s := NewNFSStorage(&config.NFSBackupConfig{})
		if err := s.Ping(context.Background()); err == nil {
			t.Fatal("Ping for empty base_dir should fail")
		}
	})
}

func TestNFSStorage_UploadDownloadDelete(t *testing.T) {
	s, base := newTestNFSStorage(t)
	ctx := context.Background()

	const key = "backups/cluster-1/backup-42.yaml"
	payload := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: hello
data:
  foo: bar
`)

	// 1. Upload
	up, err := s.Upload(ctx, key, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Upload err=%v", err)
	}
	if up == nil {
		t.Fatal("Upload result nil")
	}
	if up.StoragePath != key {
		t.Fatalf("Upload.StoragePath=%q, want %q", up.StoragePath, key)
	}
	if up.SizeBytes != int64(len(payload)) {
		t.Fatalf("Upload.SizeBytes=%d, want %d", up.SizeBytes, len(payload))
	}
	// 真实文件存在
	fullPath := filepath.Join(base, filepath.FromSlash(key))
	if _, statErr := os.Stat(fullPath); statErr != nil {
		t.Fatalf("uploaded file not found on disk: %v", statErr)
	}
	got, rerr := os.ReadFile(fullPath)
	if rerr != nil {
		t.Fatalf("read uploaded file: %v", rerr)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("uploaded bytes mismatch")
	}

	// 2. Download：读完立即显式 Close（Windows 下句柄不释放会导致 Delete 失败）
	rc, derr := s.Download(ctx, key)
	if derr != nil {
		t.Fatalf("Download err=%v", derr)
	}
	downloaded, ierr := io.ReadAll(rc)
	if cerr := rc.Close(); cerr != nil {
		t.Fatalf("close download reader: %v", cerr)
	}
	if ierr != nil {
		t.Fatalf("read download reader: %v", ierr)
	}
	if !bytes.Equal(downloaded, payload) {
		t.Fatal("downloaded bytes mismatch")
	}

	// 3. Download 不存在的 key
	rc2, err := s.Download(ctx, "not/exist.yaml")
	if err == nil {
		_ = rc2.Close()
		t.Fatal("Download not-exist key should fail")
	}

	// 4. Delete
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete err=%v", err)
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Fatalf("after Delete, file still exists or stat err=%v", err)
	}
	// Delete 幂等（删除不存在的应不报错：按当前实现 os.Remove 对不存在文件会报错，接受这个行为）
}

// TestNFSStorage_Upload_Atomic 验证写入时先 .tmp 再 rename 的原子写特性：
// 通过拦截异常路径是困难的，这里只验证：最终目录里不应该残留任何 .tmp-* 文件。
func TestNFSStorage_Upload_NoTmpLeftover(t *testing.T) {
	s, base := newTestNFSStorage(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		key := "dir/sub/file-" + string(rune('a'+i)) + ".yaml"
		_, err := s.Upload(ctx, key, strings.NewReader("hello "+string(rune('a'+i))))
		if err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	entries, derr := os.ReadDir(filepath.Join(base, "dir", "sub"))
	if derr != nil {
		t.Fatalf("ReadDir: %v", derr)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("found leftover tmp file: %s", e.Name())
		}
	}
}

// TestNFSStorage_List 验证 List(prefix) 返回的文件元信息正确（按 key 排序）。
func TestNFSStorage_List(t *testing.T) {
	s, _ := newTestNFSStorage(t)
	ctx := context.Background()

	files := map[string]string{
		"backups/cluster-a/1.yaml": "a-one",
		"backups/cluster-a/2.yaml": "a-two",
		"backups/cluster-b/3.yaml": "b-three",
		"others/skip-me.log":       "not listed",
	}
	for k, v := range files {
		if _, err := s.Upload(ctx, k, strings.NewReader(v)); err != nil {
			t.Fatalf("upload %q: %v", k, err)
		}
	}

	meta, err := s.List(ctx, "backups/")
	if err != nil {
		t.Fatalf("List err=%v", err)
	}
	// 期望只有 backups/ 下面的 3 个文件
	if len(meta) != 3 {
		t.Fatalf("List count=%d, want 3 (got: %+v)", len(meta), meta)
	}
	keys := make([]string, 0, len(meta))
	for _, m := range meta {
		keys = append(keys, m.Key)
		if m.Size <= 0 {
			t.Fatalf("meta %q Size=%d, want >0", m.Key, m.Size)
		}
		if m.ModifiedAt.IsZero() {
			t.Fatalf("meta %q ModifiedAt zero", m.Key)
		}
	}
	sort.Strings(keys)
	expect := []string{
		"backups/cluster-a/1.yaml",
		"backups/cluster-a/2.yaml",
		"backups/cluster-b/3.yaml",
	}
	if !stringsEqual(keys, expect) {
		t.Fatalf("List keys=%v, want %v", keys, expect)
	}
}

// TestNFSStorage_PathTraversal_Reject 验证 key 带 ../ 无法逃逸 baseDir。
func TestNFSStorage_PathTraversal_Reject(t *testing.T) {
	s, base := newTestNFSStorage(t)
	ctx := context.Background()

	evilKeys := []string{
		"../escape.yaml",
		"a/../../escape.yaml",
		"a/..\\..\\escape-win.yaml",
	}
	for _, k := range evilKeys {
		_, err := s.Upload(ctx, k, strings.NewReader("nope"))
		if err == nil {
			t.Fatalf("Upload path-traversal key %q should fail", k)
		}
	}
	// 验证 baseDir 的父目录下没有产生文件
	parent := filepath.Dir(base)
	entries, derr := os.ReadDir(parent)
	if derr != nil {
		t.Fatalf("ReadDir parent: %v", derr)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yaml") || strings.Contains(e.Name(), "escape") {
			t.Fatalf("path traversal leaked file into parent: %s", e.Name())
		}
	}
}

// stringsEqual 比较两个 string slice（已排好序）
func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
