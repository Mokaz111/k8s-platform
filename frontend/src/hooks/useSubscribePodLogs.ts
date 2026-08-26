import { useEffect, useMemo } from 'react';
import { useAppSelector } from '@/app/store';
import { subscribeChannel, unsubscribeChannel } from '@/app/services/websocket';
import {
  buildPodLogChannel,
  clearPodLogs,
  PodLogLinePayload,
  selectPodLogs,
} from '@/slices/wsSlice';
import { useAppDispatch } from '@/app/store';

export interface UseSubscribePodLogsOptions {
  clusterCode: string;
  namespace: string;
  podName: string;
  /** 是否在订阅前清空对应 channel 的历史缓冲，默认 true（每次打开日志面板从零开始） */
  clearBeforeSubscribe?: boolean;
}

/**
 * useSubscribePodLogs
 *
 * 根据 clusterCode/namespace/podName 推导出 channel 名并订阅 WebSocket pod_logs 频道。
 * - 挂载时（或参数变更时）自动 subscribe；卸载自动 unsubscribe
 * - 返回从 Redux ws.podLogs[channel] 取的实时日志列表
 * - 默认先清空该 channel 的历史缓冲，确保每次打开日志面板都是干净的流
 */
export function useSubscribePodLogs(options: UseSubscribePodLogsOptions): {
  channel: string;
  lines: PodLogLinePayload[];
  lastLine?: PodLogLinePayload;
  hasEof: boolean;
} {
  const { clusterCode, namespace, podName, clearBeforeSubscribe = true } = options;
  const dispatch = useAppDispatch();

  const channel = useMemo(
    () => buildPodLogChannel(clusterCode, namespace, podName),
    [clusterCode, namespace, podName],
  );

  useEffect(() => {
    if (!channel) return;
    if (clearBeforeSubscribe) {
      dispatch(clearPodLogs(channel));
    }
    subscribeChannel(channel);
    return () => {
      unsubscribeChannel(channel);
    };
  }, [channel, clearBeforeSubscribe, dispatch]);

  const lines = useAppSelector((s) => selectPodLogs(s, channel));

  const lastLine = lines[lines.length - 1];
  const hasEof = lines.some((l) => l.eof);

  return { channel, lines, lastLine, hasEof };
}
