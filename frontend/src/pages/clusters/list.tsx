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
} from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import type { ProColumns, ActionType } from '@ant-design/pro-components';
import dayjs from 'dayjs';
import {
  Cluster,
  useDeleteClusterMutation,
  useListClustersQuery,
  usePingClusterMutation,
  useUpdateClusterMutation,
} from '@/app/services/cluster';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { setSelectedClusterCode } from '@/slices/appSlice';

const statusColorMap: Record<string, string> = {
  Online: 'green',
  Offline: 'red',
  Running: 'blue',
  Pending: 'gold',
  Error: 'red',
};

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

  const [editingCluster, setEditingCluster] = useState<Cluster | null>(null);
  const [editForm] = Form.useForm();
  const [pingingCode, setPingingCode] = useState<string | null>(null);
  const [pingLoading, setPingLoading] = useState(false);

  const [updateCluster, { isLoading: updateLoading }] = useUpdateClusterMutation();
  const [deleteCluster] = useDeleteClusterMutation();
  const [pingCluster] = usePingClusterMutation();

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
        render: (v) => v || '-',
      },
      {
        title: '节点数',
        dataIndex: 'nodes',
        key: 'nodes',
        width: 90,
        render: (v) => (typeof v === 'number' ? v : '-'),
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 110,
        render: (status: string) => {
          const color = statusColorMap[status] || 'default';
          return <Tag color={color}>{status || 'Unknown'}</Tag>;
        },
      },
      {
        title: '最近同步时间',
        dataIndex: 'lastSyncTime',
        key: 'lastSyncTime',
        width: 180,
        render: (v) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm:ss') : '-'),
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
              disabled={pingingCode === record.code || pingLoading}
              onClick={async () => {
                setPingingCode(record.code);
                try {
                  const res = await pingCluster(record.code).unwrap();
                  if (res.success) {
                    message.success(
                      `Ping 成功 ${res.version ? `(v${res.version})` : ''}${
                        res.nodes ? ` 节点:${res.nodes}` : ''
                      }`,
                    );
                  } else {
                    message.error(res.message || 'Ping 失败');
                  }
                } catch {
                  message.error('Ping 请求失败');
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
              onClick={() => {
                setEditingCluster(record);
                editForm.setFieldsValue({
                  name: record.name,
                  description: record.description,
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
              <Button type="link" size="small" danger icon={<DeleteOutlined />}>
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
      pingLoading,
      deleteCluster,
      selectedClusterCode,
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
          >
            导入集群
          </Button>,
        ]}
        dataSource={data?.list || []}
        pagination={{
          current: data?.page || 1,
          pageSize: data?.size || 10,
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
        width={480}
        open={!!editingCluster}
        onClose={() => setEditingCluster(null)}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setEditingCluster(null)}>取消</Button>
            <Button
              type="primary"
              loading={updateLoading}
              onClick={async () => {
                try {
                  const values = await editForm.validateFields();
                  if (editingCluster) {
                    await updateCluster({ code: editingCluster.code, data: values }).unwrap();
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
            <Input.TextArea rows={4} placeholder="请输入描述（可选）" />
          </Form.Item>
        </Form>
      </Drawer>
    </PageContainer>
  );
};

export default ClusterList;
