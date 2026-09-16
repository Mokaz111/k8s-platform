import React, { useEffect, useMemo, useRef, useState } from 'react';
import { App, Button, Space, Tag, Typography } from 'antd';
import {
  ClearOutlined,
  DownloadOutlined,
  PauseOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { usePodLogs } from '@/hooks/usePodLogs';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { clearPodLogs, selectWSStatus } from '@/slices/wsSlice';
import request from '@/app/services/request';

const { Text } = Typography;

interface PodLogsViewerProps {
  clusterCode: string;
  namespace: string;
  podName: string;
  containerName?: string;
  height?: number;
  // 是否自动滚动到底部
  autoScroll?: boolean;
  // 是否在挂载/参数变化时自动调用后端 HTTP 接口触发日志流
  // 默认 true。后端约定：GET /api/v1/clusters/:code/pods/:ns/:pod/logs?container=&follow=1&tail=
  autoTrigger?: boolean;
  // follow 模式（持续 stream）
  follow?: boolean;
  // 初始拉取的行数
  tailLines?: number;
}

interface TriggerStreamResponse {
  channel: string;
  follow: boolean;
  tail: number;
  container?: string;
  pod: string;
  namespace: string;
  cluster: string;
  ws_endpoint: string;
  action: string;
  hint?: string;
}

/**
 * PodLogsViewer - Pod 日志实时查看器
 *
 * 工作流：
 * 1. 自动订阅 WS 通道 "pod_logs:{cluster}:{ns}:{pod}"
 * 2. （可选）调用后端 HTTP 接口触发 StreamPodLogs，让后端把日志逐行推到该频道
 * 3. 用户通过工具栏暂停滚动、清空、下载
 *
 * UI：
 * - 顶部工具栏：暂停/继续、清空、下载、连接状态
 * - 主体：黑色背景日志面板，类似 kubectl logs -f
 */
export const PodLogsViewer: React.FC<PodLogsViewerProps> = ({
  clusterCode,
  namespace,
  podName,
  containerName,
  height = 480,
  autoScroll = true,
  autoTrigger = true,
  follow = true,
  tailLines = 500,
}) => {
  const dispatch = useAppDispatch();
  const { message } = App.useApp();
  const wsStatus = useAppSelector(selectWSStatus);
  const { lines, text, eof, channel, isEmpty } = usePodLogs({
    clusterCode,
    namespace,
    podName,
  });

  const containerRef = useRef<HTMLDivElement>(null);
  const [paused, setPaused] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [triggerError, setTriggerError] = useState<string>('');

  // 触发后端日志流（仅调用一次，重试由用户主动点）
  const triggerStream = React.useCallback(async () => {
    if (!clusterCode || !namespace || !podName) return;
    setTriggering(true);
    setTriggerError('');
    try {
      const params: Record<string, string | number | boolean> = {
        follow: follow ? 1 : 0,
        tail: tailLines,
      };
      if (containerName) params.container = containerName;
      await request.get<TriggerStreamResponse>(
        `/clusters/${encodeURIComponent(clusterCode)}/pods/${encodeURIComponent(
          namespace,
        )}/${encodeURIComponent(podName)}/logs`,
        { params },
      );
    } catch (err) {
      const msg = (err as { message?: string })?.message || '触发日志流失败';
      setTriggerError(msg);
    } finally {
      setTriggering(false);
    }
  }, [clusterCode, namespace, podName, containerName, follow, tailLines]);

  useEffect(() => {
    if (!autoTrigger) return;
    if (wsStatus !== 'open') return;
    const timer = window.setTimeout(() => {
      triggerStream();
    }, 120);
    return () => window.clearTimeout(timer);
  }, [clusterCode, namespace, podName, containerName, autoTrigger, wsStatus, triggerStream]);

  // 自动滚动到底部
  useEffect(() => {
    if (!autoScroll || paused) return;
    const el = containerRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [lines, autoScroll, paused]);

  // eof 时给个提示
  useEffect(() => {
    if (eof) {
      message.info('日志流已结束');
    }
  }, [eof, message]);

  const handleClear = () => {
    dispatch(clearPodLogs(channel));
  };

  const handleDownload = () => {
    if (!text) {
      message.warning('暂无日志可下载');
      return;
    }
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${podName}-${namespace}-${clusterCode}-${Date.now()}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const fontSize = 12;
  const lineHeight = 18;
  const maxVisibleLines = Math.floor((height - 40) / lineHeight);

  // 仅渲染最近的 N 行（性能优化，避免日志过多卡死 DOM）
  const visibleLines = useMemo(() => {
    if (lines.length <= maxVisibleLines) return lines;
    return lines.slice(lines.length - maxVisibleLines);
  }, [lines, maxVisibleLines]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '4px 8px',
          background: '#fafafa',
          borderBottom: '1px solid #f0f0f0',
        }}
      >
        <Space size="small">
          <Tag color="blue">{clusterCode}</Tag>
          <Text type="secondary" style={{ fontSize: 12 }}>
            {namespace}/{podName}
          </Text>
          {containerName && (
            <Tag color="purple" style={{ fontSize: 11 }}>
              {containerName}
            </Tag>
          )}
          <Text type="secondary" style={{ fontSize: 11 }}>
            共 {lines.length} 行
          </Text>
          {eof && <Tag color="default">EOF</Tag>}
          {wsStatus !== 'open' && (
            <Tag color="warning">
              {wsStatus === 'connecting' || wsStatus === 'reconnecting'
                ? 'WebSocket 连接中'
                : 'WebSocket 未连接'}
            </Tag>
          )}
          {triggering && (
            <Text type="secondary" style={{ fontSize: 11 }}>
              正在触发流...
            </Text>
          )}
        </Space>
        <Space size="small">
          <Button
            size="small"
            type="text"
            icon={<ReloadOutlined spin={triggering} />}
            onClick={triggerStream}
            loading={triggering}
            title="重新触发日志流"
          />
          <Button
            size="small"
            type="text"
            icon={paused ? <PlayCircleOutlined /> : <PauseOutlined />}
            onClick={() => setPaused((p) => !p)}
            title={paused ? '继续滚动' : '暂停滚动'}
          />
          <Button
            size="small"
            type="text"
            icon={<ClearOutlined />}
            onClick={handleClear}
            title="清空"
          />
          <Button
            size="small"
            type="text"
            icon={<DownloadOutlined />}
            onClick={handleDownload}
            title="下载"
            disabled={isEmpty}
          />
        </Space>
      </div>
      <div
        ref={containerRef}
        style={{
          flex: 1,
          overflowY: 'auto',
          background: '#1f1f1f',
          color: '#e9e9e9',
          fontFamily: 'Menlo, Monaco, "Cascadia Code", "Courier New", monospace',
          fontSize,
          lineHeight: `${lineHeight}px`,
          padding: '8px 12px',
          margin: 0,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
        }}
      >
        {isEmpty ? (
          <div style={{ color: '#888', fontStyle: 'italic' }}>
            {triggerError
              ? triggerError
              : wsStatus !== 'open'
                ? '等待 WebSocket 连接后开始拉取日志...'
                : '等待日志输出...（已自动触发后端日志流；如长时间无输出可点工具栏刷新按钮重试）'}
          </div>
        ) : (
          visibleLines.map((l, idx) => (
            <div key={`${idx}-${l.line.length}`}>
              {l.eof ? (
                <span style={{ color: '#faad14' }}>=== 日志流结束 (EOF) ===</span>
              ) : (
                l.line || ' '
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default PodLogsViewer;
