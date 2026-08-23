//go:build !windows
// +build !windows

package storage

import (
	"fmt"
	"os"
	"syscall"
)

// checkMountedStrong 通过比较 baseDir 与 /tmp 的设备号区分"是否真挂载了 NFS"：
// 若配置了 server，但 baseDir 与 /tmp 在同一设备 → 判定为漏挂（防止误写入本地磁盘）
func (s *NFSStorage) checkMountedStrong(baseDir string) error {
	baseStat, statErr := os.Stat(baseDir)
	tmpStat, tmpErr := os.Stat("/tmp")
	if statErr != nil || tmpErr != nil {
		// 无法判断（/tmp 在某些容器里不存在），放行
		return nil
	}
	bs, ok := baseStat.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	ts, ok := tmpStat.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if bs.Dev == ts.Dev {
		return fmt.Errorf(
			"NFS 挂载校验失败：%s 与 /tmp 位于同一设备(dev=%d)，疑似 server=%s 未挂载，拒绝写入",
			baseDir, bs.Dev, s.cfg.Server,
		)
	}
	return nil
}
