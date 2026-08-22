import React, { useMemo } from 'react';
import { Badge, Button, Empty, List, Space, Tag, Tooltip, Typography } from 'antd';
import { BellOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useClusterEvents, UseClusterEventsOptions } from '@/hooks/useClusterEvents';

const { Text } = Typography;

interface ClusterEventsListProps extends UseClusterEventsOptions {
  height?: number;
  showHeader?: boolean;
  // 显示最大条数
  maxItems?: number;
  // 是否显示"清空"按钮（需要外层 dispatch）
  onClear?: () => void;
}

const eventTypeColor: Record<string, string> = {
  ADDED: 'green',
  MODIFIED: 'blue',
  DELETED: 'red',
  ERROR: 'volcano',
};

const severityBadge: Record<string, 'success' | 'warning' | 'error' | 'default'> = {
  normal: 'success',
  warning: 'warning',
  error: 'error',
};

export const ClusterEventsList: React.FC<ClusterEventsListProps> = ({
  height = 360,
  showHeader = true,
  maxItems = 100,
  onClear,
  ...opts
}) => {
  const { list, latest, warnings, errors } = useClusterEvents(opts);

  const sorted = useMemo(() => {
    const arr = list.slice(-maxItems);
    arr.reverse();
    return arr;
  }, [list, maxItems]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height }}>
      {showHeader && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '8px 12px',
            background: '#fafafa',
            borderBottom: '1px solid #f0f0f0',
          }}
        >
          <Space>
            <Badge count={list.length} size="small" offset={[6, 0]}>
              <BellOutlined style={{ fontSize: 16 }} />
            </Badge>
            <Text strong>集群事件</Text>
            <Tag color="warning">{warnings.length} warning</Tag>
            <Tag color="error">{errors.length} error</Tag>
            {latest && (
              <Tooltip title={latest.timestamp ? dayjs(latest.timestamp).format('HH:mm:ss') : ''}>
                <Text type="secondary" style={{ fontSize: 11 }}>
                  最近: {latest.cluster_code} {latest.kind} {latest.name}
                </Text>
              </Tooltip>
            )}
          </Space>
          {onClear && (
            <Button size="small" type="link" onClick={onClear}>
              清空
            </Button>
          )}
        </div>
      )}
      <div style={{ flex: 1, overflowY: 'auto' }}>
        {sorted.length === 0 ? (
          <div
            style={{
              height: '100%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Empty description="暂无集群事件" />
          </div>
        ) : (
          <List
            itemLayout="horizontal"
            dataSource={sorted}
            renderItem={(item) => (
              <List.Item style={{ padding: '8px 12px' }}>
                <Space direction="vertical" size={2} style={{ width: '100%' }}>
                  <Space wrap>
                    <Badge status={severityBadge[item.severity || 'normal']} />
                    <Tag color={eventTypeColor[item.event_type] || 'default'}>
                      {item.event_type}
                    </Tag>
                    <Tag color="purple">{item.kind}</Tag>
                    <Tag color="blue">{item.cluster_code}</Tag>
                    {item.namespace && (
                      <Text type="secondary" style={{ fontSize: 11 }}>
                        ns: {item.namespace}
                      </Text>
                    )}
                    <Text strong style={{ fontSize: 12 }}>
                      {item.name}
                    </Text>
                  </Space>
                  {(item.reason || item.message) && (
                    <Text style={{ fontSize: 11, color: 'rgba(0,0,0,0.65)' }}>
                      {item.reason ? `[${item.reason}] ` : ''}
                      {item.message || ''}
                    </Text>
                  )}
                  {item.timestamp && (
                    <Text type="secondary" style={{ fontSize: 10 }}>
                      {dayjs(item.timestamp).format('MM-DD HH:mm:ss')}
                    </Text>
                  )}
                </Space>
              </List.Item>
            )}
          />
        )}
      </div>
    </div>
  );
};

export default ClusterEventsList;
