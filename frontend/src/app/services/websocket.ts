import ReconnectingWebSocket from 'reconnecting-websocket';
import type { AnyAction } from '@reduxjs/toolkit';
import { store } from '@/app/store';
import {
  onMessage,
  setStatus,
  setReconnecting,
  addSubscription,
  removeSubscription,
  clearSubscriptions,
  WSClientAction,
  WSMessage,
  WSConnectionStatus,
} from '@/slices/wsSlice';

const WS_BASE_URL =
  import.meta.env.VITE_WS_BASE_URL ||
  // 自动从 VITE_API_BASE_URL 推导：/api/v1 -> /api/v1/ws，默认 ws://localhost:8080/api/v1/ws
  (import.meta.env.VITE_API_BASE_URL
    ? `${import.meta.env.VITE_API_BASE_URL.replace(/^http/, 'ws')}/ws`
    : `ws://${window.location.hostname}:8080/api/v1/ws`);

// 模块级单例：整个 SPA 共享一个 WebSocket 连接
let instance: WebSocketClient | null = null;

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

    // 后端约定 ?token=xxx 鉴权（websocket/handler.go）
    const url = `${WS_BASE_URL}?token=${encodeURIComponent(this.token)}`;
    this.dispatch(setStatus('connecting'));

    this.rws = new ReconnectingWebSocket(url, undefined, {
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
      this.explicitlyClosed = false;
      this.dispatch(setStatus('open'));
      // 重连后恢复订阅
      const state = store.getState().ws;
      state.subscriptions.forEach((ch) => this.sendRaw({ action: 'subscribe', channel: ch }));
      // pendingSubscriptions：在 connect 之前调用的 subscribe() 也补发
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
      } else {
        this.dispatch(setReconnecting());
      }
      if (ev.code === 1008 || ev.code === 4001) {
        // 鉴权失败 / token 过期：关闭且不重试
        this.rws?.close(1000, 'auth failed');
      }
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
    this.dispatch(clearSubscriptions());
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
  if (instance && !instance.isClosed()) {
    // 已连接：如果 token 变了则重连，否则直接返回
    return instance;
  }
  if (instance && instance.isClosed()) {
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

export function subscribeChannel(channel: string): void {
  instance?.subscribe(channel);
}

export function unsubscribeChannel(channel: string): void {
  instance?.unsubscribe(channel);
}

export type { WSConnectionStatus };
