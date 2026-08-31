import ReconnectingWebSocket from 'reconnecting-websocket';
import type { AnyAction } from '@reduxjs/toolkit';
import { store } from '@/app/store';
import {
  onMessage,
  setStatus,
  setLastError,
  setReconnecting,
  addSubscription,
  removeSubscription,
  WSClientAction,
  WSMessage,
  WSConnectionStatus,
} from '@/slices/wsSlice';

const VITE_WS = import.meta.env.VITE_WS_BASE_URL as string | undefined;
const VITE_API = import.meta.env.VITE_API_BASE_URL as string | undefined;
const WS_BASE_URL =
  (VITE_WS && VITE_WS.length > 0)
    ? VITE_WS
    : (VITE_API && VITE_API.length > 0)
      ? `${VITE_API.replace(/^http/, 'ws')}/ws`
      // 开发模式下默认同源：经 Vite proxy（ws:true）转发到后端 8080，避免跨域与直连问题
      : `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/ws`;

// 模块级单例：整个 SPA 共享一个 WebSocket 连接
let instance: WebSocketClient | null = null;

// channel 引用计数（模块级，跨连接实例存活）：
// 多个组件可能订阅同一 channel（如 task_progress / cluster_event），
// 其中一个卸载时不能立刻退订，只有计数归零才真正向后端发送 unsubscribe。
// 计数放在模块级而非实例内部，是为了在 token 刷新重建连接后仍能恢复订阅。
const channelRefs = new Map<string, number>();

export interface WebSocketClientOptions {
  token: string;
  heartbeatIntervalMs?: number; // 心跳间隔，默认 25s（< 后端 wsReadDeadline 30s）
  reconnectMinDelayMs?: number; // 重连最小退避
  reconnectMaxDelayMs?: number;
}

class WebSocketClient {
  private rws: ReconnectingWebSocket | null = null;
  private token: string;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private readonly heartbeatIntervalMs: number;
  private explicitlyClosed = false;
  private pendingSubscriptions: Set<string> = new Set();

  constructor(opts: WebSocketClientOptions) {
    this.token = opts.token;
    this.heartbeatIntervalMs = opts.heartbeatIntervalMs ?? 25_000;

    this.dispatch(setStatus('connecting'));

    // 关键：URL 以函数形式提供（reconnecting-websocket 的 UrlProvider），
    // 每次（重）连接都会重新求值，从 Redux 读取最新 token。
    // 若固化 URL，token 刷新后重连会一直携带旧 token，后端握手返回 401
    // （浏览器表现为 close code 1006），从而陷入无限失败重试。
    const urlProvider = () => WS_BASE_URL;
    const fresh = store.getState().user.token;
    const token = fresh && fresh.length > 0 ? fresh : this.token;

    this.rws = new ReconnectingWebSocket(urlProvider, [`access_token.${token}`], {
      minReconnectionDelay: opts.reconnectMinDelayMs ?? 1000,
      maxReconnectionDelay: opts.reconnectMaxDelayMs ?? 30_000,
      reconnectionDelayGrowFactor: 1.4,
      maxRetries: Infinity,
      debug: false,
    });

    this.attachListeners();
  }

  private dispatch(action: AnyAction): void {
    store.dispatch(action);
  }

  private attachListeners(): void {
    if (!this.rws) return;

    this.rws.onopen = () => {
      this.dispatch(setStatus('open'));
      // 重连后恢复订阅：以模块级 channelRefs 为准（计数 > 0 即应订阅）
      channelRefs.forEach((count, ch) => {
        if (count > 0) {
          this.dispatch(addSubscription(ch)); // 同步 Redux subscriptions
          this.sendRaw({ action: 'subscribe', channel: ch });
        }
      });
      // pendingSubscriptions：在连接打开前调用的 subscribe() 也补发
      this.pendingSubscriptions.forEach((ch) =>
        this.sendRaw({ action: 'subscribe', channel: ch }),
      );
      this.pendingSubscriptions.clear();
      this.startHeartbeat();
    };

    this.rws.onclose = (ev) => {
      this.stopHeartbeat();
      if (this.explicitlyClosed) {
        this.dispatch(setStatus('closed'));
        return;
      }
      if (ev.code === 1008 || ev.code === 4001) {
        // 鉴权失败 / token 过期（若后端在 WS 层显式下发）：
        // 停止重连并记录错误。注意：当前后端鉴权失败发生在 HTTP 握手阶段，
        // 浏览器只会给 close code 1006，此分支作为后端未来显式下发时的兜底。
        this.dispatch(setLastError(`WebSocket 鉴权失败 (close code ${ev.code})，已停止重连`));
        try {
          // close() 会置 _shouldReconnect=false 并取消 pending 的重连任务
          this.rws?.close(1000, 'auth failed');
        } catch {
          // ignore
        }
        return;
      }
      this.dispatch(setReconnecting());
    };

    this.rws.onerror = () => {
      // 错误事件后通常跟着 onclose，这里仅记录
      this.dispatch(setStatus('error'));
    };

    this.rws.onmessage = (event) => {
      this.handleMessage(event.data);
    };
  }

  private handleMessage(raw: unknown): void {
    if (typeof raw !== 'string' && !(raw instanceof ArrayBuffer) && !(raw instanceof Blob)) {
      return;
    }
    let text: string;
    if (typeof raw === 'string') {
      text = raw;
    } else if (raw instanceof ArrayBuffer) {
      text = new TextDecoder().decode(raw);
    } else {
      // Blob: 异步
      raw.text().then((t) => this.handleMessage(t)).catch(() => {});
      return;
    }
    let msg: WSMessage;
    try {
      msg = JSON.parse(text) as WSMessage;
    } catch {
      return; // 非 JSON 忽略
    }
    this.dispatch(onMessage(msg));
  }

  private sendRaw(action: WSClientAction): void {
    if (!this.rws || this.rws.readyState !== ReconnectingWebSocket.OPEN) {
      // 未连接时缓存 subscribe，等 onopen 补发
      if (action.action === 'subscribe' && action.channel) {
        this.pendingSubscriptions.add(action.channel);
      } else if (action.action === 'unsubscribe' && action.channel) {
        this.pendingSubscriptions.delete(action.channel);
      }
      return;
    }
    try {
      this.rws.send(JSON.stringify(action));
    } catch {
      // 忽略瞬时错误，重连后会补发
    }
  }

  // ---------- 公开 API ----------

  subscribe(channel: string): void {
    if (!channel) return;
    this.dispatch(addSubscription(channel));
    this.sendRaw({ action: 'subscribe', channel });
  }

  unsubscribe(channel: string): void {
    if (!channel) return;
    this.dispatch(removeSubscription(channel));
    this.sendRaw({ action: 'unsubscribe', channel });
    this.pendingSubscriptions.delete(channel);
  }

  getToken(): string {
    return this.token;
  }

  isClosed(): boolean {
    return this.explicitlyClosed;
  }

  isOpen(): boolean {
    return this.rws?.readyState === ReconnectingWebSocket.OPEN;
  }

  close(): void {
    this.explicitlyClosed = true;
    this.stopHeartbeat();
    this.pendingSubscriptions.clear();
    // 注意：不在这里清 Redux subscriptions / channelRefs。
    // token 刷新场景会重建连接并在 onopen 时按 channelRefs 恢复订阅；
    // 登出场景由 useWebSocketLifecycle 显式调用 resetChannelRefs + resetWS。
    try {
      this.rws?.close(1000, 'client close');
    } catch {
      // ignore
    }
    this.rws = null;
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      if (this.rws?.readyState === ReconnectingWebSocket.OPEN) {
        this.sendRaw({ action: 'ping' });
      }
    }, this.heartbeatIntervalMs);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }
}

// ---------- 单例管理 ----------

export function connectWebSocket(token: string): WebSocketClient {
  if (instance && !instance.isClosed() && instance.getToken() === token) {
    // 已连接且 token 未变：复用
    return instance;
  }
  if (instance) {
    // token 变化或旧实例已显式关闭：销毁重建（订阅由 channelRefs 恢复）
    instance.close();
    instance = null;
  }
  instance = new WebSocketClient({ token });
  return instance;
}

export function getWebSocket(): WebSocketClient | null {
  return instance;
}

export function disconnectWebSocket(): void {
  if (instance) {
    instance.close();
    instance = null;
  }
}

// ---------- 带引用计数的订阅管理 ----------

export function subscribeChannel(channel: string): void {
  if (!channel) return;
  const next = (channelRefs.get(channel) ?? 0) + 1;
  channelRefs.set(channel, next);
  if (next === 1) {
    // 首个订阅者才真正建立订阅（重复 subscribe 由后端 channels map 天然去重，
    // 但引用计数可避免"一个组件卸载导致其他组件断流"）
    instance?.subscribe(channel);
  }
}

export function unsubscribeChannel(channel: string): void {
  if (!channel) return;
  const cur = channelRefs.get(channel) ?? 0;
  if (cur <= 1) {
    channelRefs.delete(channel);
    instance?.unsubscribe(channel);
  } else {
    channelRefs.set(channel, cur - 1);
  }
}

/** 清空 channel 引用计数（登出时调用，避免下次登录继承旧订阅） */
export function resetChannelRefs(): void {
  channelRefs.clear();
}

export type { WSConnectionStatus };
