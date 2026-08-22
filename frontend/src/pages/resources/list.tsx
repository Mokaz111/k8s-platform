import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
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
  DeleteOutlined,
  EditOutlined,
  FileTextOutlined,
  HistoryOutlined,
  ReloadOutlined,
  SaveOutlined,
  CloudServerOutlined,
} from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import {
  KubernetesResource,
  useDeleteResourceMutation,
  useListNamespacesQuery,
  useListResourcesQuery,
} from '@/app/services/resource';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { setSelectedClusterCode } from '@/slices/appSlice';

dayjs.extend(relativeTime);

type ResourceTab = 'workload' | 'config' | 'storage';

interface KindGroup {
  key: ResourceTab;
  label: string;
  kinds: { label: string; value: string; apiVersion: string }[];
}

const KIND_GROUPS: KindGroup[] = [
  {
    key: 'workload',
    label: 'Workloads',
    kinds: [
      { label: 'Deployment', value: 'Deployment', apiVersion: 'apps/v1' },
      { label: 'StatefulSet', value: 'StatefulSet', apiVersion: 'apps/v1' },
      { label: 'DaemonSet', value: 'DaemonSet', apiVersion: 'apps/v1' },
      { label: 'Job', value: 'Job', apiVersion: 'batch/v1' },
      { label: 'CronJob', value: 'CronJob', apiVersion: 'batch/v1' },
    ],
  },
  {
    key: 'config',
    label: 'Config',
    kinds: [
      { label: 'ConfigMap', value: 'ConfigMap', apiVersion: 'v1' },
      { label: 'Secret', value: 'Secret', apiVersion: 'v1' },
    ],
  },
  {
    key: 'storage',
    label: 'Storage',
    kinds: [
      { label: 'PersistentVolume', value: 'PersistentVolume', apiVersion: 'v1' },
      { label: 'PersistentVolumeClaim', value: 'PersistentVolumeClaim', apiVersion: 'v1' },
      { label: 'StorageClass', value: 'StorageClass', apiVersion: 'storage.k8s.io/v1' },
    ],
  },
];

const ALL_KINDS_MAP: Record<string, string> = {};
KIND_GROUPS.forEach((g) => g.kinds.forEach((k) => (ALL_KINDS_MAP[k.value] = k.apiVersion)));

const ResourceList: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const actionRef = React.useRef<ActionType>();
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);
  const [searchParams, setSearchParams] = useSearchParams();

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
  const apiVersion = ALL_KINDS_MAP[kind] || currentGroup.kinds[0].apiVersion;

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
      code: clusterCode || '',
      apiVersion,
      kind,
      namespace: namespace || undefined,
      page,
      size,
      keyword: keyword || undefined,
    }),
    [clusterCode, apiVersion, kind, namespace, page, size, keyword],
  );

  const { data, refetch, isFetching } = useListResourcesQuery(listParams, {
    skip: !clusterCode,
    refetchOnMountOrArgChange: true,
  });

  const [deleteResource] = useDeleteResourceMutation();

  const handleReload = useCallback(() => {
    refetch();
  }, [refetch]);

  const editUrl = (item: KubernetesResource): string => {
    const ns = encodeURIComponent(item.metadata?.namespace || '_');
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
                onClick={() => navigate(editUrl(item))}
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
        render: (v) => v || '（集群级）',
      },
      {
        title: 'Age',
        dataIndex: ['metadata', 'creationTimestamp'],
        key: 'age',
        width: 140,
        render: (v: string | undefined) => {
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
        width: 360,
        fixed: 'right',
        render: (_v, item) => {
          const ns = item.metadata?.namespace;
          const name = item.metadata?.name || '';
          return (
            <Space size="small" wrap>
              <Button
                type="link"
                size="small"
                icon={<FileTextOutlined />}
                onClick={() => navigate(editUrl(item))}
              >
                查看 YAML
              </Button>
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                onClick={() => navigate(editUrl(item))}
              >
                编辑
              </Button>
              <Popconfirm
                title={`确定删除 ${kind}「${name}」?`}
                description="删除后该资源将从集群中移除"
                okButtonProps={{ danger: true }}
                onConfirm={async () => {
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
                <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
              <Button
                type="link"
                size="small"
                icon={<HistoryOutlined />}
                onClick={() => navigate(editUrl(item) + '?tab=versions')}
              >
                版本历史
              </Button>
              <Button
                type="link"
                size="small"
                icon={<CloudServerOutlined />}
                onClick={() => navigate(backupUrl(item))}
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
            setKind(group?.kinds[0].value || KIND_GROUPS[0].kinds[0].value);
            setPage(1);
          }}
          items={KIND_GROUPS.map((g) => ({ key: g.key, label: g.label }))}
        />

        <Space wrap align="center">
          <Select
            style={{ width: 200 }}
            placeholder="命名空间"
            value={namespace}
            onChange={(v) => {
              setNamespace(v);
              setPage(1);
            }}
            options={namespaceOptions}
            allowClear
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
        </Space>

        <ProTable<KubernetesResource>
          headerTitle={
            <Space>
              <SaveOutlined />
              <span>{currentGroup.label} / {kind}</span>
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
          scroll={{ x: 1200 }}
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
    </PageContainer>
  );
};

export default ResourceList;
