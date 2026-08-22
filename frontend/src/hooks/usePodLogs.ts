import { useEffect, useMemo, useRef } from 'react';
import { useAppSelector, useAppDispatch } from '@/app/store';
import { subscribeChannel, unsubscribeChannel } from '@/app/services/websocket';
import {
  buildPodLogChannel,
  clearPodLogs,
  PodLogLinePayload,
  selectPodLogs,
  WS_TYPE_POD_LOGS,
} from '@/slices/wsSlice';

export interface UsePodLogsOptions {
  clusterCode: string;
  namespace: string;
  podName: string;
  // 是否启用订阅（false 时不订阅也不消费），默认 true
  enabled?: boolean;
  // 是否在挂载时清空旧的日志缓冲，默认 true
  clearOnMount?: boolean;
}

export interface UsePodLogsResult {
  channel: string;
  lines: PodLogLinePayload[];
  // 纯文本拼接（用于直接展示/复制）
  text: string;
  eof: boolean;
  isEmpty: boolean;
}

/**
 * usePodLogs - 订阅 Pod 日志流
 *
 * 后端约定 channel: "pod_logs:{clusterCode}:{namespace}:{podName}"
 * 后端 BackupWorker/Informer 推送 PodLogLinePayload（含 eof 标记）
 *
 * Pod 日志流是后端按需启动（API 触发 StreamPodLogs）后通过 Hub 推送的；
 * 客户端订阅该 channel 即可接收。
 */
export function usePodLogs(opts: UsePodLogsOptions): UsePodLogsResult {
  const dispatch = useAppDispatch();
  const { clusterCode, namespace, podName, enabled = true, clearOnMount = true } = opts;
  const channel = useMemo(
    () => buildPodLogChannel(clusterCode, namespace, podName),
    [clusterCode, namespace, podName],
  );
  const mountedRef = useRef(false);

  useEffect(() => {
    if (!enabled || !clusterCode || !namespace || !podName) return;
    mountedRef.current = true;
    if (clearOnMount) dispatch(clearPodLogs(channel));
    subscribeChannel(channel);
    return () => {
      unsubscribeChannel(channel);
      mountedRef.current = false;
    };
  }, [enabled, channel, clearOnMount, dispatch, clusterCode, namespace, podName]);

  const lines = useAppSelector((s) => selectPodLogs(s, channel));

  const text = useMemo(() => lines.map((l) => l.line).join('\n'), [lines]);

  const eof = lines.length > 0 && !!lines[lines.length - 1].eof;

  return {
    channel,
    lines,
    text,
    eof,
    isEmpty: lines.length === 0,
  };
}

export const POD_LOGS_WS_TYPE = WS_TYPE_POD_LOGS;
