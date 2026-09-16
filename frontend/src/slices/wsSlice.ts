import { createSlice, PayloadAction } from '@reduxjs/toolkit';

// 与后端 websocket/hub.go 中 Type* 常量保持一致
export const WS_TYPE_POD_LOGS = 'pod_logs';
export const WS_TYPE_TASK_PROGRESS = 'task_progress';
export const WS_TYPE_CLUSTER_EVENT = 'cluster_event';
export const WS_TYPE_PONG = 'pong';
export const WS_TYPE_ERROR = 'error';

export type WSMessageType =
  | typeof WS_TYPE_POD_LOGS
  | typeof WS_TYPE_TASK_PROGRESS
  | typeof WS_TYPE_CLUSTER_EVENT
  | typeof WS_TYPE_PONG
  | typeof WS_TYPE_ERROR;

export type WSConnectionStatus =
  | 'idle'
  | 'connecting'
  | 'open'
  | 'closing'
  | 'closed'
  | 'reconnecting'
  | 'error';

// 后端约定的客户端动作（websocket/handler.go clientAction）
export interface WSClientAction {
  action: 'subscribe' | 'unsubscribe' | 'ping';
  channel?: string;
}

// 后端推送过来的统一消息（websocket/hub.go Message）
export interface WSMessage<T = unknown> {
  type: WSMessageType;
  channel?: string;
  data?: T;
}

// 备份任务进度（后端 BackupWorker 发布）
export interface TaskProgressPayload {
  task_id: string;
  backup_id?: string;
  cluster_code?: string;
  namespace?: string;
  kind?: string;
  name?: string;
  stage: 'pending' | 'exporting' | 'uploading' | 'completed' | 'failed' | 'restoring';
  progress: number; // 0-100
  message?: string;
  error?: string;
  timestamp?: string;
}

// 集群事件（K8s Informer 推送）
export interface ClusterEventPayload {
  cluster_code: string;
  kind: string;
  namespace?: string;
  name: string;
  event_type: 'ADDED' | 'MODIFIED' | 'DELETED' | 'ERROR';
  reason?: string;
  message?: string;
  severity?: 'normal' | 'warning' | 'error';
  timestamp?: string;
}

// Pod 日志单行（后端 pod_logs.go podLogLine）
export interface PodLogLinePayload {
  cluster_code: string;
  namespace: string;
  pod_name: string;
  container_name?: string;
  line: string;
  follow?: boolean;
  eof?: boolean;
}

export interface WSState {
  status: WSConnectionStatus;
  lastError?: string;
  lastConnectedAt?: string;
  reconnectCount: number;

  // 当前已订阅的 channel 集合（用于断线重连后恢复订阅）
  subscriptions: string[];

  // 三通道消息队列（环形缓冲，避免无界增长）
  taskProgress: TaskProgressPayload[];
  clusterEvents: ClusterEventPayload[];

  // Pod 日志按 channel 分桶存储，每个 pod 一个独立队列
  // 顶层 key 为 channel name，如 "pod_logs:cluster1:default:nginx"
  podLogs: Record<string, PodLogLinePayload[]>;

  // 最近一条 pong 时间戳（健康检查）
  lastPongAt?: string;
}

const MAX_TASK_PROGRESS = 200;
const MAX_CLUSTER_EVENTS = 200;
const MAX_POD_LOG_LINES = 1000;

const initialState: WSState = {
  status: 'idle',
  reconnectCount: 0,
  subscriptions: [],
  taskProgress: [],
  clusterEvents: [],
  podLogs: {},
};

// 环形缓冲：超过上限丢弃最早的
function ringPush<T>(arr: T[], item: T, max: number): T[] {
  arr.push(item);
  if (arr.length > max) {
    arr.splice(0, arr.length - max);
  }
  return arr;
}

const wsSlice = createSlice({
  name: 'ws',
  initialState,
  reducers: {
    setStatus(state, action: PayloadAction<WSConnectionStatus>) {
      state.status = action.payload;
      if (action.payload === 'open') {
        state.lastConnectedAt = new Date().toISOString();
        state.lastError = undefined;
      }
    },
    setLastError(state, action: PayloadAction<string | undefined>) {
      state.lastError = action.payload;
      if (action.payload) {
        state.status = 'error';
      }
    },
    setReconnecting(state) {
      state.status = 'reconnecting';
      state.reconnectCount += 1;
    },
    bumpReconnect(state) {
      state.reconnectCount += 1;
    },

    addSubscription(state, action: PayloadAction<string>) {
      const ch = action.payload;
      if (!ch) return;
      if (!state.subscriptions.includes(ch)) {
        state.subscriptions.push(ch);
      }
    },
    removeSubscription(state, action: PayloadAction<string>) {
      const ch = action.payload;
      state.subscriptions = state.subscriptions.filter((c) => c !== ch);
      if (ch.startsWith(WS_TYPE_POD_LOGS + ':')) {
        // pod_logs:{cluster}:{ns}:{pod} 取消订阅时清空对应缓冲
        delete state.podLogs[ch];
      }
    },
    clearSubscriptions(state) {
      state.subscriptions = [];
      state.podLogs = {};
    },

    // 统一消息入口：由 websocket client 收到后 dispatch
    onMessage(state, action: PayloadAction<WSMessage>) {
      const msg = action.payload;
      switch (msg.type) {
        case WS_TYPE_PONG:
          state.lastPongAt = new Date().toISOString();
          break;
        case WS_TYPE_ERROR: {
          const payload = msg.data as { message?: string } | undefined;
          state.lastError = payload?.message || 'WebSocket 频道订阅失败';
          break;
        }
        case WS_TYPE_TASK_PROGRESS: {
          const payload = msg.data as TaskProgressPayload | undefined;
          if (payload) ringPush(state.taskProgress, payload, MAX_TASK_PROGRESS);
          break;
        }
        case WS_TYPE_CLUSTER_EVENT: {
          const payload = msg.data as ClusterEventPayload | undefined;
          if (payload) ringPush(state.clusterEvents, payload, MAX_CLUSTER_EVENTS);
          break;
        }
        case WS_TYPE_POD_LOGS: {
          const payload = msg.data as PodLogLinePayload | undefined;
          if (!payload) break;
          // 正常路径：StreamPodLogs 直发，channel 为 "pod_logs:{cluster}:{ns}:{pod}"。
          // Redis PubSub 路径：channel 仅为 "pod_logs"，此时用 payload 字段兜底构造，
          // 避免所有 Pod 的日志混入同一个无人读取的桶。
          const channel =
            msg.channel && msg.channel.includes(':')
              ? msg.channel
              : buildPodLogChannel(payload.cluster_code, payload.namespace, payload.pod_name);
          if (!channel) break;
          if (!state.podLogs[channel]) {
            state.podLogs[channel] = [];
          }
          // eof 标记也入队，便于 UI 显示流结束
          ringPush(state.podLogs[channel], payload, MAX_POD_LOG_LINES);
          break;
        }
        default:
          break;
      }
    },

    clearTaskProgress(state) {
      state.taskProgress = [];
    },
    clearClusterEvents(state) {
      state.clusterEvents = [];
    },
    clearPodLogs(state, action: PayloadAction<string | undefined>) {
      if (action.payload) {
        delete state.podLogs[action.payload];
      } else {
        state.podLogs = {};
      }
    },

    resetWS(state) {
      state.status = 'idle';
      state.lastError = undefined;
      state.reconnectCount = 0;
      state.subscriptions = [];
      state.taskProgress = [];
      state.clusterEvents = [];
      state.podLogs = {};
      state.lastPongAt = undefined;
    },
  },
});

export const {
  setStatus,
  setLastError,
  setReconnecting,
  bumpReconnect,
  addSubscription,
  removeSubscription,
  clearSubscriptions,
  onMessage,
  clearTaskProgress,
  clearClusterEvents,
  clearPodLogs,
  resetWS,
} = wsSlice.actions;

export default wsSlice.reducer;

// ---------- 选择器 ----------

export const selectWSStatus = (s: { ws: WSState }): WSConnectionStatus => s.ws.status;
export const selectWSSubscriptions = (s: { ws: WSState }): string[] => s.ws.subscriptions;
export const selectTaskProgress = (s: { ws: WSState }): TaskProgressPayload[] => s.ws.taskProgress;
export const selectClusterEvents = (s: { ws: WSState }): ClusterEventPayload[] =>
  s.ws.clusterEvents;
export const selectPodLogs = (
  s: { ws: WSState },
  channel: string,
): PodLogLinePayload[] => s.ws.podLogs[channel] || [];

// Pod 日志 channel 构造（与后端 PodLogChannelName 一致）
export const buildPodLogChannel = (clusterCode: string, namespace: string, podName: string) =>
  `${WS_TYPE_POD_LOGS}:${clusterCode}:${namespace}:${podName}`;
