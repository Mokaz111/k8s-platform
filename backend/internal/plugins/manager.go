package plugins

import (
	"context"
	"fmt"
	"sync"

	"github.com/k8s-platform/console/internal/config"
	"github.com/k8s-platform/console/internal/plugins/storage"
	"github.com/k8s-platform/console/pkg/errcode"
	"github.com/k8s-platform/console/pkg/logger"
)

type Manager struct {
	cfg *config.Config
	log *logger.Logger

	mu            sync.RWMutex
	storageMap    map[string]storage.IBackupStorage
	defaultStorage string
}

func NewManager(cfg *config.Config, log *logger.Logger) *Manager {
	return &Manager{
		cfg:            cfg,
		log:            log,
		storageMap:     make(map[string]storage.IBackupStorage),
		defaultStorage: cfg.Backup.DefaultStorage,
	}
}

func (m *Manager) Init(ctx context.Context) error {
	local := storage.NewLocalStorage(&m.cfg.Backup.Local)
	if err := local.Ping(ctx); err != nil {
		m.log.Warnf("local storage ping failed: %v (continuing anyway)", err)
	}
	m.RegisterStorage(local)
	m.log.Infof("🔌 plugin storage registered: local (base_dir=%s)", m.cfg.Backup.Local.BaseDir)

	if m.cfg.Backup.S3 != nil {
		s3 := storage.NewS3Storage(m.cfg.Backup.S3)
		m.RegisterStorage(s3)
		m.log.Infof("🔌 plugin storage registered: s3 (bucket=%s, endpoint=%s) [MVP_TODO]",
			m.cfg.Backup.S3.Bucket, m.cfg.Backup.S3.Endpoint)
	}

	if m.cfg.Backup.NFS != nil {
		nfs := storage.NewNFSStorage(m.cfg.Backup.NFS)
		if pingErr := nfs.Ping(ctx); pingErr != nil {
			// NFS 未挂载时给出警告，依然注册该类型（后续在创建备份时会再次失败，错误更明确）
			m.log.Warnf("nfs storage ping failed: %v (registered anyway, will fail when used)", pingErr)
		} else {
			m.log.Infof("nfs storage ping ok (server=%s mount_point=%s)",
				m.cfg.Backup.NFS.Server, m.cfg.Backup.NFS.MountPoint)
		}
		m.RegisterStorage(nfs)
		mount := m.cfg.Backup.NFS.MountPoint
		if mount == "" {
			mount = m.cfg.Backup.NFS.BasePath
		}
		m.log.Infof("🔌 plugin storage registered: nfs (server=%s mount=%s)",
			m.cfg.Backup.NFS.Server, mount)
	}

	if _, ok := m.storageMap[m.defaultStorage]; !ok {
		m.defaultStorage = "local"
	}
	return nil
}

func (m *Manager) RegisterStorage(s storage.IBackupStorage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storageMap[s.StorageType()] = s
}

func (m *Manager) GetStorage(typ string) (storage.IBackupStorage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if typ == "" {
		typ = m.defaultStorage
	}
	s, ok := m.storageMap[typ]
	if !ok {
		return nil, errcode.New(errcode.PluginNotLoaded,
			fmt.Sprintf("backup storage type %q not registered", typ))
	}
	return s, nil
}

func (m *Manager) DefaultStorageType() string {
	return m.defaultStorage
}
