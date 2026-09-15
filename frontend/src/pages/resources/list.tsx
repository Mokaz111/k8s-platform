import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Drawer,
  Input,
  Popconfirm,
  Radio,
  Select,
  Space,
  Tabs,
  Tag,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns, ActionType } from '@ant-design/pro-components';
import {
  CloudServerOutlined,
  CodeOutlined,
  DeleteOutlined,
  EditOutlined,
  FileTextOutlined,
  HistoryOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import {
  KubernetesResource,
  useCreateResourceMutation,
  useDeleteResourceMutation,
  useListResourcesQuery,
} from '@/app/services/resource';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { setSelectedClusterCode } from '@/slices/appSlice';
import { usePermission } from '@/hooks/usePermission';
import { useAllowedNamespaces } from '@/hooks/useAllowedNamespaces';
import { PodLogsViewer } from '@/components/ws';
import { YamlEditor } from '@/components/YamlEditor';
import { ALL_KINDS_MAP, KIND_GROUPS, type ResourceTab } from '@/constants/k8sKinds';
import { getWorkloadStatus } from '@/utils/workloadStatus';

dayjs.extend(relativeTime);

const CLUSTER_SCOPED_NS = '__cluster__';

const defaultYamlTemplate = (apiVersion: string, kind: string, namespace?: string, name?: string): string => {
  const lines = [
    `apiVersion: ${apiVersion}`,
    `kind: ${kind}`,
    'metadata:',
  ];
  if (name) {
    lines.push(`  name: ${name}`);
  } else {
    lines.push(`  name: example-${kind.toLowerCase()}`);
  }
  if (!ALL_KINDS_MAP[kind]?.clusterScoped && namespace && namespace !== CLUSTER_SCOPED_NS) {
    lines.push(`  namespace: ${namespace}`);
  }
  lines.push('  labels:');
  lines.push('    app: example');
  if (kind === 'Deployment') {
    lines.push('spec:');
    lines.push('  replicas: 1');
    lines.push('  selector:');
    lines.push('    matchLabels:');
    lines.push('      app: example');
    lines.push('  template:');
    lines.push('    metadata:');
    lines.push('      labels:');
    lines.push('        app: example');
    lines.push('    spec:');
    lines.push('      containers:');
    lines.push('        - name: main');
    lines.push('          image: nginx:alpine');
    lines.push('          ports:');
    lines.push('            - containerPort: 80');
  } else if (kind === 'Service') {
    lines.push('spec:');
    lines.push('  type: ClusterIP');
    lines.push('  selector:');
    lines.push('    app: example');
    lines.push('  ports:');
    lines.push('    - port: 80');
    lines.push('      targetPort: 80');
  } else if (kind === 'ConfigMap') {
    lines.push('data:');
    lines.push('  key1: value1');
    lines.push('  key2: value2');
  } else if (kind === 'Secret') {
    lines.push('type: Opaque');
    lines.push('data:');
    lines.push('  # 注意：Secret data 需要 base64 编码');
    lines.push('  password: cGFzc3dvcmQ=');
  } else if (kind === 'Namespace') {
    // Namespace 不支持 namespace 和 labels 后面的内容
    return `apiVersion: v1\nkind: Namespace\nmetadata:\n  name: ${name || 'example-namespace'}\n`;
  } else {
    lines.push('spec: {}');
  }
  return lines.join('\n') + '\n';
};

const ResourceList: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const actionRef = React.useRef<ActionType>();
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);
  const [searchParams, setSearchParams] = useSearchParams();
  const { hasPerm } = usePermission();
  const canView = hasPerm('resource:view') || hasPerm('resource:get') || hasPerm('resource:list');
  const canCreate = hasPerm('resource:create');
  const canUpdate = hasPerm('resource:update') || hasPerm('resource:edit');
  const canDelete = hasPerm('resource:delete');
  const canBackup = hasPerm('backup:create');

  const clusterCodeParam = searchParams.get('cluster_code');
  const tabParam = (searchParams.get('tab') as ResourceTab) || 'workload';
  const kindParam = searchParams.get('kind') || KIND_GROUPS[0].kinds[0].value;
  const namespaceParam = searchParams.get('namespace') || '';

  const [clusterCode, setClusterCode] = useState<string | null>(
    clusterCodeParam || selectedClusterCode,
  );
  const [tab, setTab] = useState<ResourceTab>(tabParam);
  const [kind, setKind] = useState<string>(kindParam);
  const [namespace, setNamespace] = useState<string>(namespaceParam);
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);

  // 创建资源 Drawer
  const [createDrawerOpen, setCreateDrawerOpen] = useState(false);
  const [createYaml, setCreateYaml] = useState<string>('');

  // Pod 日志 Drawer
  const [logDrawerOpen, setLogDrawerOpen] = useState(false);
  const [currentPod, setCurrentPod] = useState<{
    clusterCode: string;
    namespace: string;
    name: string;
  } | null>(null);

  useEffect(() => {
    const newCode = clusterCodeParam || selectedClusterCode;
    if (newCode && newCode !== clusterCode) {
      setClusterCode(newCode);
      if (selectedClusterCode !== newCode) {
        dispatch(setSelectedClusterCode(newCode));
      }
    }
  }, [clusterCodeParam, selectedClusterCode, clusterCode, dispatch]);

  useEffect(() => {
    const params: Record<string, string> = {
      tab,
      kind,
    };
    if (clusterCode) params.cluster_code = clusterCode;
    if (namespace) params.namespace = namespace;
    setSearchParams(params, { replace: true });
  }, [clusterCode, tab, kind, namespace, setSearchParams]);

  const currentGroup = KIND_GROUPS.find((g) => g.key === tab) || KIND_GROUPS[0];
  const kindMeta = ALL_KINDS_MAP[kind] || currentGroup.kinds[0];
  const apiVersion = kindMeta.apiVersion;
  const isClusterScoped = !!kindMeta.clusterScoped;

  const { namespaces: namespaceData, isFullCluster } = useAllowedNamespaces(
    isClusterScoped ? undefined : clusterCode,
  );
  const namespaceOptions = useMemo(() => {
    if (isClusterScoped) {
      return [{ label: '（集群级资源，无 Namespace）', value: CLUSTER_SCOPED_NS }];
    }
    const base = namespaceData.length > 0 ? namespaceData : isFullCluster ? ['default'] : [];
    const opts = base.map((n) => ({ label: n, value: n }));
    if (isFullCluster) {
      return [{ label: '（全部命名空间）', value: '' }, ...opts];
    }
    return opts;
  }, [namespaceData, isClusterScoped, isFullCluster]);

  // 切换 kind 如果是集群级，自动清 namespace 为 __cluster__
  useEffect(() => {
    if (isClusterScoped && namespace !== CLUSTER_SCOPED_NS) {
      setNamespace(CLUSTER_SCOPED_NS);
    }
    if (!isClusterScoped && namespace === CLUSTER_SCOPED_NS) {
      setNamespace(isFullCluster ? '' : namespaceData[0] || '');
    }
  }, [isClusterScoped, namespace, isFullCluster, namespaceData]);

  useEffect(() => {
    if (
      !isClusterScoped &&
      !isFullCluster &&
      namespaceData.length > 0 &&
      !namespaceData.includes(namespace)
    ) {
      setNamespace(namespaceData[0]);
    }
  }, [isClusterScoped, isFullCluster, namespaceData, namespace]);

  const effectiveNs = isClusterScoped ? undefined : namespace || undefined;

  const listParams = useMemo(
    () => ({
      code: clusterCode || '',
      apiVersion,
      kind,
      namespace: effectiveNs,
      page,
      size,
      keyword: keyword || undefined,
    }),
    [clusterCode, apiVersion, kind, effectiveNs, page, size, keyword],
  );

  const { data, refetch, isFetching } = useListResourcesQuery(listParams, {
    skip: !canView || !clusterCode || (!isClusterScoped && !isFullCluster && !namespace),
    refetchOnMountOrArgChange: true,
  });

  const [deleteResource] = useDeleteResourceMutation();
  const [createResource, { isLoading: createLoading }] = useCreateResourceMutation();

  const handleReload = useCallback(() => {
    refetch();
  }, [refetch]);

  const openCreateDrawer = () => {
    if (!clusterCode) {
      message.warning('请先选择一个集群');
      return;
    }
    if (!canCreate) {
      message.error('无创建资源权限');
      return;
    }
    const tmpl = defaultYamlTemplate(
      apiVersion,
      kind,
      isClusterScoped ? undefined : namespace || 'default',
    );
    setCreateYaml(tmpl);
    setCreateDrawerOpen(true);
  };

  const handleCreateConfirm = async () => {
    if (!clusterCode) return;
    if (!createYaml.trim()) {
      message.error('YAML 内容不能为空');
      return;
    }
    try {
      await createResource({
        code: clusterCode,
        apiVersion,
        kind,
        yaml: createYaml,
      }).unwrap();
      message.success('创建成功');
      setCreateDrawerOpen(false);
      handleReload();
    } catch {
      // interceptor handles errors
    }
  };

  const editUrl = (item: KubernetesResource): string => {
    const itemNs = item.metadata?.namespace;
    const ns = itemNs ? encodeURIComponent(itemNs) : encodeURIComponent('_');
    const n = encodeURIComponent(item.metadata?.name || '');
    const k = encodeURIComponent(kind);
    const av = encodeURIComponent(apiVersion);
    return `/resources/${clusterCode}/${av}/${k}/${ns}/${n}/edit`;
  };

  const backupUrl = (item: KubernetesResource): string => {
    const ns = item.metadata?.namespace || '';
    const n = item.metadata?.name || '';
    const params = new URLSearchParams({
      code: clusterCode || '',
      kind,
      apiVersion,
      name: n,
    });
    if (ns) params.set('namespace', ns);
    return `/backups/create?${params.toString()}`;
  };

  const columns: ProColumns<KubernetesResource>[] = useMemo(
    () => [
      {
        title: '名称',
        dataIndex: ['metadata', 'name'],
        key: 'name',
        width: 260,
        fixed: 'left',
        render: (_v, item) => {
          const labels = item.metadata?.labels as Record<string, string> | undefined;
          const labelEntries = labels
            ? Object.entries(labels).slice(0, 3)
            : [];
          return (
            <Space direction="vertical" size={0} style={{ maxWidth: '100%' }}>
              <a
                style={{ fontWeight: 600, wordBreak: 'break-all' }}
                onClick={() => {
                  if (!canView && !canUpdate) {
                    message.error('无查看资源权限');
                    return;
                  }
                  navigate(editUrl(item));
                }}
              >
                {item.metadata?.name || '-'}
              </a>
              {labelEntries.length > 0 && (
                <Space size={[4, 4]} wrap style={{ maxWidth: '100%' }}>
                  {labelEntries.map(([k, v]) => (
                    <Tag
                      key={k}
                      style={{
                        fontSize: 11,
                        lineHeight: '16px',
                        paddingInline: 4,
                      }}
                    >
                      {k}={v}
                    </Tag>
                  ))}
                </Space>
              )}
            </Space>
          );
        },
      },
      {
        title: '命名空间',
        dataIndex: ['metadata', 'namespace'],
        key: 'namespace',
        width: 140,
        render: (_dom, record) => record.metadata?.namespace || <Tag>（集群级）</Tag>,
      },
      {
        title: '状态',
        key: 'status',
        width: 180,
        render: (_dom, record) => {
          const st = getWorkloadStatus(kind, record);
          return (
            <Space direction="vertical" size={0}>
              <Tag color={st.color}>{st.text}</Tag>
              {st.extra && (
                <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>{st.extra}</span>
              )}
            </Space>
          );
        },
      },
      {
        title: 'Age',
        dataIndex: ['metadata', 'creationTimestamp'],
        key: 'age',
        width: 140,
        render: (_dom, record) => {
          const v = record.metadata?.creationTimestamp;
          if (!v) return '-';
          const t = dayjs(v);
          return (
            <Space direction="vertical" size={0}>
              <span>{t.fromNow()}</span>
              <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>
                {t.format('YYYY-MM-DD HH:mm')}
              </span>
            </Space>
          );
        },
      },
      {
        title: '操作',
        key: 'actions',
        width: 340,
        fixed: 'right',
        render: (_v, item) => {
          const ns = item.metadata?.namespace;
          const name = item.metadata?.name || '';
          const canViewItem = canView || canUpdate;
          const isPod = item.kind === 'Pod';
          return (
            <Space size="small" wrap>
              <Button
                type="link"
                size="small"
                icon={<FileTextOutlined />}
                onClick={() => {
                  if (!canViewItem) {
                    message.error('无查看资源权限');
                    return;
                  }
                  navigate(editUrl(item));
                }}
                disabled={!canViewItem}
              >
                查看 YAML
              </Button>
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                onClick={() => {
                  if (!canUpdate) {
                    message.error('无编辑资源权限');
                    return;
                  }
                  navigate(editUrl(item));
                }}
                disabled={!canUpdate}
              >
                编辑
              </Button>
              <Button
                type="link"
                size="small"
                icon={<HistoryOutlined />}
                onClick={() => {
                  if (!canViewItem) {
                    message.error('无查看资源权限');
                    return;
                  }
                  navigate(editUrl(item) + '?tab=versions');
                }}
                disabled={!canViewItem}
              >
                版本历史
              </Button>
              {isPod && (
                <Button
                  type="link"
                  size="small"
                  icon={<CodeOutlined />}
                  onClick={() => {
                    if (!canViewItem) {
                      message.error('无查看资源权限');
                      return;
                    }
                    if (!clusterCode) {
                      message.warning('请先选择集群');
                      return;
                    }
                    setCurrentPod({
                      clusterCode: clusterCode,
                      namespace: ns || '',
                      name: name,
                    });
                    setLogDrawerOpen(true);
                  }}
                  disabled={!canViewItem}
                >
                  查看日志
                </Button>
              )}
              <Popconfirm
                title={`确定删除 ${kind}「${name}」?`}
                description="删除后该资源将从集群中移除"
                okButtonProps={{ danger: true, disabled: !canDelete }}
                onConfirm={async () => {
                  if (!canDelete) return;
                  try {
                    await deleteResource({
                      code: clusterCode || '',
                      apiVersion,
                      kind,
                      namespace: ns,
                      name,
                    }).unwrap();
                    message.success('删除成功');
                    handleReload();
                  } catch {
                    // interceptor
                  }
                }}
              >
                <Button
                  type="link"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  disabled={!canDelete}
                >
                  删除
                </Button>
              </Popconfirm>
              <Button
                type="link"
                size="small"
                icon={<CloudServerOutlined />}
                onClick={() => {
                  if (!canBackup) {
                    message.error('无创建备份权限');
                    return;
                  }
                  navigate(backupUrl(item));
                }}
                disabled={!canBackup}
              >
                备份
              </Button>
            </Space>
          );
        },
      },
    ],
    [
      kind,
      clusterCode,
      apiVersion,
      deleteResource,
      handleReload,
      navigate,
      canView,
      canUpdate,
      canDelete,
      canBackup,
    ],
  );

  const emptyCluster = !clusterCode;

  return (
    <PageContainer>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Tabs
          activeKey={tab}
          onChange={(k) => {
            const newTab = k as ResourceTab;
            const group = KIND_GROUPS.find((g) => g.key === newTab);
            setTab(newTab);
            const firstKind = group?.kinds[0];
            setKind(firstKind?.value || KIND_GROUPS[0].kinds[0].value);
            setPage(1);
          }}
          items={KIND_GROUPS.map((g) => ({ key: g.key, label: g.label }))}
        />

        <Space wrap align="center">
          <Select
            style={{ width: 200 }}
            placeholder="命名空间"
            value={isClusterScoped ? CLUSTER_SCOPED_NS : namespace}
            disabled={isClusterScoped}
            onChange={(v) => {
              setNamespace(v);
              setPage(1);
            }}
            options={namespaceOptions}
            allowClear={!isClusterScoped}
          />
          <Radio.Group
            value={kind}
            onChange={(e) => {
              setKind(e.target.value);
              setPage(1);
            }}
            optionType="button"
            buttonStyle="solid"
            options={currentGroup.kinds.map((k) => ({ label: k.label, value: k.value }))}
          />
          <Input.Search
            style={{ width: 280 }}
            allowClear
            placeholder="按名称关键字搜索"
            onSearch={(v) => {
              setKeyword(v);
              setPage(1);
            }}
            onPressEnter={(e) => {
              const target = e.currentTarget;
              setKeyword(target.value);
              setPage(1);
            }}
          />
          <Button icon={<ReloadOutlined />} onClick={handleReload}>
            刷新
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={openCreateDrawer}
            disabled={emptyCluster || !canCreate}
          >
            创建 {kind}
          </Button>
        </Space>

        <ProTable<KubernetesResource>
          headerTitle={
            <Space>
              <SaveOutlined />
              <span>{currentGroup.label} / {kind}</span>
              {isClusterScoped && <Tag color="orange">集群级</Tag>}
              {clusterCode && (
                <Tag color="blue">集群: {clusterCode}</Tag>
              )}
            </Space>
          }
          actionRef={actionRef}
          rowKey={(item) =>
            `${item.metadata?.namespace || ''}/${item.metadata?.name || ''}/${item.metadata?.uid || ''}`
          }
          columns={columns}
          loading={isFetching || emptyCluster}
          search={false}
          toolBarRender={false}
          dataSource={emptyCluster ? [] : data?.items || []}
          pagination={{
            current: data?.page || page,
            pageSize: data?.size || size,
            total: data?.total || 0,
            showSizeChanger: true,
            onChange: (p, s) => {
              setPage(p);
              setSize(s);
            },
          }}
          scroll={{ x: 1480 }}
          options={{
            reload: handleReload,
            density: true,
            fullScreen: true,
            setting: true,
          }}
          locale={{
            emptyText: emptyCluster ? '请先在顶部选择一个集群' : '暂无数据',
          }}
        />
      </Space>

      <Drawer
        title={
          <Space>
            <PlusOutlined />
            <span>创建 {kind}</span>
            {clusterCode && <Tag color="blue">集群: {clusterCode}</Tag>}
            {!isClusterScoped && namespace && <Tag>ns: {namespace}</Tag>}
          </Space>
        }
        width={900}
        open={createDrawerOpen}
        onClose={() => setCreateDrawerOpen(false)}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setCreateDrawerOpen(false)}>取消</Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={createLoading}
              onClick={handleCreateConfirm}
            >
              提交创建
            </Button>
          </Space>
        }
      >
        <YamlEditor
          value={createYaml}
          onChange={setCreateYaml}
          height="calc(100vh - 220px)"
          editorKey={`create-${kind}`}
        />
      </Drawer>

      <Drawer
        title={
          <Space>
            <CodeOutlined />
            <span>Pod 日志</span>
            {currentPod && (
              <>
                <Tag color="blue">集群: {currentPod.clusterCode}</Tag>
                <Tag>ns: {currentPod.namespace}</Tag>
                <Tag color="purple">Pod: {currentPod.name}</Tag>
              </>
            )}
          </Space>
        }
        width={1100}
        open={logDrawerOpen}
        onClose={() => {
          setLogDrawerOpen(false);
          setCurrentPod(null);
        }}
        destroyOnClose
      >
        {currentPod && (
          <PodLogsViewer
            clusterCode={currentPod.clusterCode}
            namespace={currentPod.namespace}
            podName={currentPod.name}
            height={Math.max(480, window.innerHeight - 220)}
            autoScroll
            autoTrigger
            follow
            tailLines={500}
          />
        )}
      </Drawer>
    </PageContainer>
  );
};

export default ResourceList;
