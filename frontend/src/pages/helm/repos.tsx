import React, { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import {
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import {
  HelmChart,
  HelmRepo,
  useAddRepoMutation,
  useListReposQuery,
  useRemoveRepoMutation,
  useSearchChartsQuery,
  useUpdateReposMutation,
} from '@/app/services/helm';
import { usePermission } from '@/hooks/usePermission';

const { Text } = Typography;

interface AddRepoForm {
  name: string;
  url: string;
  username?: string;
  password?: string;
}

const HelmRepos: React.FC = () => {
  const navigate = useNavigate();
  const { hasPerm } = usePermission();
  const canView = hasPerm('helm:view');
  const canManage = hasPerm('helm:install');

  const { data, refetch, isFetching, error } = useListReposQuery(undefined, {
    skip: !canView,
    refetchOnMountOrArgChange: true,
  });
  const [addRepo, { isLoading: adding }] = useAddRepoMutation();
  const [removeRepo] = useRemoveRepoMutation();
  const [updateRepos, { isLoading: updating }] = useUpdateReposMutation();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [form] = Form.useForm<AddRepoForm>();

  const [chartRepo, setChartRepo] = useState<string>('');
  const [chartKeyword, setChartKeyword] = useState<string>('');
  const [chartQuery, setChartQuery] = useState<{ repo?: string; keyword?: string }>({});

  const repos = data?.items || [];

  const {
    data: chartsData,
    isFetching: chartsLoading,
    refetch: refetchCharts,
  } = useSearchChartsQuery(chartQuery, {
    skip: !canView || repos.length === 0,
    refetchOnMountOrArgChange: true,
  });

  const repoOptions = useMemo(
    () => [
      { label: '全部仓库', value: '' },
      ...repos.map((r) => ({ label: r.name, value: r.name })),
    ],
    [repos],
  );

  const errMsg = (error as { message?: string } | undefined)?.message;

  const handleAdd = async () => {
    try {
      const values = await form.validateFields();
      await addRepo({
        name: values.name.trim(),
        url: values.url.trim(),
        username: values.username?.trim() || undefined,
        password: values.password || undefined,
      }).unwrap();
      message.success(`仓库「${values.name.trim()}」已添加`);
      setDrawerOpen(false);
      form.resetFields();
    } catch {
      // interceptor / validation
    }
  };

  const handleUpdate = async (name?: string) => {
    try {
      await updateRepos(name ? { name } : undefined).unwrap();
      message.success(name ? `仓库「${name}」索引已更新` : '全部仓库索引已更新');
      refetchCharts();
    } catch {
      // interceptor
    }
  };

  const repoColumns: ProColumns<HelmRepo>[] = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 220,
      render: (_, r) => <Text strong>{r.name}</Text>,
    },
    {
      title: '地址',
      dataIndex: 'url',
      key: 'url',
      ellipsis: true,
      copyable: true,
    },
    {
      title: '操作',
      key: 'actions',
      width: 260,
      render: (_, r) => (
        <Space>
          <Button
            type="link"
            size="small"
            icon={<SyncOutlined />}
            disabled={!canManage}
            loading={updating}
            onClick={() => handleUpdate(r.name)}
          >
            更新索引
          </Button>
          <Popconfirm
            title={`确定移除仓库「${r.name}」？`}
            description="仅从本平台 Helm CLI 仓库列表移除，不影响已安装的 Release"
            okButtonProps={{ danger: true, disabled: !canManage }}
            onConfirm={async () => {
              if (!canManage) return;
              try {
                await removeRepo(r.name).unwrap();
                message.success(`已移除仓库「${r.name}」`);
              } catch {
                // interceptor
              }
            }}
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />} disabled={!canManage}>
              移除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const chartColumns: ProColumns<HelmChart>[] = [
    {
      title: 'Chart',
      dataIndex: 'name',
      key: 'name',
      width: 280,
      render: (_, r) => <Text code>{r.name}</Text>,
    },
    {
      title: 'Version',
      dataIndex: 'version',
      key: 'version',
      width: 140,
    },
    {
      title: 'App Version',
      dataIndex: 'app_version',
      key: 'app_version',
      width: 140,
      render: (v) => v || '-',
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '操作',
      key: 'install',
      width: 120,
      render: (_, r) => (
        <Button
          type="link"
          size="small"
          disabled={!canManage}
          onClick={() => {
            const params = new URLSearchParams({ chart_ref: r.name, version: r.version || '' });
            navigate(`/helm/list?${params.toString()}`);
          }}
        >
          去安装
        </Button>
      ),
    },
  ];

  return (
    <PageContainer
      header={{
        title: '制品仓库',
        subTitle: '管理 Helm Chart 仓库，并检索可安装的 Chart',
      }}
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {errMsg && (
          <Alert
            type="error"
            showIcon
            message="无法列出 Helm 仓库"
            description={`${errMsg}。请确认平台所在节点已安装 helm CLI，且当前账号具备 helm:view 权限。`}
          />
        )}

        <ProTable<HelmRepo>
          headerTitle={
            <Space>
              <span>Helm 仓库</span>
              <Tag>{repos.length} 个</Tag>
            </Space>
          }
          rowKey="name"
          search={false}
          loading={isFetching}
          dataSource={repos}
          columns={repoColumns}
          pagination={false}
          toolBarRender={() => [
            <Button key="reload" icon={<ReloadOutlined />} onClick={() => refetch()}>
              刷新
            </Button>,
            <Button
              key="update"
              icon={<SyncOutlined />}
              loading={updating}
              disabled={!canManage}
              onClick={() => handleUpdate()}
            >
              更新全部索引
            </Button>,
            <Button
              key="add"
              type="primary"
              icon={<PlusOutlined />}
              disabled={!canManage}
              onClick={() => {
                form.resetFields();
                setDrawerOpen(true);
              }}
            >
              添加仓库
            </Button>,
          ]}
        />

        <ProTable<HelmChart>
          headerTitle="Chart 检索"
          rowKey={(r) => `${r.name}@${r.version}`}
          search={false}
          loading={chartsLoading}
          dataSource={chartsData?.items || []}
          columns={chartColumns}
          pagination={{ pageSize: 10, showSizeChanger: true }}
          toolbar={{
            search: undefined,
          }}
          toolBarRender={() => [
            <Select
              key="repo"
              style={{ width: 200 }}
              value={chartRepo}
              options={repoOptions}
              onChange={setChartRepo}
              placeholder="筛选仓库"
            />,
            <Input.Search
              key="kw"
              style={{ width: 280 }}
              allowClear
              placeholder="按 Chart 名称搜索"
              value={chartKeyword}
              onChange={(e) => setChartKeyword(e.target.value)}
              onSearch={(v) => {
                setChartKeyword(v);
                setChartQuery({ repo: chartRepo || undefined, keyword: v || undefined });
              }}
            />,
            <Button
              key="search"
              type="primary"
              icon={<SearchOutlined />}
              onClick={() =>
                setChartQuery({ repo: chartRepo || undefined, keyword: chartKeyword || undefined })
              }
            >
              搜索
            </Button>,
          ]}
        />
      </Space>

      <Drawer
        title="添加 Helm 仓库"
        width={520}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        extra={
          <Space>
            <Button onClick={() => setDrawerOpen(false)}>取消</Button>
            <Button type="primary" loading={adding} onClick={handleAdd}>
              添加
            </Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="仓库名称"
            rules={[
              { required: true, message: '请输入名称' },
              { pattern: /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/, message: '仅允许字母数字及 ._-，且以字母或数字开头' },
            ]}
          >
            <Input placeholder="如 bitnami" />
          </Form.Item>
          <Form.Item
            name="url"
            label="仓库地址"
            rules={[
              { required: true, message: '请输入 URL' },
              { type: 'url', message: '请输入合法 http/https 地址' },
            ]}
          >
            <Input placeholder="https://charts.bitnami.com/bitnami" />
          </Form.Item>
          <Form.Item name="username" label="用户名（可选）">
            <Input placeholder="私有仓库认证" autoComplete="off" />
          </Form.Item>
          <Form.Item name="password" label="密码（可选）">
            <Input.Password placeholder="私有仓库认证" autoComplete="new-password" />
          </Form.Item>
        </Form>
      </Drawer>
    </PageContainer>
  );
};

export default HelmRepos;
