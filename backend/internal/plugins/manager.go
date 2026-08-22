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
		m.RegisterStorage(nfs)
		m.log.Infof("🔌 plugin storage registered: nfs (base_path=%s) [MVP_TODO]",
			m.cfg.Backup.NFS.BasePath)
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
