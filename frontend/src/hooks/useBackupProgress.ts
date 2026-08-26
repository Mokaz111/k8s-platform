import { useEffect, useMemo } from 'react';
import { useAppDispatch } from '@/app/store';
import { clearTaskProgress, TaskProgressPayload } from '@/slices/wsSlice';
import { useSubscribeTaskProgress } from './useSubscribeTaskProgress';

export interface UseBackupProgressOptions {
  /** 按集群 code 过滤进度事件；undefined = 不过滤 */
  clusterCode?: string;
  /** 组件挂载时先清空历史进度环形缓冲，默认 false */
  clearOnMount?: boolean;
  /** 只追踪特定 backup_id / task_id 的进度 */
  backupId?: string | number;
  /**
   * 聚合策略：
   * - 'per-stage'（默认）：返回按 task_id 去重后的所有阶段事件（按时间升序，便于 Timeline）
   * - 'latest-per-task'：按 task_id 分组取最新一条（任务墙式展示）
   */
  mode?: 'per-stage' | 'latest-per-task';
}

/**
 * useBackupProgress — 面向备份/恢复场景的进度聚合 hook。
 *
 * 在 useSubscribeTaskProgress 基础上额外提供：
 *  - clusterCode / backupId 过滤
 *  - 按 task 聚合最新事件
 *  - clearOnMount 控制首次进入是否清空历史
 */
export function useBackupProgress(options: UseBackupProgressOptions = {}): {
  list: TaskProgressPayload[];
  /** key 为 task_id，value 为该任务最新事件 */
  byTaskId: Record<string, TaskProgressPayload>;
} {
  const { clusterCode, clearOnMount = false, backupId, mode = 'per-stage' } = options;
  const dispatch = useAppDispatch();
  const { all } = useSubscribeTaskProgress(backupId);

  useEffect(() => {
    if (clearOnMount) {
      dispatch(clearTaskProgress());
    }
  }, [clearOnMount, dispatch]);

  const filtered = useMemo(() => {
    if (!clusterCode) return all;
    return all.filter((p) => !p.cluster_code || p.cluster_code === clusterCode);
  }, [all, clusterCode]);

  const byTaskId = useMemo(() => {
    const map: Record<string, TaskProgressPayload> = {};
    filtered.forEach((p) => {
      const id = String(p.task_id ?? p.backup_id);
      if (!id) return;
      // 后出现覆盖先出现（ws taskProgress 环形缓冲按入队顺序，后面 = 更新）
      map[id] = p;
    });
    return map;
  }, [filtered]);

  const list = useMemo(() => {
    if (mode === 'latest-per-task') {
      return Object.values(byTaskId).sort((a, b) =>
        (a.timestamp || '').localeCompare(b.timestamp || ''),
      );
    }
    // per-stage：原始时间升序
    return [...filtered].sort((a, b) =>
      (a.timestamp || '').localeCompare(b.timestamp || ''),
    );
  }, [mode, byTaskId, filtered]);

  return { list, byTaskId };
}
