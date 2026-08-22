import React, { useCallback, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import {
  PlusOutlined,
  ReloadOutlined,
  EditOutlined,
  KeyOutlined,
  TeamOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import {
  BindUserRoleBody,
  CreateUserBody,
  ListUsersParams,
  ScopeType,
  UpdateUserBody,
  User,
  UserRole,
  useBindUserRoleMutation,
  useCreateUserMutation,
  useListRolesQuery,
  useListUserRolesQuery,
  useListUsersQuery,
  useResetPasswordMutation,
  useUnbindUserRoleMutation,
  useUpdateUserMutation,
  useUpdateUserStatusMutation,
} from '@/app/services/rbac';
import { useListClustersQuery } from '@/app/services/cluster';
import { usePermission } from '@/hooks/usePermission';

const { Text } = Typography;

// 后端 UserStatus：1=Enabled 2=Disabled 3=Locked
const statusLabelMap: Record<number, string> = {
  1: '启用',
  2: '禁用',
  3: '锁定',
};
const statusColorMap: Record<number, string> = {
  1: 'success',
  2: 'error',
  3: 'warning',
};

const scopeLabelMap: Record<ScopeType, string> = {
  platform: '平台',
  cluster: '集群',
  namespace: '命名空间',
};

const UserList: React.FC = () => {
  const { hasPerm } = usePermission();
  const canView = hasPerm('user:view');
  const canCreate = hasPerm('user:create');
  const canUpdate = hasPerm('user:update');

  const [filters, setFilters] = useState<ListUsersParams>({
    page: 1,
    page_size: 10,
  });

  const { data, isFetching, refetch } = useListUsersQuery(filters, {
    skip: !canView,
    refetchOnMountOrArgChange: true,
  });

  const [createUser, { isLoading: createLoading }] = useCreateUserMutation();
  const [updateUser, { isLoading: updateLoading }] = useUpdateUserMutation();
  const [updateStatus] = useUpdateUserStatusMutation();
  const [resetPwd] = useResetPasswordMutation();

  const [createOpen, setCreateOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [createForm] = Form.useForm<CreateUserBody>();
  const [editForm] = Form.useForm<UpdateUserBody>();

  // 角色绑定 Drawer 状态
  const [rolesUser, setRolesUser] = useState<User | null>(null);

  const handleReload = useCallback(() => refetch(), [refetch]);

  const handleSearch = useCallback((keyword: string) => {
    setFilters((prev) => ({ ...prev, keyword: keyword || undefined, page: 1 }));
  }, []);

  const handlePageChange = useCallback((page: number, pageSize: number) => {
    setFilters((prev) => ({ ...prev, page, page_size: pageSize }));
  }, []);

  const handleCreate = useCallback(async () => {
    try {
      const values = await createForm.validateFields();
      const res = await createUser(values).unwrap();
      message.success(res.message || `用户 ${values.username} 创建成功`);
      setCreateOpen(false);
      createForm.resetFields();
      refetch();
    } catch {
      // validator 拦截
    }
  }, [createForm, createUser, refetch]);

  const handleOpenEdit = useCallback(
    (record: User) => {
      setEditingUser(record);
      editForm.setFieldsValue({
        display_name: record.display_name,
        email: record.email,
        phone: record.phone,
      });
    },
    [editForm],
  );

  const handleEdit = useCallback(async () => {
    if (!editingUser) return;
    try {
      const values = await editForm.validateFields();
      await updateUser({ id: editingUser.id, body: values }).unwrap();
      message.success('用户信息已更新');
      setEditingUser(null);
      refetch();
    } catch {
      // validator 拦截
    }
  }, [editingUser, editForm, updateUser, refetch]);

  const handleToggleStatus = useCallback(
    async (record: User) => {
      // 1 -> 2 (启用->禁用), 2 -> 1 (禁用->启用)
      const next = record.status === 1 ? 2 : 1;
      try {
        await updateStatus({ id: record.id, status: next }).unwrap();
        message.success(`已${next === 1 ? '启用' : '禁用'}用户 ${record.username}`);
        refetch();
      } catch {
        // interceptor
      }
    },
    [updateStatus, refetch],
  );

  const handleResetPassword = useCallback(
    async (record: User) => {
      // 弹一个 prompt 让输入新密码
      let pwd = '';
      Modal.confirm({
        title: `重置用户 ${record.username} 的密码`,
        content: (
          <Input.Password
            placeholder="请输入新密码（至少 8 位）"
            onChange={(e) => {
              pwd = e.target.value;
            }}
          />
        ),
        onOk: async () => {
          if (!pwd || pwd.length < 8) {
            message.error('密码至少 8 位');
            throw new Error('invalid');
          }
          try {
            await resetPwd({ id: record.id, password: pwd }).unwrap();
            message.success('密码已重置');
          } catch {
            // interceptor
          }
        },
      });
    },
    [resetPwd],
  );

  const columns: ProColumns<User>[] = useMemo(
    () => [
      {
        title: '用户名',
        dataIndex: 'username',
        key: 'username',
        width: 140,
        fixed: 'left',
        render: (text) => <Text strong>{text}</Text>,
      },
      {
        title: '显示名',
        dataIndex: 'display_name',
        key: 'display_name',
        width: 120,
        render: (text) => text || '-',
      },
      {
        title: '邮箱',
        dataIndex: 'email',
        key: 'email',
        width: 180,
        render: (text) => text ? <Text copyable>{text}</Text> : '-',
      },
      {
        title: '手机',
        dataIndex: 'phone',
        key: 'phone',
        width: 130,
        render: (text) => text || '-',
      },
      {
        title: '来源',
        dataIndex: 'auth_source',
        key: 'auth_source',
        width: 90,
        render: (text) => <Tag>{text || 'local'}</Tag>,
      },
      {
        title: '角色数',
        dataIndex: 'role_count',
        key: 'role_count',
        width: 80,
        align: 'right',
        render: (text) => {
          const n = Number(text);
          return n ? <Tag color="blue">{n}</Tag> : <Text type="secondary">0</Text>;
        },
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 90,
        render: (text, record) => {
          const st = Number(text);
          if (!canUpdate) {
            return <Tag color={statusColorMap[st] || 'default'}>{statusLabelMap[st] || '-'}</Tag>;
          }
          return (
            <Popconfirm
              title={st === 1 ? '确认禁用该用户？' : '确认启用该用户？'}
              onConfirm={() => handleToggleStatus(record)}
            >
              <Switch
                checked={st === 1}
                checkedChildren="启用"
                unCheckedChildren="禁用"
              />
            </Popconfirm>
          );
        },
      },
      {
        title: '最后登录',
        dataIndex: 'last_login_at',
        key: 'last_login_at',
        width: 160,
        render: (text) =>
          text ? dayjs(text as string).format('YYYY-MM-DD HH:mm:ss') : <Text type="secondary">未登录</Text>,
      },
      {
        title: '创建时间',
        dataIndex: 'created_at',
        key: 'created_at',
        width: 160,
        render: (text) =>
          text ? dayjs(text as string).format('YYYY-MM-DD HH:mm:ss') : '-',
      },
      {
        title: '操作',
        key: 'action',
        width: 220,
        fixed: 'right',
        render: (_text, record) => (
          <Space size={4}>
            <Tooltip title="绑定角色">
              <Button
                type="link"
                size="small"
                icon={<TeamOutlined />}
                disabled={!canUpdate}
                onClick={() => setRolesUser(record)}
              >
                角色
              </Button>
            </Tooltip>
            <Tooltip title="编辑">
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                disabled={!canUpdate}
                onClick={() => handleOpenEdit(record)}
              />
            </Tooltip>
            <Tooltip title="重置密码">
              <Button
                type="link"
                size="small"
                icon={<KeyOutlined />}
                disabled={!canUpdate}
                onClick={() => handleResetPassword(record)}
              />
            </Tooltip>
          </Space>
        ),
      },
    ],
    [canUpdate, handleToggleStatus, handleOpenEdit, handleResetPassword],
  );

  if (!canView) {
    return (
      <PageContainer>
        <Card>
          <Text type="warning">您没有查看用户列表的权限（user:view）。</Text>
        </Card>
      </PageContainer>
    );
  }

  return (
    <PageContainer
      extra={[
        <Button
          key="create"
          type="primary"
          icon={<PlusOutlined />}
          disabled={!canCreate}
          onClick={() => setCreateOpen(true)}
        >
          新建用户
        </Button>,
        <Button key="reload" icon={<ReloadOutlined />} loading={isFetching} onClick={handleReload}>
          刷新
        </Button>,
      ]}
    >
      <ProTable<User>
        rowKey="id"
        loading={isFetching}
        columns={columns}
        dataSource={data?.items || []}
        scroll={{ x: 1400 }}
        search={false}
        headerTitle={
          <Input.Search
            placeholder="搜索用户名/邮箱"
            allowClear
            enterButton
            style={{ width: 260 }}
            onSearch={handleSearch}
          />
        }
        pagination={{
          current: filters.page || 1,
          pageSize: filters.page_size || 10,
          total: data?.total || 0,
          showSizeChanger: true,
          showTotal: (total) => `共 ${total} 条`,
          onChange: handlePageChange,
        }}
        options={{ density: false, fullScreen: false, reload: false, setting: false }}
      />

      {/* 新建用户 Drawer */}
      <Drawer
        title="新建用户"
        open={createOpen}
        width={420}
        onClose={() => setCreateOpen(false)}
        extra={
          <Space>
            <Button onClick={() => setCreateOpen(false)}>取消</Button>
            <Button type="primary" loading={createLoading} onClick={handleCreate}>
              创建
            </Button>
          </Space>
        }
      >
        <Form<CreateUserBody> form={createForm} layout="vertical">
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, min: 3, max: 64, message: '请输入 3-64 位用户名' }]}
          >
            <Input placeholder="登录用户名" />
          </Form.Item>
          <Form.Item
            name="password"
            label="初始密码"
            rules={[{ required: true, min: 8, max: 128, message: '密码至少 8 位' }]}
          >
            <Input.Password placeholder="至少 8 位" />
          </Form.Item>
          <Form.Item name="display_name" label="显示名">
            <Input placeholder="可选" />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[{ type: 'email', message: '邮箱格式不正确' }]}
          >
            <Input placeholder="可选" />
          </Form.Item>
          <Form.Item name="phone" label="手机">
            <Input placeholder="可选" />
          </Form.Item>
          <Form.Item name="auth_source" label="认证来源" initialValue="local">
            <Select
              options={[
                { value: 'local', label: '本地 (local)' },
                { value: 'ldap', label: 'LDAP' },
                { value: 'oidc', label: 'OIDC' },
              ]}
            />
          </Form.Item>
        </Form>
      </Drawer>

      {/* 编辑用户 Drawer */}
      <Drawer
        title={`编辑用户 - ${editingUser?.username || ''}`}
        open={!!editingUser}
        width={420}
        onClose={() => setEditingUser(null)}
        extra={
          <Space>
            <Button onClick={() => setEditingUser(null)}>取消</Button>
            <Button type="primary" loading={updateLoading} onClick={handleEdit}>
              保存
            </Button>
          </Space>
        }
      >
        <Form<UpdateUserBody> form={editForm} layout="vertical">
          <Form.Item name="display_name" label="显示名">
            <Input />
          </Form.Item>
          <Form.Item
            name="email"
            label="邮箱"
            rules={[{ type: 'email', message: '邮箱格式不正确' }]}
          >
            <Input />
          </Form.Item>
          <Form.Item name="phone" label="手机">
            <Input />
          </Form.Item>
        </Form>
      </Drawer>

      {/* 角色绑定 Drawer */}
      {rolesUser && (
        <UserRolesDrawer user={rolesUser} onClose={() => setRolesUser(null)} />
      )}
    </PageContainer>
  );
};

// 用户角色绑定 Drawer
const UserRolesDrawer: React.FC<{ user: User; onClose: () => void }> = ({
  user,
  onClose,
}) => {
  const { hasPerm } = usePermission();
  const canBind = hasPerm('user:bind_role');

  const { data: userRoles, isFetching: rolesFetching } = useListUserRolesQuery(user.id);
  const { data: rolesData } = useListRolesQuery({ page: 1, page_size: 200 });
  const { data: clustersData } = useListClustersQuery(undefined);

  const [bindRole, { isLoading: bindLoading }] = useBindUserRoleMutation();
  const [unbindRole] = useUnbindUserRoleMutation();

  const [bindForm] = Form.useForm<BindUserRoleBody>();

  const roleOptions = useMemo(
    () => (rolesData?.items || []).map((r) => ({ value: r.id, label: `${r.name} (${r.code})` })),
    [rolesData],
  );
  const clusterOptions = useMemo(
    () => (clustersData?.items || []).map((c) => ({ value: c.code, label: `${c.name} (${c.code})` })),
    [clustersData],
  );

  const handleBind = useCallback(async () => {
    try {
      const values = await bindForm.validateFields();
      // 命名空间级需要 cluster_code + namespace；集群级需要 cluster_code；平台级无需
      if (
        values.scope_type === 'namespace' &&
        (!values.cluster_code || !values.namespace)
      ) {
        message.error('命名空间级权限需填写集群与命名空间');
        return;
      }
      if (values.scope_type === 'cluster' && !values.cluster_code) {
        message.error('集群级权限需填写集群');
        return;
      }
      await bindRole({ id: user.id, body: values }).unwrap();
      message.success('角色绑定成功');
      bindForm.resetFields();
    } catch {
      // validator 拦截
    }
  }, [bindForm, bindRole, user.id]);

  const handleUnbind = useCallback(
    async (ur: UserRole) => {
      try {
        await unbindRole({
          id: user.id,
          body: {
            role_id: ur.role_id,
            scope_type: ur.scope_type,
            cluster_code: ur.cluster_code,
            namespace: ur.namespace,
          },
        }).unwrap();
        message.success('已解绑角色');
      } catch {
        // interceptor
      }
    },
    [unbindRole, user.id],
  );

  return (
    <Drawer
      title={`角色绑定 - ${user.username}`}
      open
      width={640}
      onClose={onClose}
    >
      {/* 新增绑定 */}
      {canBind && (
        <Card size="small" title="新增角色绑定" style={{ marginBottom: 16 }}>
          <Form<BindUserRoleBody> form={bindForm} layout="vertical" initialValues={{ scope_type: 'platform' }}>
            <Form.Item name="role_id" label="角色" rules={[{ required: true, message: '请选择角色' }]}>
              <Select options={roleOptions} placeholder="选择角色" showSearch optionFilterProp="label" />
            </Form.Item>
            <Form.Item name="scope_type" label="数据范围" rules={[{ required: true }]}>
              <Select
                options={[
                  { value: 'platform', label: '平台级' },
                  { value: 'cluster', label: '集群级' },
                  { value: 'namespace', label: '命名空间级' },
                ]}
              />
            </Form.Item>
            <Form.Item noStyle shouldUpdate={(prev, cur) => prev.scope_type !== cur.scope_type}>
              {({ getFieldValue }) => {
                const scope = getFieldValue('scope_type') as ScopeType;
                const needCluster = scope === 'cluster' || scope === 'namespace';
                const needNs = scope === 'namespace';
                return (
                  <>
                    {needCluster && (
                      <Form.Item
                        name="cluster_code"
                        label="集群"
                        rules={needCluster ? [{ required: true, message: '请选择集群' }] : []}
                      >
                        <Select options={clusterOptions} placeholder="选择集群" showSearch optionFilterProp="label" allowClear />
                      </Form.Item>
                    )}
                    {needNs && (
                      <Form.Item
                        name="namespace"
                        label="命名空间"
                        rules={needNs ? [{ required: true, message: '请输入命名空间' }] : []}
                      >
                        <Input placeholder="如 default / kube-system" />
                      </Form.Item>
                    )}
                  </>
                );
              }}
            </Form.Item>
            <Button type="primary" loading={bindLoading} onClick={handleBind}>
              绑定
            </Button>
          </Form>
        </Card>
      )}

      {/* 已绑定角色列表 */}
      <Card size="small" title={`已绑定角色 (${userRoles?.length || 0})`} loading={rolesFetching}>
        {(userRoles || []).length === 0 ? (
          <Text type="secondary">暂无绑定角色</Text>
        ) : (
          <Space direction="vertical" style={{ width: '100%' }} size={8}>
            {(userRoles || []).map((ur) => (
              <Card
                key={`${ur.id}-${ur.role_id}-${ur.scope_type}-${ur.cluster_code || ''}-${ur.namespace || ''}`}
                size="small"
                type="inner"
              >
                <Space style={{ justifyContent: 'space-between', width: '100%' }}>
                  <Space direction="vertical" size={0}>
                    <Space size={8}>
                      <Text strong>{ur.role_name}</Text>
                      <Tag color="geekblue">{ur.role_code}</Tag>
                      <Tag color="purple">{scopeLabelMap[ur.scope_type]}</Tag>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {ur.cluster_code ? `集群: ${ur.cluster_code}` : ''}
                      {ur.namespace ? ` / 命名空间: ${ur.namespace}` : ''}
                    </Text>
                  </Space>
                  {canBind && (
                    <Popconfirm
                      title="确认解绑该角色？"
                      onConfirm={() => handleUnbind(ur)}
                    >
                      <Button danger size="small" icon={<DeleteOutlined />}>
                        解绑
                      </Button>
                    </Popconfirm>
                  )}
                </Space>
              </Card>
            ))}
          </Space>
        )}
      </Card>

      <div style={{ marginTop: 16 }}>
        <Button onClick={onClose}>关闭</Button>
      </div>
    </Drawer>
  );
};

export default UserList;
