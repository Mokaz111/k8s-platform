import React, { useCallback, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  DatePicker,
  Descriptions,
  Form,
  Input,
  Modal,
  Row,
  Space,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import { ReloadOutlined, EyeOutlined, SearchOutlined, ClearOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import {
  AuditLog,
  ListAuditLogsParams,
  useListAuditLogsQuery,
} from '@/app/services/rbac';
import { usePermission } from '@/hooks/usePermission';

const { Text } = Typography;
const { RangePicker } = DatePicker;

// 状态颜色映射：success=成功, failed=失败, error=异常
const statusColorMap: Record<string, string> = {
  success: 'success',
  failed: 'error',
  error: 'error',
  pending: 'warning',
};

const statusLabelMap: Record<string, string> = {
  success: '成功',
  failed: '失败',
  error: '异常',
  pending: '进行中',
};

// 请求方法颜色
const methodColorMap: Record<string, string> = {
  GET: 'blue',
  POST: 'green',
  PUT: 'orange',
  PATCH: 'gold',
  DELETE: 'red',
};

interface FilterFormValues {
  username?: string;
  module?: string;
  action?: string;
  status?: string;
  cluster_code?: string;
  namespace?: string;
  target_type?: string;
  target_id?: string;
  trace_id?: string;
  time_range?: [dayjs.Dayjs, dayjs.Dayjs];
}

const AuditLogList: React.FC = () => {
  const { hasPerm } = usePermission();
  const canView = hasPerm('audit:view');

  const [filters, setFilters] = useState<ListAuditLogsParams>({
    page: 1,
    page_size: 10,
  });

  const [detailRecord, setDetailRecord] = useState<AuditLog | null>(null);
  const [searchForm] = Form.useForm<FilterFormValues>();

  const { data, isFetching, refetch } = useListAuditLogsQuery(filters, {
    skip: !canView,
    refetchOnMountOrArgChange: true,
  });

  const handleReload = useCallback(() => refetch(), [refetch]);

  const handleSearch = useCallback((values: FilterFormValues) => {
    setFilters((prev) => ({
      ...prev,
      username: values.username || undefined,
      module: values.module || undefined,
      action: values.action || undefined,
      status: values.status || undefined,
      cluster_code: values.cluster_code || undefined,
      namespace: values.namespace || undefined,
      target_type: values.target_type || undefined,
      target_id: values.target_id || undefined,
      trace_id: values.trace_id || undefined,
      start_time: values.time_range?.[0]?.format('YYYY-MM-DD HH:mm:ss'),
      end_time: values.time_range?.[1]?.format('YYYY-MM-DD HH:mm:ss'),
      page: 1,
    }));
  }, []);

  const handleReset = useCallback(() => {
    searchForm.resetFields();
    setFilters({ page: 1, page_size: 10 });
  }, [searchForm]);

  const handlePageChange = useCallback((page: number, pageSize: number) => {
    setFilters((prev) => ({ ...prev, page, page_size: pageSize }));
  }, []);

  const columns: ProColumns<AuditLog>[] = useMemo(
    () => [
      {
        title: '时间',
        dataIndex: 'created_at',
        key: 'created_at',
        width: 170,
        fixed: 'left',
        render: (_text, record) =>
          record.created_at
            ? dayjs(record.created_at).format('YYYY-MM-DD HH:mm:ss')
            : '-',
      },
      {
        title: '用户',
        dataIndex: 'username',
        key: 'username',
        width: 130,
        render: (_text, record) => (
          <Tooltip title={`ID: ${record.user_id ?? '-'}`}>
            <span>{record.username || '-'}</span>
          </Tooltip>
        ),
      },
      {
        title: '模块/操作',
        key: 'module',
        width: 170,
        render: (_text, record) => (
          <Space direction="vertical" size={0}>
            <Space size={4} wrap>
              <Tag color="geekblue">{record.module || '-'}</Tag>
              <Tag color="purple">{record.action || '-'}</Tag>
            </Space>
            {record.target_type && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                {record.target_type}
                {record.target_id ? `: ${record.target_id}` : ''}
              </Text>
            )}
          </Space>
        ),
      },
      {
        title: '请求',
        key: 'request',
        width: 200,
        render: (_text, record) => (
          <Space direction="vertical" size={0} style={{ width: '100%' }}>
            <Space size={4}>
              {record.request_method && (
                <Tag color={methodColorMap[record.request_method] || 'default'}>
                  {record.request_method}
                </Tag>
              )}
              <Text style={{ fontSize: 12 }} ellipsis>
                {record.request_uri || '-'}
              </Text>
            </Space>
            {(record.cluster_code || record.namespace) && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                {record.cluster_code || ''}
                {record.namespace ? ` / ${record.namespace}` : ''}
              </Text>
            )}
          </Space>
        ),
      },
      {
        title: '客户端 IP',
        dataIndex: 'client_ip',
        key: 'client_ip',
        width: 130,
        render: (text) => text || '-',
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 90,
        render: (text) => {
          const status = String(text || '');
          return (
            <Tag color={statusColorMap[status] || 'default'}>
              {statusLabelMap[status] || status || '-'}
            </Tag>
          );
        },
      },
      {
        title: '耗时',
        dataIndex: 'cost_ms',
        key: 'cost_ms',
        width: 90,
        align: 'right',
        render: (text) => {
          const ms = Number(text);
          if (!ms && ms !== 0) return '-';
          const color = ms > 1000 ? 'red' : ms > 300 ? 'orange' : 'green';
          return <Tag color={color}>{ms} ms</Tag>;
        },
      },
      {
        title: '响应码',
        dataIndex: 'response_code',
        key: 'response_code',
        width: 80,
        align: 'right',
        render: (text) => {
          const code = Number(text);
          if (!code && code !== 0) return '-';
          const color = code >= 200 && code < 300 ? 'success' : code >= 400 ? 'error' : 'default';
          return <Tag color={color}>{code}</Tag>;
        },
      },
      {
        title: '错误信息',
        dataIndex: 'error_msg',
        key: 'error_msg',
        width: 180,
        render: (text) =>
          text ? (
            <Tooltip title={text}>
              <Text type="danger" style={{ fontSize: 12 }} ellipsis>
                {String(text)}
              </Text>
            </Tooltip>
          ) : (
            '-'
          ),
      },
      {
        title: 'TraceID',
        dataIndex: 'trace_id',
        key: 'trace_id',
        width: 140,
        render: (text) =>
          text ? (
            <Tooltip title={text}>
              <Text code style={{ fontSize: 11 }}>
                {String(text).slice(0, 12)}
              </Text>
            </Tooltip>
          ) : (
            '-'
          ),
      },
      {
        title: '操作',
        key: 'action',
        width: 70,
        fixed: 'right',
        render: (_text, record) => (
          <Button
            type="link"
            size="small"
            icon={<EyeOutlined />}
            onClick={() => setDetailRecord(record)}
          >
            详情
          </Button>
        ),
      },
    ],
    [],
  );

  if (!canView) {
    return (
      <PageContainer>
        <Card>
          <Text type="warning">您没有查看审计日志的权限（audit:view）。</Text>
        </Card>
      </PageContainer>
    );
  }

  return (
    <PageContainer
      extra={[
        <Button
          key="reload"
          icon={<ReloadOutlined />}
          loading={isFetching}
          onClick={handleReload}
        >
          刷新
        </Button>,
      ]}
    >
      {/* 多维度筛选栏 */}
      <Card style={{ marginBottom: 16 }} bodyStyle={{ paddingBottom: 8 }}>
        <Form<FilterFormValues>
          form={searchForm}
          layout="inline"
          onFinish={handleSearch}
          style={{ rowGap: 12 }}
        >
          <Row gutter={[16, 12]} style={{ width: '100%' }}>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="username" label="用户">
                <Input allowClear placeholder="用户名" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="module" label="模块">
                <Input allowClear placeholder="如 cluster/backup/resource" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="action" label="操作">
                <Input allowClear placeholder="如 create/update/delete" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="status" label="状态">
                <Input allowClear placeholder="success/failed/error" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="cluster_code" label="集群">
                <Input allowClear placeholder="集群编码" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="namespace" label="命名空间">
                <Input allowClear placeholder="namespace" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="target_type" label="目标类型">
                <Input allowClear placeholder="如 Cluster/Backup" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="target_id" label="目标ID">
                <Input allowClear placeholder="目标对象 ID" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={6}>
              <Form.Item name="trace_id" label="TraceID">
                <Input allowClear placeholder="链路追踪 ID" />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12} md={8} lg={12}>
              <Form.Item name="time_range" label="时间范围">
                <RangePicker
                  showTime
                  style={{ width: '100%' }}
                  format="YYYY-MM-DD HH:mm:ss"
                />
              </Form.Item>
            </Col>
            <Col xs={24} style={{ textAlign: 'right' }}>
              <Space>
                <Button type="primary" htmlType="submit" icon={<SearchOutlined />}>
                  查询
                </Button>
                <Button onClick={handleReset} icon={<ClearOutlined />}>
                  重置
                </Button>
              </Space>
            </Col>
          </Row>
        </Form>
      </Card>

      <ProTable<AuditLog>
        rowKey="id"
        loading={isFetching}
        columns={columns}
        dataSource={data?.items || []}
        scroll={{ x: 1400 }}
        search={false}
        pagination={{
          current: filters.page || 1,
          pageSize: filters.page_size || 10,
          total: data?.total || 0,
          showSizeChanger: true,
          showTotal: (total) => `共 ${total} 条`,
          onChange: handlePageChange,
        }}
        options={{
          density: false,
          fullScreen: false,
          reload: false,
          setting: false,
        }}
      />

      {detailRecord && (
        <AuditDetailModal record={detailRecord} onClose={() => setDetailRecord(null)} />
      )}
    </PageContainer>
  );
};

// 审计详情弹窗
const AuditDetailModal: React.FC<{ record: AuditLog; onClose: () => void }> = ({
  record,
  onClose,
}) => {
  const jsonBody = useMemo(() => {
    try {
      return JSON.stringify(record.request_body, null, 2);
    } catch {
      return String(record.request_body ?? '');
    }
  }, [record.request_body]);

  return (
    <Modal
      title="审计日志详情"
      open
      onCancel={onClose}
      onOk={onClose}
      width={720}
      footer={null}
    >
      <Descriptions column={2} size="small" bordered>
        <Descriptions.Item label="时间" span={2}>
          {record.created_at
            ? dayjs(record.created_at).format('YYYY-MM-DD HH:mm:ss')
            : '-'}
        </Descriptions.Item>
        <Descriptions.Item label="TraceID" span={2}>
          {record.trace_id || '-'}
        </Descriptions.Item>
        <Descriptions.Item label="用户">{record.username || '-'}</Descriptions.Item>
        <Descriptions.Item label="用户ID">{record.user_id ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="客户端IP" span={2}>
          {record.client_ip || '-'}
        </Descriptions.Item>
        <Descriptions.Item label="模块">{record.module || '-'}</Descriptions.Item>
        <Descriptions.Item label="操作">{record.action || '-'}</Descriptions.Item>
        <Descriptions.Item label="目标类型">{record.target_type || '-'}</Descriptions.Item>
        <Descriptions.Item label="目标ID">{record.target_id || '-'}</Descriptions.Item>
        <Descriptions.Item label="集群">{record.cluster_code || '-'}</Descriptions.Item>
        <Descriptions.Item label="命名空间">{record.namespace || '-'}</Descriptions.Item>
        <Descriptions.Item label="请求方法" span={2}>
          <Tag color={methodColorMap[record.request_method || ''] || 'default'}>
            {record.request_method || '-'}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item label="请求URI" span={2}>
          <Text copyable style={{ wordBreak: 'break-all' }}>
            {record.request_uri || '-'}
          </Text>
        </Descriptions.Item>
        <Descriptions.Item label="状态">
          <Tag color={statusColorMap[String(record.status || '')] || 'default'}>
            {statusLabelMap[String(record.status || '')] || record.status || '-'}
          </Tag>
        </Descriptions.Item>
        <Descriptions.Item label="响应码">{record.response_code ?? '-'}</Descriptions.Item>
        <Descriptions.Item label="耗时" span={2}>
          {record.cost_ms ?? '-'} ms
        </Descriptions.Item>
        <Descriptions.Item label="UserAgent" span={2}>
          <Text type="secondary" style={{ fontSize: 12, wordBreak: 'break-all' }}>
            {record.user_agent || '-'}
          </Text>
        </Descriptions.Item>
        <Descriptions.Item label="错误信息" span={2}>
          {record.error_msg ? <Text type="danger">{record.error_msg}</Text> : '-'}
        </Descriptions.Item>
        <Descriptions.Item label="请求体" span={2}>
          <pre
            style={{
              maxHeight: 260,
              overflow: 'auto',
              background: 'rgba(0,0,0,0.03)',
              padding: 8,
              borderRadius: 4,
              fontSize: 12,
              margin: 0,
            }}
          >
            {jsonBody || '(空)'}
          </pre>
        </Descriptions.Item>
      </Descriptions>
    </Modal>
  );
};

export default AuditLogList;
