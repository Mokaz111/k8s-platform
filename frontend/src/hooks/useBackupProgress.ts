import { useEffect, useMemo, useRef } from 'react';
import { useAppSelector, useAppDispatch } from '@/app/store';
import { subscribeChannel, unsubscribeChannel } from '@/app/services/websocket';
import {
  selectTaskProgress,
  TaskProgressPayload,
  WS_TYPE_TASK_PROGRESS,
  clearTaskProgress,
} from '@/slices/wsSlice';

/**
 * useBackupProgress - 订阅备份/恢复任务进度通道
 *
 * 后端推送 channel: "task_progress"
 * 后端推送 type: "task_progress"
 *
 * 可选按 backupId / clusterCode 过滤
 */
export interface UseBackupProgressOptions {
  backupId?: string;
  clusterCode?: string;
  // 是否自动清除旧消息（首次挂载时），默认 true
  clearOnMount?: boolean;
  enabled?: boolean;
}

export interface UseBackupProgressResult {
  list: TaskProgressPayload[];
  latest?: TaskProgressPayload;
  latestByTask: Record<string, TaskProgressPayload>;
  // 一条特定 backup 的最新进度（仅当 backupId 传入时有效）
  latestByBackup: Record<string, TaskProgressPayload>;
}

export function useBackupProgress(
  opts: UseBackupProgressOptions = {},
): UseBackupProgressResult {
  const dispatch = useAppDispatch();
  const { backupId, clusterCode, clearOnMount = true, enabled = true } = opts;
  const mountedRef = useRef(false);

  useEffect(() => {
    if (!enabled) return;
    mountedRef.current = true;
    if (clearOnMount) dispatch(clearTaskProgress());
    subscribeChannel(WS_TYPE_TASK_PROGRESS);
    return () => {
      unsubscribeChannel(WS_TYPE_TASK_PROGRESS);
      mountedRef.current = false;
    };
  }, [enabled, clearOnMount, dispatch]);

  const all = useAppSelector(selectTaskProgress);

  const filtered = useMemo(() => {
    return all.filter((p) => {
      if (backupId && p.backup_id !== backupId) return false;
      if (clusterCode && p.cluster_code !== clusterCode) return false;
      return true;
    });
  }, [all, backupId, clusterCode]);

  const latestByTask = useMemo(() => {
    const map: Record<string, TaskProgressPayload> = {};
    for (const p of filtered) {
      const id = p.task_id;
      if (!id) continue;
      const prev = map[id];
      if (!prev || (p.timestamp || '') > (prev.timestamp || '')) {
        map[id] = p;
      }
    }
    return map;
  }, [filtered]);

  const latestByBackup = useMemo(() => {
    const map: Record<string, TaskProgressPayload> = {};
    for (const p of filtered) {
      const id = p.backup_id;
      if (!id) continue;
      const prev = map[id];
      if (!prev || (p.timestamp || '') > (prev.timestamp || '')) {
        map[id] = p;
      }
    }
    return map;
  }, [filtered]);

  return {
    list: filtered,
    latest: filtered[filtered.length - 1],
    latestByTask,
    latestByBackup,
  };
}
