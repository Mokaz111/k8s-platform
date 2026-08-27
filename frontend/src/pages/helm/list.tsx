import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Checkbox,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Select,
  Space,
  Tabs,
  Tag,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns, ActionType } from '@ant-design/pro-components';
import Editor from '@monaco-editor/react';
import type { editor } from 'monaco-editor';
// 本地 Monaco 加载配置（替代 CDN，离线环境可用）
import '@/app/monaco';
import {
  DeleteOutlined,
  HistoryOutlined,
  PlusOutlined,
  ReloadOutlined,
  RollbackOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import {
  HelmRelease,
  InstallReleaseBody,
  useInstallReleaseMutation,
  useListHistoryQuery,
  useListReleasesQuery,
  useRollbackReleaseMutation,
  useUninstallReleaseMutation,
} from '@/app/services/helm';
import { useListClustersQuery } from '@/app/services/cluster';
import { useListNamespacesQuery } from '@/app/services/resource';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { setSelectedClusterCode } from '@/slices/appSlice';
import { usePermission } from '@/hooks/usePermission';

dayjs.extend(relativeTime);

const statusColorMap: Record<string, string> = {
  deployed: 'success',
  failed: 'error',
  pending: 'warning',
  superseded: 'default',
  uninstalled: 'default',
  uninstalling: 'processing',
  pending_install: 'warning',
  pending_upgrade: 'warning',
  pending_rollback: 'warning',
};

const statusLabelMap: Record<string, string> = {
  deployed: '已部署',
  failed: '失败',
  pending: '等待中',
  superseded: '已废弃',
  uninstalled: '已卸载',
  uninstalling: '卸载中',
  pending_install: '安装中',
  pending_upgrade: '升级中',
  pending_rollback: '回滚中',
};

const DEFAULT_VALUES_YAML = `# Helm values.yaml
# 示例：
# replicaCount: 1
# image:
#   repository: nginx
#   tag: alpine
# service:
#   type: ClusterIP
#   port: 80
`;

interface InstallFormValues {
  release_name: string;
  namespace: string;
  chart_ref: string;
  repo_url?: string;
  version?: string;
  values?: string;
  wait?: boolean;
  dry_run?: boolean;
}

const HelmList: React.FC = () => {
  const dispatch = useAppDispatch();
  const actionRef = React.useRef<ActionType>();
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);
  const { hasPerm } = usePermission();
  const canView = hasPerm('helm:view');
  const canInstall = hasPerm('helm:install');
  const canUninstall = hasPerm('helm:uninstall');
  const canRollback = hasPerm('helm:rollback');

  const { data: clustersData } = useListClustersQuery(undefined, {
    refetchOnMountOrArgChange: true,
  });
  const clusterOptions = useMemo(
    () =>
      (clustersData?.items || []).map((c) => ({
        label: c.name ? `${c.name} (${c.code})` : c.code,
        value: c.code,
      })),
    [clustersData],
  );

  const [clusterCode, setClusterCode] = useState<string | null>(selectedClusterCode);
  const [namespace, setNamespace] = useState<string>('');

  useEffect(() => {
    if (selectedClusterCode && !clusterCode) {
      setClusterCode(selectedClusterCode);
    }
  }, [selectedClusterCode, clusterCode]);

  const { data: namespaceData } = useListNamespacesQuery(clusterCode || '', {
    skip: !clusterCode,
    refetchOnMountOrArgChange: true,
  });
  const namespaceOptions = useMemo(() => {
    const base = namespaceData || ['default'];
    return [
      { label: '（全部命名空间）', value: '' },
      ...base.map((n) => ({ label: n, value: n })),
    ];
  }, [namespaceData]);

  const listParams = useMemo(
    () => ({
      clusterCode: clusterCode || '',
      namespace: namespace || undefined,
    }),
    [clusterCode, namespace],
  );

  const { data, refetch, isFetching } = useListReleasesQuery(listParams, {
    skip: !clusterCode,
    refetchOnMountOrArgChange: true,
  });
  const [installRelease, { isLoading: installLoading }] = useInstallReleaseMutation();
  const [uninstallRelease] = useUninstallReleaseMutation();
  const [rollbackRelease] = useRollbackReleaseMutation();

  // Install drawer
  const [installOpen, setInstallOpen] = useState(false);
  const [installForm] = Form.useForm<InstallFormValues>();
  const installValuesWatch = Form.useWatch('values', installForm) as string | undefined;
  const installEditorRef = React.useRef<editor.IStandaloneCodeEditor | null>(null);

  // History/rollback drawer
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyRecord, setHistoryRecord] = useState<HelmRelease | null>(null);

  const handleReload = useCallback(() => refetch(), [refetch]);

  const openInstallDrawer = () => {
    if (!clusterCode) {
      message.warning('请先选择集群');
      return;
    }
    if (!canInstall) {
      message.error('无 Helm 安装权限');
      return;
    }
    installForm.resetFields();
    installForm.setFieldsValue({
      release_name: '',
      namespace: namespace || 'default',
      chart_ref: '',
      repo_url: '',
      version: '',
      values: DEFAULT_VALUES_YAML,
      wait: false,
      dry_run: false,
    });
    setInstallOpen(true);
  };

  const handleInstallConfirm = async () => {
    if (!clusterCode) return;
    try {
      const values = await installForm.validateFields();
      const body: InstallReleaseBody = {
        release_name: values.release_name.trim(),
        namespace: values.namespace.trim(),
        chart_ref: values.chart_ref.trim(),
        repo_url: values.repo_url?.trim() || undefined,
        version: values.version?.trim() || undefined,
        values: values.values?.trim() || undefined,
        wait: !!values.wait,
        dry_run: !!values.dry_run,
      };
      const res = await installRelease({ clusterCode, body }).unwrap();
      const tip = values.dry_run
        ? 'Dry-run 验证完成'
        : `Release「${body.release_name}」安装/升级已提交`;
      message.success(tip);
      if (!values.dry_run) {
        setInstallOpen(false);
        handleReload();
      } else if (res?.output) {
        // dry-run 时把输出回填到 values 旁的可视区，便于检查
        // 这里仅提示，不修改表单
      }
    } catch {
      // interceptor
    }
  };

  const handleUninstall = async (record: HelmRelease) => {
    if (!clusterCode || !canUninstall) return;
    try {
      await uninstallRelease({
        clusterCode,
        namespace: record.namespace,
        name: record.name,
      }).unwrap();
      message.success(`Release「${record.name}」已卸载`);
      handleReload();
    } catch {
      // interceptor
    }
  };

  const handleRollback = async (record: HelmRelease, revision?: number) => {
    if (!clusterCode || !canRollback) return;
    try {
      await rollbackRelease({
        clusterCode,
        namespace: record.namespace,
        name: record.name,
        revision,
      }).unwrap();
      message.success(
        revision
          ? `已回滚 Release「${record.name}」到 revision ${revision}`
          : `Release「${record.name}」已回滚到上一版本`,
      );
      handleReload();
    } catch {
      // interceptor
    }
  };

  const openHistoryDrawer = (record: HelmRelease) => {
    if (!canView) {
      message.error('无 Helm 查看权限');
      return;
    }
    setHistoryRecord(record);
    setHistoryOpen(true);
  };

  const columns: ProColumns<HelmRelease>[] = useMemo(
    () => [
      {
        title: 'Release',
        dataIndex: 'name',
        key: 'name',
        width: 220,
        fixed: 'left',
        render: (_v, item) => (
          <Space>
            <ThunderboltOutlined style={{ color: '#1677ff' }} />
            <span style={{ fontWeight: 600 }}>{item.name}</span>
          </Space>
        ),
      },
      {
        title: '命名空间',
        dataIndex: 'namespace',
        key: 'namespace',
        width: 140,
        render: (_v, item) => <Tag color="blue">{item.namespace}</Tag>,
      },
      {
        title: '版本',
        dataIndex: 'revision',
        key: 'revision',
        width: 90,
        render: (_v, item) => <Tag color="purple">rev {item.revision || '-'}</Tag>,
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 120,
        render: (_v, item) => {
          const status = (item.status || '').toLowerCase();
          const color = statusColorMap[status] || 'default';
          const label = statusLabelMap[status] || item.status || '-';
          return <Tag color={color}>{label}</Tag>;
        },
      },
      {
        title: 'Chart',
        dataIndex: 'chart',
        key: 'chart',
        width: 200,
        render: (_v, item) => (
          <Space direction="vertical" size={0}>
            <span style={{ wordBreak: 'break-all' }}>{item.chart || '-'}</span>
            {item.appVersion && (
              <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
                app: {item.appVersion}
              </span>
            )}
          </Space>
        ),
      },
      {
        title: '更新时间',
        dataIndex: 'updated',
        key: 'updated',
        width: 180,
        render: (_v, item) => {
          if (!item.updated) return '-';
          const t = dayjs(item.updated);
          if (!t.isValid()) return item.updated;
          return (
            <Space direction="vertical" size={0}>
              <span>{t.format('YYYY-MM-DD HH:mm')}</span>
              <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>{t.fromNow()}</span>
            </Space>
          );
        },
      },
      {
        title: '操作',
        key: 'actions',
        width: 280,
        fixed: 'right',
        render: (_v, item) => (
          <Space size="small" wrap>
            <Button
              type="link"
              size="small"
              icon={<HistoryOutlined />}
              onClick={() => openHistoryDrawer(item)}
              disabled={!canView}
            >
              历史/回滚
            </Button>
            <Popconfirm
              title={`确认回滚 Release「${item.name}」到上一版本？`}
              description="将创建新的 revision 并恢复至上一版本"
              okButtonProps={{ disabled: !canRollback }}
              onConfirm={() => handleRollback(item)}
            >
              <Button
                type="link"
                size="small"
                icon={<RollbackOutlined />}
                disabled={!canRollback}
              >
                快速回滚
              </Button>
            </Popconfirm>
            <Popconfirm
              title={`确认卸载 Release「${item.name}」？`}
              description="卸载将删除该 Release 关联的所有资源"
              okButtonProps={{ danger: true, disabled: !canUninstall }}
              onConfirm={() => handleUninstall(item)}
            >
              <Button
                type="link"
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={!canUninstall}
              >
                卸载
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ],
    [
      clusterCode,
      canView,
      canInstall,
      canUninstall,
      canRollback,
      handleReload,
      handleUninstall,
      handleRollback,
    ],
  );

  return (
    <PageContainer>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Space wrap align="center">
          <Select
            style={{ width: 240 }}
            placeholder="选择集群"
            value={clusterCode || undefined}
            loading={!clustersData}
            onChange={(v) => {
              setClusterCode(v || null);
              if (v) dispatch(setSelectedClusterCode(v));
              setNamespace('');
            }}
            options={clusterOptions}
            showSearch
            optionFilterProp="label"
            allowClear
          />
          <Select
            style={{ width: 220 }}
            placeholder="命名空间"
            value={namespace}
            onChange={(v) => setNamespace(v || '')}
            options={namespaceOptions}
            allowClear
            showSearch
            optionFilterProp="label"
          />
          <Button icon={<ReloadOutlined />} onClick={handleReload} loading={isFetching}>
            刷新
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={openInstallDrawer}
            disabled={!clusterCode || !canInstall}
          >
            安装 / 升级 Release
          </Button>
        </Space>

        <ProTable<HelmRelease>
          headerTitle={
            <Space>
              <ThunderboltOutlined />
              <span>Helm Releases</span>
              {clusterCode && <Tag color="blue">集群: {clusterCode}</Tag>}
            </Space>
          }
          actionRef={actionRef}
          rowKey={(item) => `${item.namespace}/${item.name}`}
          columns={columns}
          loading={isFetching || !clusterCode}
          search={false}
          toolBarRender={false}
          dataSource={!clusterCode ? [] : data?.items || []}
          pagination={{
            current: 1,
            pageSize: 50,
            total: data?.total || 0,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50', '100'],
          }}
          scroll={{ x: 1200 }}
          options={{
            reload: handleReload,
            density: true,
            fullScreen: true,
            setting: true,
          }}
          locale={{
            emptyText: !clusterCode ? '请先选择集群' : '该集群下暂无 Helm Release',
          }}
        />
      </Space>

      <Drawer
        title={
          <Space>
            <PlusOutlined />
            <span>安装 / 升级 Helm Release</span>
            {clusterCode && <Tag color="blue">集群: {clusterCode}</Tag>}
          </Space>
        }
        width={960}
        open={installOpen}
        onClose={() => setInstallOpen(false)}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setInstallOpen(false)}>取消</Button>
            <Button
              type="primary"
              icon={<ThunderboltOutlined />}
              loading={installLoading}
              onClick={handleInstallConfirm}
            >
              提交
            </Button>
          </Space>
        }
      >
        <Form<InstallFormValues>
          form={installForm}
          layout="vertical"
          requiredMark
        >
          <Space style={{ width: '100%' }} size="middle">
            <Form.Item
              name="release_name"
              label="Release 名称"
              rules={[{ required: true, message: '请输入 release 名称' }]}
              style={{ flex: 1, minWidth: 220 }}
            >
              <Input placeholder="如 my-app" allowClear />
            </Form.Item>
            <Form.Item
              name="namespace"
              label="命名空间"
              rules={[{ required: true, message: '请输入命名空间' }]}
              style={{ flex: 1, minWidth: 220 }}
            >
              <Select
                placeholder="选择或输入命名空间"
                options={namespaceOptions.filter((o) => o.value !== '')}
                showSearch
                allowClear
                filterOption={(input, option) =>
                  (option?.label as string)?.toLowerCase().includes(input.toLowerCase()) ?? false
                }
              />
            </Form.Item>
          </Space>

          <Form.Item
            name="chart_ref"
            label="Chart 引用"
            rules={[{ required: true, message: '请输入 chart 引用' }]}
            help="如 stable/nginx-ingress、bitnami/redis、oci://registry/chart、或本地路径 ./charts/my-app"
          >
            <Input placeholder="如 bitnami/redis 或 oci://registry/charts/app" allowClear />
          </Form.Item>

          <Space style={{ width: '100%' }} size="middle">
            <Form.Item
              name="repo_url"
              label="Repo URL（可选）"
              style={{ flex: 1, minWidth: 320 }}
              help="首次引用仓库时需要，如 https://charts.bitnami.com/bitnami"
            >
              <Input
                placeholder="https://charts.bitnami.com/bitnami"
                allowClear
              />
            </Form.Item>
            <Form.Item
              name="version"
              label="Chart Version（可选）"
              style={{ flex: 1, minWidth: 220 }}
              help="指定 chart 版本，留空则用最新"
            >
              <Input placeholder="如 18.1.0" allowClear />
            </Form.Item>
          </Space>

          <Form.Item name="values" label="Values (YAML)">
            <div style={{ height: 380, border: '1px solid #d9d9d9', borderRadius: 6 }}>
              <Editor
                height="100%"
                defaultLanguage="yaml"
                language="yaml"
                theme="light"
                value={installValuesWatch ?? DEFAULT_VALUES_YAML}
                onMount={(ed) => (installEditorRef.current = ed)}
                onChange={(v) =>
                  installForm.setFieldValue('values', v ?? '')
                }
                options={{
                  minimap: { enabled: false },
                  fontSize: 13,
                  lineNumbers: 'on',
                  automaticLayout: true,
                  scrollBeyondLastLine: false,
                  tabSize: 2,
                  insertSpaces: true,
                  wordWrap: 'on',
                }}
              />
            </div>
          </Form.Item>

          <Form.Item name="wait" valuePropName="checked">
            <Checkbox>等待就绪 (--wait)：阻塞至所有资源就绪才算成功</Checkbox>
          </Form.Item>
          <Form.Item name="dry_run" valuePropName="checked">
            <Checkbox>Dry-run (--dry-run)：仅模拟，不实际变更集群</Checkbox>
          </Form.Item>
        </Form>
      </Drawer>

      <Drawer
        title={
          <Space>
            <HistoryOutlined />
            <span>Release 历史 / 回滚</span>
            {historyRecord && (
              <>
                <Tag color="blue">ns: {historyRecord.namespace}</Tag>
                <Tag color="purple">{historyRecord.name}</Tag>
              </>
            )}
          </Space>
        }
        width={920}
        open={historyOpen}
        onClose={() => {
          setHistoryOpen(false);
          setHistoryRecord(null);
        }}
        destroyOnClose
      >
        <HistoryRollbackPanel
          clusterCode={clusterCode || ''}
          record={historyRecord}
          canRollback={canRollback}
          onRollback={handleRollback}
        />
      </Drawer>
    </PageContainer>
  );
};

// ---------- 子组件：历史与回滚面板 ----------

interface HistoryRollbackPanelProps {
  clusterCode: string;
  record: HelmRelease | null;
  canRollback: boolean;
  onRollback: (record: HelmRelease, revision?: number) => void;
}

const HistoryRollbackPanel: React.FC<HistoryRollbackPanelProps> = ({
  clusterCode,
  record,
  canRollback,
  onRollback,
}) => {
  const [activeTab, setActiveTab] = useState<'history' | 'output'>('history');

  const {
    data: historyData,
    isLoading,
    refetch,
  } = useListHistoryQuery(
    {
      clusterCode,
      namespace: record?.namespace || '',
      name: record?.name || '',
    },
    { skip: !record || !clusterCode, refetchOnMountOrArgChange: true },
  );

  if (!record) {
    return <div style={{ color: 'rgba(0,0,0,0.45)' }}>请选择一个 Release</div>;
  }

  const items = (historyData?.items || []) as Array<{
    revision: number;
    status: string;
    chart: string;
    app_version?: string;
    description?: string;
    updated?: string;
  }>;

  const historyColumns: ProColumns<{
    revision: number;
    status: string;
    chart: string;
    app_version?: string;
    description?: string;
    updated?: string;
  }>[] = [
    {
      title: 'Revision',
      dataIndex: 'revision',
      key: 'revision',
      width: 100,
      render: (_v, item) => <Tag color="purple">rev {item.revision}</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (_v, item) => {
        const status = (item.status || '').toLowerCase();
        const color = statusColorMap[status] || 'default';
        const label = statusLabelMap[status] || item.status || '-';
        return <Tag color={color}>{label}</Tag>;
      },
    },
    {
      title: 'Chart',
      dataIndex: 'chart',
      key: 'chart',
      width: 220,
      render: (_v, item) => (
        <Space direction="vertical" size={0}>
          <span style={{ wordBreak: 'break-all' }}>{item.chart || '-'}</span>
          {item.app_version && (
            <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
              app: {item.app_version}
            </span>
          )}
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      render: (_v, item) => item.description || '-',
    },
    {
      title: '更新时间',
      dataIndex: 'updated',
      key: 'updated',
      width: 180,
      render: (_v, item) => {
        if (!item.updated) return '-';
        const t = dayjs(item.updated);
        if (!t.isValid()) return item.updated;
        return t.format('YYYY-MM-DD HH:mm:ss');
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      fixed: 'right',
      render: (_v, item) => {
        const current = String(record.revision) === String(item.revision);
        return (
          <Popconfirm
            title={`回滚到 revision ${item.revision}？`}
            description="将创建新的 revision 并恢复至此版本"
            okButtonProps={{ disabled: !canRollback || current }}
            onConfirm={() => onRollback(record, item.revision)}
          >
            <Button
              type="link"
              size="small"
              icon={<RollbackOutlined />}
              disabled={!canRollback || current}
            >
              {current ? '当前版本' : '回滚到此'}
            </Button>
          </Popconfirm>
        );
      },
    },
  ];

  return (
    <Tabs
      activeKey={activeTab}
      onChange={(k) => setActiveTab(k as 'history' | 'output')}
      items={[
        {
          key: 'history',
          label: '修订历史',
          children: (
            <ProTable<{
              revision: number;
              status: string;
              chart: string;
              app_version?: string;
              description?: string;
              updated?: string;
            }>
              rowKey={(item) => `rev-${item.revision}`}
              columns={historyColumns}
              loading={isLoading}
              dataSource={items}
              search={false}
              toolBarRender={() => [
                <Button
                  key="refresh"
                  icon={<ReloadOutlined />}
                  onClick={() => refetch()}
                  loading={isLoading}
                >
                  刷新
                </Button>,
              ]}
              pagination={false}
              scroll={{ x: 900 }}
              options={false}
            />
          ),
        },
        {
          key: 'output',
          label: 'Release 概要',
          children: (
            <Space direction="vertical" style={{ width: '100%' }} size="middle">
              <DescriptionsCompact
                items={[
                  { label: '集群', value: clusterCode },
                  { label: '命名空间', value: record.namespace },
                  { label: 'Release', value: record.name },
                  { label: '当前 Revision', value: record.revision },
                  { label: '状态', value: record.status },
                  { label: 'Chart', value: record.chart },
                  { label: 'App Version', value: record.appVersion || '-' },
                  { label: '最近更新', value: record.updated || '-' },
                ]}
              />
              <Space>
                <Popconfirm
                  title={`回滚 Release「${record.name}」到上一版本？`}
                  description="将创建新的 revision 并恢复至上一版本"
                  okButtonProps={{ disabled: !canRollback }}
                  onConfirm={() => onRollback(record)}
                >
                  <Button
                    type="primary"
                    icon={<RollbackOutlined />}
                    disabled={!canRollback}
                  >
                    回滚到上一版本
                  </Button>
                </Popconfirm>
              </Space>
            </Space>
          ),
        },
      ]}
    />
  );
};

// ---------- 子组件：紧凑的描述列表 ----------

interface DescriptionsItem {
  label: string;
  value: React.ReactNode;
}

const DescriptionsCompact: React.FC<{ items: DescriptionsItem[] }> = ({ items }) => {
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(2, 1fr)',
        gap: 8,
      }}
    >
      {items.map((it, idx) => (
        <div
          key={idx}
          style={{
            border: '1px solid #f0f0f0',
            borderRadius: 6,
            padding: '8px 12px',
            background: '#fafafa',
          }}
        >
          <div style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12, marginBottom: 4 }}>
            {it.label}
          </div>
          <div style={{ wordBreak: 'break-all' }}>{it.value ?? '-'}</div>
        </div>
      ))}
    </div>
  );
};

export default HelmList;
