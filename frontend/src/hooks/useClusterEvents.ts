import { useEffect, useMemo } from 'react';
import { useAppSelector, useAppDispatch } from '@/app/store';
import { subscribeChannel, unsubscribeChannel } from '@/app/services/websocket';
import {
  clearClusterEvents,
  ClusterEventPayload,
  selectClusterEvents,
  WS_TYPE_CLUSTER_EVENT,
} from '@/slices/wsSlice';

export interface UseClusterEventsOptions {
  clusterCode?: string;
  namespace?: string;
  kind?: string;
  // 自动清除旧消息（首次挂载时），默认 true
  clearOnMount?: boolean;
  enabled?: boolean;
}

export interface UseClusterEventsResult {
  list: ClusterEventPayload[];
  latest?: ClusterEventPayload;
  // 严重度过滤后的子集
  warnings: ClusterEventPayload[];
  errors: ClusterEventPayload[];
}

/**
 * useClusterEvents - 订阅集群资源事件通道
 *
 * 后端 channel: "cluster_event"
 * 后端 type: "cluster_event"
 * 后端 Informer 推送 ClusterEventPayload
 */
export function useClusterEvents(
  opts: UseClusterEventsOptions = {},
): UseClusterEventsResult {
  const dispatch = useAppDispatch();
  const { clusterCode, namespace, kind, clearOnMount = true, enabled = true } = opts;

  useEffect(() => {
    if (!enabled) return;
    if (clearOnMount) dispatch(clearClusterEvents());
    subscribeChannel(WS_TYPE_CLUSTER_EVENT);
    return () => {
      unsubscribeChannel(WS_TYPE_CLUSTER_EVENT);
    };
  }, [enabled, clearOnMount, dispatch]);

  const all = useAppSelector(selectClusterEvents);

  const filtered = useMemo(() => {
    return all.filter((e) => {
      if (clusterCode && e.cluster_code !== clusterCode) return false;
      if (namespace && e.namespace !== namespace) return false;
      if (kind && e.kind !== kind) return false;
      return true;
    });
  }, [all, clusterCode, namespace, kind]);

  const warnings = useMemo(
    () => filtered.filter((e) => e.severity === 'warning'),
    [filtered],
  );
  const errors = useMemo(
    () => filtered.filter((e) => e.severity === 'error'),
    [filtered],
  );

  return {
    list: filtered,
    latest: filtered[filtered.length - 1],
    warnings,
    errors,
  };
}
