import { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { connectWebSocket, disconnectWebSocket } from '@/app/services/websocket';
import { resetWS } from '@/slices/wsSlice';

/**
 * useWebSocket - 全局唯一 WebSocket 连接生命周期管理 Hook
 *
 * 设计要点：
 * - 仅在已登录（token 存在）时建立连接
 * - token 变更时自动断开旧连接并重连（避免使用过期 token）
 * - 卸载时不主动断开（BasicLayout 卸载通常意味着整个 SPA 销毁），
 *   由 logout() 流程显式调用 disconnectWebSocket
 *
 * 在 BasicLayout 中调用一次即可。
 */
export function useWebSocketLifecycle(): void {
  const dispatch = useAppDispatch();
  const token = useAppSelector((s) => s.user.token);

  useEffect(() => {
    if (!token) {
      // 未登录：确保旧连接关闭
      disconnectWebSocket();
      dispatch(resetWS());
      return;
    }
    // 建立连接，重连由 reconnecting-websocket 内部处理
    connectWebSocket(token);

    return () => {
      // 当 token 变化或组件卸载：断开 + 重置
      disconnectWebSocket();
      dispatch(resetWS());
    };
  }, [token, dispatch]);
}
