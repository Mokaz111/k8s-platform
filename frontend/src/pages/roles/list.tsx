import React, { useCallback, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Popconfirm,
  Space,
  Switch,
  Tag,
  Tooltip,
  Tree,
  Typography,
  message,
} from 'antd';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import {
  PlusOutlined,
  ReloadOutlined,
  EditOutlined,
  SafetyOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import {
  CreateRoleBody,
  ListRolesParams,
  Permission,
  Role,
  UpdateRoleBody,
  useCreateRoleMutation,
  useDeleteRoleMutation,
  useGetRolePermissionsQuery,
  useListPermissionsQuery,
  useListRolesQuery,
  useSetRolePermissionsMutation,
  useUpdateRoleMutation,
} from '@/app/services/rbac';
import { usePermission } from '@/hooks/usePermission';

const { Text } = Typography;

const RoleList: React.FC = () => {
  const { hasPerm } = usePermission();
  // 后端角色相关接口统一要求 role:manage（见 middleware/auth.go），无细分权限点
  const canView = hasPerm('role:manage');
  const canCreate = hasPerm('role:manage');
  const canUpdate = hasPerm('role:manage');
  const canDelete = hasPerm('role:manage');

  const [filters, setFilters] = useState<ListRolesParams>({
    page: 1,
    page_size: 10,
  });

  const { data, isFetching, refetch } = useListRolesQuery(filters, {
    skip: !canView,
    refetchOnMountOrArgChange: true,
  });

  const [createRole, { isLoading: createLoading }] = useCreateRoleMutation();
  const [updateRole, { isLoading: updateLoading }] = useUpdateRoleMutation();
  const [deleteRole] = useDeleteRoleMutation();

  const [createOpen, setCreateOpen] = useState(false);
  const [editingRole, setEditingRole] = useState<Role | null>(null);
  const [createForm] = Form.useForm<CreateRoleBody>();
  const [editForm] = Form.useForm<UpdateRoleBody>();

  // 权限设置 Drawer
  const [permsRole, setPermsRole] = useState<Role | null>(null);

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
      const res = await createRole(values).unwrap();
      message.success(res.message || `角色 ${values.code} 创建成功`);
      setCreateOpen(false);
      createForm.resetFields();
      refetch();
    } catch {
      // validator 拦截
    }
  }, [createForm, createRole, refetch]);

  const handleOpenEdit = useCallback(
    (record: Role) => {
      setEditingRole(record);
      editForm.setFieldsValue({
        name: record.name,
        description: record.description,
      });
    },
    [editForm],
  );

  const handleEdit = useCallback(async () => {
    if (!editingRole) return;
    try {
      const values = await editForm.validateFields();
      await updateRole({ id: editingRole.id, body: values }).unwrap();
      message.success('角色已更新');
      setEditingRole(null);
      refetch();
    } catch {
      // validator 拦截
    }
  }, [editingRole, editForm, updateRole, refetch]);

  const handleToggleStatus = useCallback(
    async (record: Role) => {
      const next = record.status === 1 ? 0 : 1;
      try {
        await updateRole({ id: record.id, body: { status: next } }).unwrap();
        message.success(`已${next === 1 ? '启用' : '禁用'}角色 ${record.name}`);
        refetch();
      } catch {
        // interceptor
      }
    },
    [updateRole, refetch],
  );

  const handleDelete = useCallback(
    async (record: Role) => {
      if (record.builtin === 1) {
        message.error('内置角色不可删除');
        return;
      }
      try {
        await deleteRole(record.id).unwrap();
        message.success(`角色 ${record.name} 已删除`);
        refetch();
      } catch {
        // interceptor
      }
    },
    [deleteRole, refetch],
  );

  const columns: ProColumns<Role>[] = useMemo(
    () => [
      {
        title: '角色编码',
        dataIndex: 'code',
        key: 'code',
        width: 160,
        fixed: 'left',
        render: (text, record) => (
          <Space direction="vertical" size={0}>
            <Space size={6}>
              <Text strong>{text}</Text>
              {record.builtin === 1 && <Tag color="gold">内置</Tag>}
            </Space>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {record.name}
            </Text>
          </Space>
        ),
      },
      {
        title: '描述',
        dataIndex: 'description',
        key: 'description',
        width: 220,
        render: (text) => text ? <Text type="secondary">{text}</Text> : '-',
      },
      {
        title: '权限数',
        dataIndex: 'perm_count',
        key: 'perm_count',
        width: 90,
        align: 'right',
        render: (text) => {
          const n = Number(text);
          return n ? <Tag color="blue">{n}</Tag> : <Text type="secondary">0</Text>;
        },
      },
      {
        title: '用户数',
        dataIndex: 'user_count',
        key: 'user_count',
        width: 90,
        align: 'right',
        render: (text) => {
          const n = Number(text);
          return n ? <Tag color="green">{n}</Tag> : <Text type="secondary">0</Text>;
        },
      },
      {
        title: '状态',
        dataIndex: 'status',
        key: 'status',
        width: 90,
        render: (text, record) => {
          if (!canUpdate) {
            return <Tag color={record.status === 1 ? 'success' : 'error'}>{record.status === 1 ? '启用' : '禁用'}</Tag>;
          }
          return (
            <Popconfirm
              title={record.status === 1 ? '确认禁用该角色？' : '确认启用该角色？'}
              onConfirm={() => handleToggleStatus(record)}
            >
              <Switch checked={record.status === 1} checkedChildren="启用" unCheckedChildren="禁用" />
            </Popconfirm>
          );
        },
      },
      {
        title: '创建时间',
        dataIndex: 'created_at',
        key: 'created_at',
        width: 160,
        render: (text) => (text ? dayjs(text as string).format('YYYY-MM-DD HH:mm:ss') : '-'),
      },
      {
        title: '操作',
        key: 'action',
        width: 220,
        fixed: 'right',
        render: (_text, record) => (
          <Space size={4}>
            <Tooltip title="权限设置">
              <Button
                type="link"
                size="small"
                icon={<SafetyOutlined />}
                disabled={!canUpdate}
                onClick={() => setPermsRole(record)}
              >
                权限
              </Button>
            </Tooltip>
            <Tooltip title="编辑">
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                disabled={!canUpdate || record.builtin === 1}
                onClick={() => handleOpenEdit(record)}
              />
            </Tooltip>
            <Tooltip title="删除">
              <Popconfirm
                title="确认删除该角色？"
                onConfirm={() => handleDelete(record)}
                disabled={record.builtin === 1 || !canDelete}
              >
                <Button
                  type="link"
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  disabled={record.builtin === 1 || !canDelete}
                />
              </Popconfirm>
            </Tooltip>
          </Space>
        ),
      },
    ],
    [canUpdate, canDelete, handleToggleStatus, handleOpenEdit, handleDelete],
  );

  if (!canView) {
    return (
      <PageContainer>
        <Card>
          <Text type="warning">您没有查看角色列表的权限（role:manage）。</Text>
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
          新建角色
        </Button>,
        <Button key="reload" icon={<ReloadOutlined />} loading={isFetching} onClick={handleReload}>
          刷新
        </Button>,
      ]}
    >
      <ProTable<Role>
        rowKey="id"
        loading={isFetching}
        columns={columns}
        dataSource={data?.items || []}
        scroll={{ x: 1100 }}
        search={false}
        headerTitle={
          <Input.Search
            placeholder="搜索角色编码/名称"
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

      {/* 新建角色 Drawer */}
      <Drawer
        title="新建角色"
        open={createOpen}
        width={440}
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
        <Form<CreateRoleBody> form={createForm} layout="vertical">
          <Form.Item
            name="code"
            label="角色编码"
            rules={[
              { required: true, min: 2, max: 64, message: '请输入 2-64 位角色编码' },
              { pattern: /^[a-z0-9_:]+$/, message: '仅支持小写字母、数字、下划线和冒号' },
            ]}
            tooltip="用于权限点关联，如 platform_admin / cluster_viewer"
          >
            <Input placeholder="如 cluster_viewer" />
          </Form.Item>
          <Form.Item name="name" label="角色名称" rules={[{ required: true, message: '请输入角色名称' }]}>
            <Input placeholder="如 集群观察者" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} placeholder="可选" />
          </Form.Item>
        </Form>
      </Drawer>

      {/* 编辑角色 Drawer */}
      <Drawer
        title={`编辑角色 - ${editingRole?.code || ''}`}
        open={!!editingRole}
        width={440}
        onClose={() => setEditingRole(null)}
        extra={
          <Space>
            <Button onClick={() => setEditingRole(null)}>取消</Button>
            <Button type="primary" loading={updateLoading} onClick={handleEdit}>
              保存
            </Button>
          </Space>
        }
      >
        <Form<UpdateRoleBody> form={editForm} layout="vertical">
          <Form.Item name="name" label="角色名称" rules={[{ required: true, message: '请输入角色名称' }]}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} />
          </Form.Item>
        </Form>
      </Drawer>

      {/* 权限设置 Drawer */}
      {permsRole && (
        <RolePermissionsDrawer role={permsRole} onClose={() => setPermsRole(null)} />
      )}
    </PageContainer>
  );
};

// 角色权限设置 Drawer
const RolePermissionsDrawer: React.FC<{ role: Role; onClose: () => void }> = ({
  role,
  onClose,
}) => {
  const { hasPerm } = usePermission();
  const canSet = hasPerm('role:manage');

  const { data: allPerms, isFetching: permsFetching } = useListPermissionsQuery();
  const { data: rolePerms, isFetching: rolePermsFetching } = useGetRolePermissionsQuery(role.id);
  const [setPerms, { isLoading: saveLoading }] = useSetRolePermissionsMutation();

  const [checkedKeys, setCheckedKeys] = useState<string[]>([]);

  // 权限树数据：按 module 分组
  const treeData = useMemo(() => {
    const grouped = new Map<string, Permission[]>();
    (allPerms || []).forEach((p) => {
      const mod = p.module || 'other';
      if (!grouped.has(mod)) grouped.set(mod, []);
      grouped.get(mod)!.push(p);
    });
    return Array.from(grouped.entries()).map(([mod, perms]) => ({
      key: `module:${mod}`,
      title: (
        <Space size={6}>
          <Text strong>{mod}</Text>
          <Tag>{perms.length}</Tag>
        </Space>
      ),
      children: perms.map((p) => ({
        key: p.code,
        title: (
          <Space size={6}>
            <Text>{p.name}</Text>
            <Text type="secondary" code style={{ fontSize: 11 }}>
              {p.code}
            </Text>
          </Space>
        ),
      })),
    }));
  }, [allPerms]);

  // 角色已有权限加载后同步到 checkedKeys（仅叶子节点 code）
  const leafCodes = useMemo(
    () => (allPerms || []).map((p) => p.code),
    [allPerms],
  );

  // 当 rolePerms 加载完成时初始化勾选
  React.useEffect(() => {
    if (rolePerms) {
      // 仅保留实际存在的叶子节点 code（后端可能存了已删除的权限点）
      setCheckedKeys((rolePerms.perms || []).filter((c) => leafCodes.includes(c)));
    }
  }, [rolePerms, leafCodes]);

  const handleCheck = useCallback(
    (checked: React.Key[] | { checked: React.Key[]; halfChecked: React.Key[] }) => {
      // checkStrictly 模式下 onCheck 返回 { checked, halfChecked }；否则返回 Key[]
      const keys = Array.isArray(checked) ? checked : checked.checked;
      // 仅保留叶子节点 code（去掉 module 分组 key，分组 key 以 "module:" 前缀）
      const leafOnly = keys.filter((k) => !String(k).startsWith('module:'));
      setCheckedKeys(leafOnly as string[]);
    },
    [],
  );

  const handleSave = useCallback(async () => {
    try {
      await setPerms({ id: role.id, perms: checkedKeys }).unwrap();
      message.success(`已保存 ${checkedKeys.length} 个权限点`);
      onClose();
    } catch {
      // interceptor
    }
  }, [setPerms, role.id, checkedKeys, onClose]);

  // 展开的 key（默认全部展开）
  const expandedKeys = useMemo(
    () => treeData.map((n) => n.key as string),
    [treeData],
  );

  return (
    <Drawer
      title={`权限设置 - ${role.name} (${role.code})`}
      open
      width={520}
      onClose={onClose}
      extra={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button
            type="primary"
            loading={saveLoading}
            disabled={!canSet}
            onClick={handleSave}
          >
            保存
          </Button>
        </Space>
      }
    >
      <Card size="small" style={{ marginBottom: 12 }}>
        <Space size={12}>
          <Text>已勾选权限：</Text>
          <Tag color="blue">{checkedKeys.length}</Tag>
          <Text type="secondary">/ 共 {leafCodes.length} 个权限点</Text>
        </Space>
      </Card>

      <Card
        size="small"
        loading={permsFetching || rolePermsFetching}
        bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
      >
        {treeData.length === 0 && !permsFetching ? (
          <Text type="secondary">暂无权限点数据</Text>
        ) : (
          <Tree
            checkable
            defaultExpandedKeys={expandedKeys}
            checkedKeys={checkedKeys}
            onCheck={handleCheck}
            treeData={treeData}
            selectable={false}
            // 父子节点不联动，避免模块分组 key 被当作权限点提交
            checkStrictly
          />
        )}
      </Card>

      <div style={{ marginTop: 16 }}>
        <Button onClick={onClose}>关闭</Button>
      </div>
    </Drawer>
  );
};

export default RoleList;
