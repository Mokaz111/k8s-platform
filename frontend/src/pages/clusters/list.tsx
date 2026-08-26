import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Button, Drawer, Form, Input, Popconfirm, Space, Spin, Tag, message } from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import {
  PlayCircleOutlined,
  EditOutlined,
  DeleteOutlined,
  AppstoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  ExperimentOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import type { ProColumns, ActionType } from '@ant-design/pro-components';
import dayjs from 'dayjs';
import {
  CLUSTER_STATUS_ONLINE,
  Cluster,
  useDeleteClusterMutation,
  useListClustersQuery,
  usePingClusterMutation,
  useTempPingMutation,
  useUpdateClusterMutation,
} from '@/app/services/cluster';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { setSelectedClusterCode } from '@/slices/appSlice';
import { usePermission } from '@/hooks/usePermission';

interface QueryParams {
  keyword?: string;
  page?: number;
  size?: number;
}

const ClusterList: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const actionRef = useRef<ActionType>();
  const [searchParams, setSearchParams] = useSearchParams();
  const keywordParam = searchParams.get('keyword') || '';
  const selectedClusterCode = useAppSelector((s) => s.app.selectedClusterCode);

  const [queryParams, setQueryParams] = useState<QueryParams>({
    keyword: keywordParam,
    page: 1,
    size: 10,
  });

  const { data, isFetching, refetch } = useListClustersQuery(queryParams, {
    refetchOnMountOrArgChange: true,
  });

  const { hasPerm } = usePermission();
  const canCreate = hasPerm('cluster:create');
  const canUpdate = hasPerm('cluster:update');
  const canDelete = hasPerm('cluster:delete');
  const canPing = hasPerm('cluster:ping');

  const [editingCluster, setEditingCluster] = useState<Cluster | null>(null);
  const [editForm] = Form.useForm();
  const [pingingCode, setPingingCode] = useState<string | null>(null);
  const [pingLoading, setPingLoading] = useState(false);
  const [editPingResult, setEditPingResult] = useState<{ success?: boolean; message?: string } | null>(null);
  const [editPingLoading, setEditPingLoading] = useState(false);

  const [updateCluster, { isLoading: updateLoading }] = useUpdateClusterMutation();
  const [deleteCluster] = useDeleteClusterMutation();
  const [pingCluster] = usePingClusterMutation();
  const [tempPing] = useTempPingMutation();

  const handleReload = useCallback(() => {
    refetch();
  }, [refetch]);

  const columns: ProColumns<Cluster>[] = useMemo(
    () => [
      {
        title: '名称 / 编码',
        dataIndex: 'name',
        key: 'name',
        width: 220,
        fixed: 'left',
        render: (_text, record) => (
          <Space direction="vertical" size={0}>
            <a
              style={{ fontWeight: 600 }}
              onClick={() => {
                dispatch(setSelectedClusterCode(record.code));
                navigate(`/resources/list?cluster_code=${encodeURIComponent(record.code)}`);
              }}
            >
              {record.name}
            </a>
            <span style={{ color: 'rgba(0,0,0,0.45)', fontSize: 12 }}>{record.code}</span>
          </Space>
        ),
      },
      {
        title: '版本',
        dataIndex: 'version',
        key: 'version',
        width: 140,
        render: (_dom, record) => record.version || '-',
      },
      {
        title: '节点数',
        dataIndex: 'node_count',
        key: 'node_count',
        width: 90,
        render: (_dom, record) => (typeof record.node_count === 'number' ? record.node_count : '-'),
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 110,
        render: (_dom, record) => {
          const online = record.status === CLUSTER_STATUS_ONLINE;
          return <Tag color={online ? 'green' : 'red'}>{online ? 'Online' : 'Offline'}</Tag>;
        },
      },
      {
        title: '最近同步时间',
        dataIndex: 'last_sync_at',
        key: 'last_sync_at',
        width: 180,
        render: (_dom, record) => (record.last_sync_at ? dayjs(record.last_sync_at).format('YYYY-MM-DD HH:mm:ss') : '-'),
      },
      {
        title: '操作',
        key: 'actions',
        width: 320,
        fixed: 'right',
        render: (_text, record) => (
          <Space size="small" wrap>
            <Button
              type="link"
              size="small"
              icon={pingingCode === record.code ? <Spin size="small" /> : <PlayCircleOutlined />}
              disabled={pingingCode === record.code || pingLoading || !canPing}
              onClick={async () => {
                setPingingCode(record.code);
                try {
                  const res = await pingCluster(record.code).unwrap();
                  message.success(
                    `Ping 成功${res.server_version ? ` (${res.server_version})` : ''}${
                      typeof res.node_count === 'number' ? ` 节点:${res.node_count}` : ''
                    }${res.cost_ms ? ` 耗时:${res.cost_ms}ms` : ''}`,
                  );
                } catch {
                  message.error('Ping 失败');
                } finally {
                  setPingingCode(null);
                  handleReload();
                }
              }}
            >
              Ping
            </Button>
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              disabled={!canUpdate}
              onClick={() => {
                setEditingCluster(record);
                setEditPingResult(null);
                editForm.setFieldsValue({
                  name: record.name,
                  description: record.description,
                  labels: '',
                  kubeconfig_text: '',
                });
              }}
            >
              编辑
            </Button>
            <Popconfirm
              title={`确定删除集群「${record.name}」?`}
              description="删除后无法恢复，请确认。"
              okButtonProps={{ danger: true }}
              onConfirm={async () => {
                try {
                  await deleteCluster(record.code).unwrap();
                  message.success('删除成功');
                  if (selectedClusterCode === record.code) {
                    dispatch(setSelectedClusterCode(null));
                  }
                  handleReload();
                } catch {
                  // error handled by interceptor
                }
              }}
            >
            <Button type="link" size="small" danger icon={<DeleteOutlined />} disabled={!canDelete}>
                删除
              </Button>
            </Popconfirm>
            <Button
              type="link"
              size="small"
              icon={<AppstoreOutlined />}
              onClick={() => {
                dispatch(setSelectedClusterCode(record.code));
                navigate(`/resources/list?cluster_code=${encodeURIComponent(record.code)}`);
              }}
            >
              资源
            </Button>
          </Space>
        ),
      },
    ],
    [
      dispatch,
      editForm,
      handleReload,
      navigate,
      pingingCode,
      pingCluster,
      tempPing,
      pingLoading,
      deleteCluster,
      selectedClusterCode,
      canPing,
      canUpdate,
      canDelete,
    ],
  );

  return (
    <PageContainer>
      <ProTable<Cluster>
        headerTitle="集群列表"
        actionRef={actionRef}
        rowKey="code"
        columns={columns}
        loading={isFetching}
        search={{
          labelWidth: 'auto',
          defaultCollapsed: false,
          searchText: '搜索',
          resetText: '重置',
        }}
        form={{
          initialValues: { keyword: keywordParam },
        }}
        onSubmit={(values) => {
          const kw = (values.keyword as string) || '';
          setSearchParams({ keyword: kw });
          setQueryParams((prev) => ({ ...prev, keyword: kw, page: 1 }));
        }}
        onReset={() => {
          setSearchParams({});
          setQueryParams((prev) => ({ ...prev, keyword: '', page: 1 }));
        }}
        toolBarRender={() => [
          <Button key="refresh" icon={<ReloadOutlined />} onClick={handleReload}>
            刷新
          </Button>,
          <Button
            key="import"
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => navigate('/clusters/import')}
            disabled={!canCreate}
          >
            导入集群
          </Button>,
        ]}
        dataSource={data?.items || []}
        pagination={{
          current: queryParams.page || 1,
          pageSize: queryParams.size || 10,
          total: data?.total || 0,
          defaultPageSize: 10,
          showSizeChanger: true,
          onChange: (page, pageSize) => {
            setQueryParams((prev) => ({ ...prev, page, size: pageSize }));
          },
        }}
        scroll={{ x: 1200 }}
        options={{
          reload: handleReload,
          density: true,
          fullScreen: true,
          setting: true,
        }}
      />

      <Drawer
        title={editingCluster ? `编辑集群：${editingCluster.name}` : '编辑集群'}
        width={560}
        open={!!editingCluster}
        onClose={() => setEditingCluster(null)}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setEditingCluster(null)}>取消</Button>
            <Button
              icon={<ExperimentOutlined />}
              loading={editPingLoading}
              disabled={!canPing}
              onClick={async () => {
                if (!editingCluster) return;
                const kubeText = editForm.getFieldValue('kubeconfig_text') as string | undefined;
                if (!kubeText || !kubeText.trim()) {
                  message.warning('请先在「更新 Kubeconfig」字段粘贴内容后再测试连接');
                  return;
                }
                try {
                  setEditPingLoading(true);
                  setEditPingResult(null);
                  // 用临时 ping 验证「尚未保存」的新 kubeconfig（POST /clusters/_/ping）
                  const res = await tempPing({ kubeconfig_text: kubeText }).unwrap();
                  const info = `${res.server_version || '未知版本'} · 节点 ${res.node_count ?? '-'} · ${res.cost_ms}ms`;
                  setEditPingResult({ success: true, message: info });
                  message.success('连接校验通过');
                } catch {
                  setEditPingResult({ success: false, message: '连接校验失败' });
                } finally {
                  setEditPingLoading(false);
                }
              }}
            >
              校验连接
            </Button>
            <Button
              type="primary"
              loading={updateLoading}
              onClick={async () => {
                if (!canUpdate) {
                  message.error('无集群编辑权限');
                  return;
                }
                try {
                  const values = (await editForm.validateFields()) as {
                    name: string;
                    description?: string;
                    labels?: string;
                    kubeconfig_text?: string;
                  };
                  if (editingCluster) {
                    // labels / kubeconfig_text 留空不代表要清空，只有非空才会提交
                    const payload: {
                      name: string;
                      description?: string;
                      labels?: string;
                      kubeconfig_text?: string;
                    } = {
                      name: values.name,
                      description: values.description,
                    };
                    if (values.labels && values.labels.trim()) {
                      try {
                        JSON.parse(values.labels);
                      } catch {
                        message.error('labels 必须是合法 JSON');
                        return;
                      }
                      payload.labels = values.labels;
                    }
                    if (values.kubeconfig_text && values.kubeconfig_text.trim()) {
                      // 用户更新了 kubeconfig：建议先校验连接再保存
                      if (!editPingResult?.success) {
                        message.warning('更新 Kubeconfig 后建议先点击「校验连接」并确认成功后再保存');
                        return;
                      }
                      payload.kubeconfig_text = values.kubeconfig_text;
                    }
                    await updateCluster({ code: editingCluster.code, data: payload }).unwrap();
                    message.success('更新成功');
                    setEditingCluster(null);
                    handleReload();
                  }
                } catch {
                  // validate error handled by Form
                }
              }}
            >
              保存
            </Button>
          </Space>
        }
      >
        <Form form={editForm} layout="vertical" preserve={false}>
          <Form.Item
            name="name"
            label="集群名称"
            rules={[{ required: true, message: '请输入集群名称' }]}
          >
            <Input placeholder="请输入集群名称" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} placeholder="请输入描述（可选）" />
          </Form.Item>
          <Form.Item
            name="labels"
            label="标签 (JSON)"
            help='例如 {"env":"prod","team":"sre"}。留空表示不修改，空字符串或合法 JSON 会覆盖之前的值'
            rules={[
              {
                validator: (_: unknown, value: unknown) => {
                  if (value === undefined || value === null || value === '') return Promise.resolve();
                  try {
                    JSON.parse(String(value));
                    return Promise.resolve();
                  } catch {
                    return Promise.reject(new Error('labels 必须是合法 JSON'));
                  }
                },
              },
            ]}
          >
            <Input.TextArea rows={2} style={{ fontFamily: 'monospace' }} placeholder="留空 = 不修改" />
          </Form.Item>
          <Form.Item
            name="kubeconfig_text"
            label="更新 Kubeconfig (可选)"
            help="留空表示不修改；若填写，强烈建议先点「校验连接」再保存"
          >
            <Input.TextArea rows={10} style={{ fontFamily: 'monospace' }} placeholder="粘贴新的 kubeconfig YAML 内容" />
          </Form.Item>
          {editPingResult && (
            <Space>
              {editPingResult.success ? (
                <span style={{ color: '#52c41a' }}>
                  <CheckCircleOutlined /> 连接校验通过
                  {editPingResult.message ? `：${editPingResult.message}` : ''}
                </span>
              ) : (
                <span style={{ color: '#ff4d4f' }}>
                  <CloseCircleOutlined /> 连接校验失败
                  {editPingResult.message ? `：${editPingResult.message}` : ''}
                </span>
              )}
            </Space>
          )}
        </Form>
      </Drawer>
    </PageContainer>
  );
};

export default ClusterList;
