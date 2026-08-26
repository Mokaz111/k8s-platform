import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@/app/store';
import {
  connectWebSocket,
  disconnectWebSocket,
  resetChannelRefs,
} from '@/app/services/websocket';
import { resetWS } from '@/slices/wsSlice';

/**
 * useWebSocket - 全局唯一 WebSocket 连接生命周期管理 Hook
 *
 * 设计要点：
 * - 仅在已登录（token 存在）时建立连接
 * - token 变更时自动断开旧连接并重连（连接 URL 每次重连都会读取最新 token）
 * - 登出（token 置空）时彻底清理：连接、channel 引用计数、Redux ws 状态
 * - 组件卸载（token 变化）时断开连接但保留 channel 引用计数，
 *   新连接建立后由 onopen 按引用计数自动恢复页面级订阅
 *
 * 在 BasicLayout 中调用一次即可。
 */
export function useWebSocketLifecycle(): void {
  const dispatch = useAppDispatch();
  const token = useAppSelector((s) => s.user.token);

  useEffect(() => {
    if (!token) {
      // 未登录：确保旧连接关闭，并清理订阅引用计数（防止下次登录继承）
      disconnectWebSocket();
      resetChannelRefs();
      dispatch(resetWS());
      return;
    }
    // 建立连接，重连由 reconnecting-websocket 内部处理
    connectWebSocket(token);

    return () => {
      // token 变化或组件卸载：断开连接 + 重置 Redux ws 状态。
      // channelRefs 保留：新连接 onopen 时按计数恢复订阅，
      // 页面组件无需重新挂载即可继续收推送
      disconnectWebSocket();
      dispatch(resetWS());
    };
  }, [token, dispatch]);
}
