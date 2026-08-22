import React, { useMemo } from 'react';
import { Empty, Progress, Space, Tag, Timeline, Typography } from 'antd';
import {
  CheckCircleTwoTone,
  ClockCircleTwoTone,
  CloseCircleTwoTone,
  LoadingOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useBackupProgress, UseBackupProgressOptions } from '@/hooks/useBackupProgress';
import { TaskProgressPayload } from '@/slices/wsSlice';

const { Text } = Typography;

interface BackupProgressPanelProps extends UseBackupProgressOptions {
  height?: number;
  // 紧凑模式：仅展示最新一条
  compact?: boolean;
}

const stageColor: Record<string, string> = {
  pending: '#8c8c8c',
  exporting: '#1677ff',
  uploading: '#722ed1',
  restoring: '#fa8c16',
  completed: '#52c41a',
  failed: '#ff4d4f',
};

const stageLabel: Record<string, string> = {
  pending: '等待中',
  exporting: '导出中',
  uploading: '上传中',
  restoring: '恢复中',
  completed: '已完成',
  failed: '失败',
};

const stageIcon = (stage: string) => {
  switch (stage) {
    case 'completed':
      return <CheckCircleTwoTone twoToneColor="#52c41a" />;
    case 'failed':
      return <CloseCircleTwoTone twoToneColor="#ff4d4f" />;
    case 'pending':
      return <ClockCircleTwoTone twoToneColor="#8c8c8c" />;
    case 'exporting':
    case 'uploading':
    case 'restoring':
      return <SyncOutlined spin style={{ color: stageColor[stage] }} />;
    default:
      return <LoadingOutlined />;
  }
};

export const BackupProgressPanel: React.FC<BackupProgressPanelProps> = ({
  height = 320,
  compact = false,
  ...opts
}) => {
  const { list } = useBackupProgress(opts);

  const sorted = useMemo(() => {
    const arr = [...list];
    arr.sort((a, b) => (a.timestamp || '').localeCompare(b.timestamp || ''));
    return compact ? arr.slice(-1) : arr.reverse();
  }, [list, compact]);

  if (sorted.length === 0) {
    return (
      <div style={{ height, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Empty description="暂无备份进度推送" />
      </div>
    );
  }

  if (compact) {
    const latest = sorted[0];
    return <CompactProgress item={latest} />;
  }

  return (
    <div style={{ height, overflowY: 'auto', padding: '12px 8px' }}>
      <Timeline
        items={sorted.map((item, idx) => ({
          key: `${item.task_id}-${idx}`,
          dot: stageIcon(item.stage),
          children: (
            <Space direction="vertical" size={2} style={{ width: '100%' }}>
              <Space>
                <Tag color={stageColor[item.stage] || 'default'}>
                  {stageLabel[item.stage] || item.stage}
                </Tag>
                {item.kind && <Tag color="purple">{item.kind}</Tag>}
                {item.name && <Text type="secondary">{item.name}</Text>}
                {item.cluster_code && (
                  <Tag color="blue">{item.cluster_code}</Tag>
                )}
              </Space>
              {item.namespace && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  ns: {item.namespace}
                </Text>
              )}
              <Progress
                percent={item.progress ?? 0}
                status={
                  item.stage === 'completed'
                    ? 'success'
                    : item.stage === 'failed'
                      ? 'exception'
                      : 'active'
                }
                size="small"
              />
              {item.message && <Text style={{ fontSize: 12 }}>{item.message}</Text>}
              {item.error && (
                <Text type="danger" style={{ fontSize: 12 }}>
                  {item.error}
                </Text>
              )}
              {item.timestamp && (
                <Text type="secondary" style={{ fontSize: 11 }}>
                  {dayjs(item.timestamp).format('HH:mm:ss')}
                </Text>
              )}
            </Space>
          ),
        }))}
      />
    </div>
  );
};

const CompactProgress: React.FC<{ item?: TaskProgressPayload }> = ({ item }) => {
  if (!item) return <Empty description="无进度" />;
  return (
    <Space direction="vertical" size={4} style={{ width: '100%' }}>
      <Space>
        {stageIcon(item.stage)}
        <Tag color={stageColor[item.stage] || 'default'}>
          {stageLabel[item.stage] || item.stage}
        </Tag>
        {item.kind && <Tag color="purple">{item.kind}</Tag>}
        {item.name && <Text type="secondary">{item.name}</Text>}
      </Space>
      <Progress
        percent={item.progress ?? 0}
        status={
          item.stage === 'completed'
            ? 'success'
            : item.stage === 'failed'
              ? 'exception'
              : 'active'
        }
      />
      {item.message && <Text style={{ fontSize: 12 }}>{item.message}</Text>}
      {item.error && (
        <Text type="danger" style={{ fontSize: 12 }}>
          {item.error}
        </Text>
      )}
    </Space>
  );
};

export default BackupProgressPanel;
