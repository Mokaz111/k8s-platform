import { useEffect, useMemo } from 'react';
import { useAppSelector } from '@/app/store';
import { subscribeChannel, unsubscribeChannel } from '@/app/services/websocket';
import {
  selectTaskProgress,
  TaskProgressPayload,
  WS_TYPE_TASK_PROGRESS,
} from '@/slices/wsSlice';

/**
 * useSubscribeTaskProgress
 *
 * 订阅 WebSocket task_progress 频道，并返回完整或按 backup_id 过滤的进度事件流。
 *
 * - 组件挂载时自动 subscribe；卸载时自动 unsubscribe（即使多组件重复订阅，
 *   Hub 端也会去重，而 websocket.ts 内部 addSubscription 已做集合去重）。
 * - 当 filterBackupId 为 undefined 时：返回全量 taskProgress 环形缓冲（用于全局 TaskCenter）。
 * - 当 filterBackupId 为 string 时：只返回匹配该 backup_id/task_id 的最新进度，
 *   便于备份列表页按行更新 Progress 组件。
 */
export function useSubscribeTaskProgress(
  filterBackupId?: string | number,
): {
  all: TaskProgressPayload[];
  latest: TaskProgressPayload | undefined;
} {
  // 始终订阅全局 task_progress 频道：首次进入任一使用此 hook 的组件即建立订阅
  useEffect(() => {
    subscribeChannel(WS_TYPE_TASK_PROGRESS);
    return () => {
      unsubscribeChannel(WS_TYPE_TASK_PROGRESS);
    };
  }, []);

  const all = useAppSelector(selectTaskProgress);

  return useMemo(() => {
    if (filterBackupId === undefined || filterBackupId === null) {
      return { all, latest: all[all.length - 1] };
    }
    const id = String(filterBackupId);
    const matched = all.filter(
      (p) => String(p.task_id) === id || String(p.backup_id) === id,
    );
    return { all: matched, latest: matched[matched.length - 1] };
  }, [all, filterBackupId]);
}
