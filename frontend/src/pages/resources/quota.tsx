import React, { useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Drawer,
  Empty,
  Form,
  Input,
  Popconfirm,
  Progress,
  Row,
  Select,
  Space,
  Tabs,
  Tag,
  Tooltip,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import Editor from '@monaco-editor/react';
import {
  KubernetesResource,
  useCreateResourceMutation,
  useDeleteResourceMutation,
  useListNamespacesQuery,
  useListResourcesQuery,
} from '@/app/services/resource';
import { useAppSelector } from '@/app/store';
import { usePermission } from '@/hooks/usePermission';

// ResourceQuota status 中的 key → 中文标签
const QUOTA_LABELS: Record<string, string> = {
  'cpu': 'CPU',
  'memory': '内存',
  'pods': 'Pod 数',
  'requests.cpu': '请求 CPU',
  'requests.memory': '请求内存',
  'limits.cpu': '限制 CPU',
  'limits.memory': '限制内存',
  'services': 'Service 数',
  'services.nodeports': 'NodePort 数',
  'services.loadbalancers': 'LoadBalancer 数',
  'configmaps': 'ConfigMap 数',
  'secrets': 'Secret 数',
  'persistentvolumeclaims': 'PVC 数',
  'replicationcontrollers': 'RC 数',
  'resourcequotas': 'ResourceQuota 数',
};

// 解析 k8s 资源量 (CPU=millicores, Memory=bytes) 为可读文本
function parseQuantity(v: string): number {
  if (!v) return 0;
  if (v.endsWith('m')) return parseInt(v.slice(0, -1), 10);
  return parseFloat(v) * 1000; // CPU cores → millicores
}

function formatQuantity(v: string | undefined): string {
  if (!v) return '-';
  return v;
}

// 计算使用率百分比
function calcPercent(used: string, hard: string): number {
  const u = parseQuantity(used);
  const h = parseQuantity(hard);
  if (h === 0) return 0;
  return Math.min(100, Math.round((u / h) * 100));
}

// ResourceQuota 列表组件
const ResourceQuotaList: React.FC<{
  clusterCode: string;
  namespace: string;
  canView: boolean;
  canCreate: boolean;
  canDelete: boolean;
  onReload: () => void;
}> = ({ clusterCode, namespace, canView, canCreate, canDelete, onReload }) => {
  const [createOpen, setCreateOpen] = useState(false);
  const [createYaml, setCreateYaml] = useState('');
  const [createResource, { isLoading: creating }] = useCreateResourceMutation();
  const [deleteResource] = useDeleteResourceMutation();

  const { data, refetch, isFetching } = useListResourcesQuery(
    {
      code: clusterCode,
      apiVersion: 'v1',
      kind: 'ResourceQuota',
      namespace,
      page: 1,
      size: 100,
    },
    { skip: !clusterCode || !namespace, refetchOnMountOrArgChange: true },
  );

  const handleCreate = async () => {
    if (!createYaml.trim()) {
      message.error('YAML 不能为空');
      return;
    }
    try {
      await createResource({
        code: clusterCode,
        apiVersion: 'v1',
        kind: 'ResourceQuota',
        yaml: createYaml,
      }).unwrap();
      message.success('创建成功');
      setCreateOpen(false);
      refetch();
    } catch {
      // interceptor
    }
  };

  const openCreate = () => {
    setCreateYaml(
      `apiVersion: v1
kind: ResourceQuota
metadata:
  name: quota-${namespace}
  namespace: ${namespace}
spec:
  hard:
    requests.cpu: "4"
    requests.memory: 8Gi
    limits.cpu: "8"
    limits.memory: 16Gi
    pods: "20"
    services: "10"
`,
    );
    setCreateOpen(true);
  };

  const columns: ProColumns<KubernetesResource>[] = [
    {
      title: '名称',
      dataIndex: ['metadata', 'name'],
      key: 'name',
      width: 200,
      render: (_v, item) => (
        <span style={{ fontWeight: 600 }}>{item.metadata?.name || '-'}</span>
      ),
    },
    {
      title: '命名空间',
      dataIndex: ['metadata', 'namespace'],
      key: 'namespace',
      width: 120,
      render: (_v, item) => item.metadata?.namespace || '-',
    },
    {
      title: '资源使用情况',
      key: 'usage',
      width: 600,
      render: (_v, item) => {
        const hard = (item.spec?.hard || {}) as Record<string, string>;
        const used = (item.status?.used || {}) as Record<string, string>;
        const keys = Object.keys(hard);
        if (keys.length === 0) return <Tag>无配额限制</Tag>;
        return (
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            {keys.map((key) => {
              const h = hard[key] || '-';
              const u = used[key] || '0';
              const pct = calcPercent(u, h);
              const color = pct >= 90 ? '#ff4d4f' : pct >= 70 ? '#faad14' : '#52c41a';
              return (
                <div key={key} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <Tooltip title={`${QUOTA_LABELS[key] || key}: ${u} / ${h}`}>
                    <span style={{ width: 100, fontSize: 12, color: 'rgba(0,0,0,0.65)' }}>
                      {QUOTA_LABELS[key] || key}
                    </span>
                  </Tooltip>
                  <Progress
                    percent={pct}
                    size="small"
                    strokeColor={color}
                    style={{ flex: 1, minWidth: 120 }}
                    format={() => `${u} / ${h}`}
                  />
                </div>
              );
            })}
          </Space>
        );
      },
    },
    {
      title: 'Age',
      dataIndex: ['metadata', 'creationTimestamp'],
      key: 'age',
      width: 100,
      render: (_v, item) => {
        const ts = item.metadata?.creationTimestamp;
        if (!ts) return '-';
        const diff = Date.now() - new Date(ts).getTime();
        const days = Math.floor(diff / 86400000);
        const hours = Math.floor((diff % 86400000) / 3600000);
        if (days > 0) return `${days}d${hours}h`;
        const mins = Math.floor((diff % 3600000) / 60000);
        return `${hours}h${mins}m`;
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      fixed: 'right',
      render: (_v, item) => (
        <Popconfirm
          title={`删除 ${item.metadata?.name}?`}
          onConfirm={async () => {
            if (!canDelete) return;
            try {
              await deleteResource({
                code: clusterCode,
                apiVersion: 'v1',
                kind: 'ResourceQuota',
                namespace: item.metadata?.namespace,
                name: item.metadata?.name || '',
              }).unwrap();
              message.success('删除成功');
              refetch();
            } catch {
              // interceptor
            }
          }}
        >
          <Button type="link" size="small" danger icon={<DeleteOutlined />} disabled={!canDelete}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <>
      <ProTable<KubernetesResource>
        headerTitle="ResourceQuota"
        rowKey={(item) => item.metadata?.uid || item.metadata?.name || ''}
        columns={columns}
        loading={isFetching}
        search={false}
        toolBarRender={() => [
          <Button key="reload" icon={<ReloadOutlined />} onClick={() => refetch()}>
            刷新
          </Button>,
          <Button
            key="create"
            type="primary"
            icon={<PlusOutlined />}
            onClick={openCreate}
            disabled={!canCreate}
          >
            创建配额
          </Button>,
        ]}
        dataSource={data?.items || []}
        pagination={false}
        scroll={{ x: 1000 }}
        locale={{ emptyText: !clusterCode ? '请先选择集群' : !namespace ? '请先选择命名空间' : '暂无 ResourceQuota' }}
      />
      <Drawer
        title="创建 ResourceQuota"
        width={800}
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setCreateOpen(false)}>取消</Button>
            <Button type="primary" loading={creating} onClick={handleCreate}>
              提交
            </Button>
          </Space>
        }
      >
        <div style={{ height: 'calc(100vh - 200px)', minHeight: 400 }}>
          <Editor
            height="100%"
            defaultLanguage="yaml"
            theme="vs"
            value={createYaml}
            onChange={(v) => setCreateYaml(v || '')}
            options={{
              minimap: { enabled: false },
              fontSize: 13,
              lineNumbers: 'on',
              automaticLayout: true,
              scrollBeyondLastLine: false,
              tabSize: 2,
            }}
          />
        </div>
      </Drawer>
    </>
  );
};

// LimitRange 列表组件
const LimitRangeList: React.FC<{
  clusterCode: string;
  namespace: string;
  canView: boolean;
  canCreate: boolean;
  canDelete: boolean;
}> = ({ clusterCode, namespace, canCreate, canDelete }) => {
  const [createOpen, setCreateOpen] = useState(false);
  const [createYaml, setCreateYaml] = useState('');
  const [createResource, { isLoading: creating }] = useCreateResourceMutation();
  const [deleteResource] = useDeleteResourceMutation();

  const { data, refetch, isFetching } = useListResourcesQuery(
    {
      code: clusterCode,
      apiVersion: 'v1',
      kind: 'LimitRange',
      namespace,
      page: 1,
      size: 100,
    },
    { skip: !clusterCode || !namespace, refetchOnMountOrArgChange: true },
  );

  const handleCreate = async () => {
    if (!createYaml.trim()) {
      message.error('YAML 不能为空');
      return;
    }
    try {
      await createResource({
        code: clusterCode,
        apiVersion: 'v1',
        kind: 'LimitRange',
        yaml: createYaml,
      }).unwrap();
      message.success('创建成功');
      setCreateOpen(false);
      refetch();
    } catch {
      // interceptor
    }
  };

  const openCreate = () => {
    setCreateYaml(
      `apiVersion: v1
kind: LimitRange
metadata:
  name: limits-${namespace}
  namespace: ${namespace}
spec:
  limits:
    - type: Container
      default:
        cpu: 500m
        memory: 512Mi
      defaultRequest:
        cpu: 100m
        memory: 128Mi
      max:
        cpu: "2"
        memory: 2Gi
      min:
        cpu: 50m
        memory: 64Mi
`,
    );
    setCreateOpen(true);
  };

  const columns: ProColumns<KubernetesResource>[] = [
    {
      title: '名称',
      dataIndex: ['metadata', 'name'],
      key: 'name',
      width: 200,
      render: (_v, item) => (
        <span style={{ fontWeight: 600 }}>{item.metadata?.name || '-'}</span>
      ),
    },
    {
      title: '命名空间',
      dataIndex: ['metadata', 'namespace'],
      key: 'namespace',
      width: 120,
      render: (_v, item) => item.metadata?.namespace || '-',
    },
    {
      title: '限制规则',
      key: 'limits',
      width: 600,
      render: (_v, item) => {
        const limits = (item.spec?.limits || []) as Array<Record<string, unknown>>;
        if (!limits || limits.length === 0) return <Tag>无限制</Tag>;
        return (
          <Space direction="vertical" size={4} style={{ width: '100%' }}>
            {limits.map((lim, idx) => {
              const type = lim.type as string;
              const def = lim.default as Record<string, string> | undefined;
              const req = lim.defaultRequest as Record<string, string> | undefined;
              const max = lim.max as Record<string, string> | undefined;
              const min = lim.min as Record<string, string> | undefined;
              const fields = ['cpu', 'memory'] as const;
              return (
                <Card
                  key={idx}
                  size="small"
                  type="inner"
                  title={<Tag color="blue">{type}</Tag>}
                  style={{ marginBottom: 4 }}
                >
                  <Row gutter={16}>
                    {fields.map((f) => (
                      <Col key={f} span={6}>
                        <div style={{ fontSize: 12 }}>
                          <span style={{ color: 'rgba(0,0,0,0.45)' }}>{f.toUpperCase()}</span>
                          <div style={{ marginTop: 2 }}>
                            {def?.[f] && <Tag color="cyan">默认: {def[f]}</Tag>}
                            {req?.[f] && <Tag color="blue">请求: {req[f]}</Tag>}
                            {max?.[f] && <Tag color="red">上限: {max[f]}</Tag>}
                            {min?.[f] && <Tag color="orange">下限: {min[f]}</Tag>}
                          </div>
                        </div>
                      </Col>
                    ))}
                  </Row>
                </Card>
              );
            })}
          </Space>
        );
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      fixed: 'right',
      render: (_v, item) => (
        <Popconfirm
          title={`删除 ${item.metadata?.name}?`}
          onConfirm={async () => {
            if (!canDelete) return;
            try {
              await deleteResource({
                code: clusterCode,
                apiVersion: 'v1',
                kind: 'LimitRange',
                namespace: item.metadata?.namespace,
                name: item.metadata?.name || '',
              }).unwrap();
              message.success('删除成功');
              refetch();
            } catch {
              // interceptor
            }
          }}
        >
          <Button type="link" size="small" danger icon={<DeleteOutlined />} disabled={!canDelete}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <>
      <ProTable<KubernetesResource>
        headerTitle="LimitRange"
        rowKey={(item) => item.metadata?.uid || item.metadata?.name || ''}
        columns={columns}
        loading={isFetching}
        search={false}
        toolBarRender={() => [
          <Button key="reload" icon={<ReloadOutlined />} onClick={() => refetch()}>
            刷新
          </Button>,
          <Button
            key="create"
            type="primary"
            icon={<PlusOutlined />}
            onClick={openCreate}
            disabled={!canCreate}
          >
            创建限制规则
          </Button>,
        ]}
        dataSource={data?.items || []}
        pagination={false}
        scroll={{ x: 1000 }}
        locale={{ emptyText: !clusterCode ? '请先选择集群' : !namespace ? '请先选择命名空间' : '暂无 LimitRange' }}
      />
      <Drawer
        title="创建 LimitRange"
        width={800}
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setCreateOpen(false)}>取消</Button>
            <Button type="primary" loading={creating} onClick={handleCreate}>
              提交
            </Button>
          </Space>
        }
      >
        <div style={{ height: 'calc(100vh - 200px)', minHeight: 400 }}>
          <Editor
            height="100%"
            defaultLanguage="yaml"
            theme="vs"
            value={createYaml}
            onChange={(v) => setCreateYaml(v || '')}
            options={{
              minimap: { enabled: false },
              fontSize: 13,
              lineNumbers: 'on',
              automaticLayout: true,
              scrollBeyondLastLine: false,
              tabSize: 2,
            }}
          />
        </div>
      </Drawer>
    </>
  );
};

const QuotaManagement: React.FC = () => {
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);
  const { hasPerm } = usePermission();
  const canView = hasPerm('resource:view') || hasPerm('resource:list');
  const canCreate = hasPerm('resource:create');
  const canDelete = hasPerm('resource:delete');

  const [namespace, setNamespace] = useState('default');
  const [activeTab, setActiveTab] = useState('resourcequota');

  const { data: nsData } = useListNamespacesQuery(selectedClusterCode || '', {
    skip: !selectedClusterCode,
    refetchOnMountOrArgChange: true,
  });
  const namespaceOptions = useMemo(
    () => (nsData || ['default']).map((n) => ({ label: n, value: n })),
    [nsData],
  );

  return (
    <PageContainer>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Space wrap align="center">
          <Select
            style={{ width: 240 }}
            placeholder="选择命名空间"
            value={namespace}
            onChange={setNamespace}
            options={namespaceOptions}
            showSearch
            optionFilterProp="label"
          />
          <Tag color="blue">集群: {selectedClusterCode || '未选择'}</Tag>
        </Space>

        {!selectedClusterCode ? (
          <Empty description="请先在顶部选择一个集群" />
        ) : (
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'resourcequota',
                label: '资源配额 (ResourceQuota)',
                children: (
                  <ResourceQuotaList
                    clusterCode={selectedClusterCode}
                    namespace={namespace}
                    canView={canView}
                    canCreate={canCreate}
                    canDelete={canDelete}
                    onReload={() => {}}
                  />
                ),
              },
              {
                key: 'limitrange',
                label: '限制范围 (LimitRange)',
                children: (
                  <LimitRangeList
                    clusterCode={selectedClusterCode}
                    namespace={namespace}
                    canView={canView}
                    canCreate={canCreate}
                    canDelete={canDelete}
                  />
                ),
              },
            ]}
          />
        )}
      </Space>
    </PageContainer>
  );
};

export default QuotaManagement;
