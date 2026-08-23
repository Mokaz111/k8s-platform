//go:build windows
// +build windows

package storage

// Windows 下不做 NFS 挂载有效性强校验（syscall.Stat_t 不可用，也极少把生产 NFS 挂到 Windows）
// 仅在配置了 server 时打印一条提示；开发者模式下只按目录是否存在判断
func (s *NFSStorage) checkMountedStrong(baseDir string) error {
	return nil
}
