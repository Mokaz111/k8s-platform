import React, { useEffect, useMemo } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import { ProLayout } from '@ant-design/pro-components';
import { Avatar, Badge, Dropdown, Select, Space, Tooltip, message } from 'antd';
import {
  DashboardOutlined,
  ClusterOutlined,
  ImportOutlined,
  AppstoreOutlined,
  UserOutlined,
  LogoutOutlined,
  DownOutlined,
  DatabaseOutlined,
  CloudServerOutlined,
  FileTextOutlined,
  SafetyOutlined,
  TeamOutlined,
  AuditOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { logout, fetchCurrentUser } from '@/slices/userSlice';
import { setSelectedClusterCode } from '@/slices/appSlice';
import { selectWSStatus } from '@/slices/wsSlice';
import { useListClustersQuery } from '@/app/services/cluster';
import { useWebSocketLifecycle } from '@/hooks/useWebSocket';
import { usePermission } from '@/hooks/usePermission';

// 菜单路由的权限配置：route path -> 所需权限点（任一满足即可显示）
// 没有配置 perm 的菜单默认对所有登录用户显示
// 注意：权限码必须与后端 models/seed.go 的 seedPermissionsData 完全一致，
// 否则普通角色（cluster-admin / platform-viewer 等）菜单会被全部隐藏
const MENU_PERM_MAP: Record<string, string[]> = {
  // 概览：后端无 dashboard 权限点，对所有登录用户开放
  // 集群管理：只要有任何 cluster 相关权限就显示分组
  '/clusters': ['cluster:list', 'cluster:create', 'cluster:update', 'cluster:delete', 'cluster:ping'],
  '/clusters/list': ['cluster:list'],
  '/clusters/import': ['cluster:create'],
  // 资源管理
  '/resources': ['resource:list', 'resource:get', 'resource:create', 'resource:update', 'resource:delete'],
  '/resources/list': ['resource:list'],
  '/resources/quota': ['resource:list'],
  // 备份管理
  '/backups': ['backup:list', 'backup:create', 'backup:restore', 'backup:delete'],
  '/backups/list': ['backup:list'],
  '/backups/create': ['backup:create'],
  // 运维工具（Pod 日志）：后端无独立权限点，访问控制由 RequireNamespaceScope 数据范围校验
  // 权限管理
  '/rbac': ['user:manage', 'role:manage'],
  '/rbac/users': ['user:manage'],
  '/rbac/roles': ['role:manage'],
  // 审计日志
  '/audit': ['audit:list'],
  '/audit/list': ['audit:list'],
  // Helm 应用管理
  '/helm': ['helm:view', 'helm:install', 'helm:uninstall', 'helm:rollback'],
  '/helm/list': ['helm:view'],
};

type MenuRoute = {
  path: string;
  name?: string;
  icon?: React.ReactNode;
  routes?: MenuRoute[];
};

/**
 * 根据权限点过滤菜单：
 * - 叶子节点：有 perm 配置则必须有任一权限
 * - 父节点：子节点过滤后非空才保留
 */
function filterRoutesByPermission(
  routes: MenuRoute[],
  hasAnyPerm: (codes: string[]) => boolean,
  isPlatformAdmin: boolean,
): MenuRoute[] {
  if (isPlatformAdmin) return routes;
  const result: MenuRoute[] = [];
  for (const r of routes) {
    const perms = MENU_PERM_MAP[r.path];
    if (r.routes && r.routes.length > 0) {
      const children = filterRoutesByPermission(r.routes, hasAnyPerm, isPlatformAdmin);
      if (children.length === 0) {
        // 子菜单全被过滤：如果父节点本身配置了 perm 并且有，则单独显示
        if (perms && perms.length > 0 && hasAnyPerm(perms)) {
          result.push({ ...r, routes: undefined });
        }
        continue;
      }
      result.push({ ...r, routes: children });
      continue;
    }
    // 叶子节点
    if (!perms || perms.length === 0 || hasAnyPerm(perms)) {
      result.push(r);
    }
  }
  return result;
}

// WebSocket 连接状态徽标：实时显示 ws 连接状态
const WSStatusBadge: React.FC = () => {
  const status = useAppSelector(selectWSStatus);
  const statusMap: Record<string, { status: 'success' | 'processing' | 'warning' | 'error' | 'default'; label: string; color: string }> = {
    idle: { status: 'default', label: '未连接', color: '#d9d9d9' },
    connecting: { status: 'processing', label: '连接中', color: '#1677ff' },
    open: { status: 'success', label: '已连接', color: '#52c41a' },
    closing: { status: 'default', label: '关闭中', color: '#faad14' },
    closed: { status: 'default', label: '已断开', color: '#d9d9d9' },
    reconnecting: { status: 'warning', label: '重连中', color: '#faad14' },
    error: { status: 'error', label: '连接错误', color: '#ff4d4f' },
  };
  const cfg = statusMap[status] || statusMap.idle;
  return (
    <Tooltip title={`WebSocket: ${cfg.label}`}>
      <Badge status={cfg.status} text={<span style={{ color: 'rgba(0,0,0,0.65)', fontSize: 12 }}>{cfg.label}</span>} />
    </Tooltip>
  );
};

const BasicLayout: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const { currentUser, token } = useAppSelector((state) => state.user);
  const { selectedClusterCode, collapsed } = useAppSelector((state) => state.app);
  const { hasAnyPerm, isPlatformAdmin } = usePermission();

  // WebSocket 全局连接生命周期：token 存在时自动建立连接
  useWebSocketLifecycle();

  // 有 token 但无用户信息（如刷新页面/StrictMode 重挂）时自动补拉当前用户与权限点，
  // 否则权限过滤会把所有菜单隐藏、hasPerm 全部失效
  useEffect(() => {
    if (token && !currentUser) {
      dispatch(fetchCurrentUser());
    }
  }, [token, currentUser, dispatch]);

  const { data, isLoading } = useListClustersQuery(undefined, {
    skip: !token,
  });
  const clusters = data?.items || [];

  useEffect(() => {
    if (!selectedClusterCode && clusters.length > 0) {
      dispatch(setSelectedClusterCode(clusters[0].code));
    }
  }, [clusters, selectedClusterCode, dispatch]);

  const handleLogout = () => {
    dispatch(logout());
    message.success('已退出登录');
    navigate('/login', { replace: true });
  };

  const userMenuItems = [
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      label: '退出登录',
      onClick: handleLogout,
    },
  ];

  const rawRoutes: MenuRoute[] = [
          {
            path: '/dashboard',
            name: '概览',
            icon: <DashboardOutlined />,
          },
          {
            path: '/clusters',
            name: '集群管理',
            icon: <ClusterOutlined />,
            routes: [
              {
                path: '/clusters/list',
                name: '集群列表',
                icon: <ClusterOutlined />,
              },
              {
                path: '/clusters/import',
                name: '导入集群',
                icon: <ImportOutlined />,
              },
            ],
          },
          {
            path: '/resources',
            name: '资源管理',
            icon: <AppstoreOutlined />,
            routes: [
              {
                path: '/resources/list',
                name: '资源列表',
                icon: <AppstoreOutlined />,
              },
              {
                path: '/resources/quota',
                name: '配额管理',
                icon: <DashboardOutlined />,
              },
            ],
          },
          {
            path: '/backups',
            name: '备份管理',
            icon: <DatabaseOutlined />,
            routes: [
              {
                path: '/backups/list',
                name: '备份列表',
                icon: <DatabaseOutlined />,
              },
              {
                path: '/backups/create',
                name: '新建备份',
                icon: <CloudServerOutlined />,
              },
            ],
          },
          {
            path: '/pods',
            name: '运维工具',
            icon: <FileTextOutlined />,
            routes: [
              {
                path: '/pods/logs',
                name: 'Pod 日志',
                icon: <FileTextOutlined />,
              },
            ],
          },
          {
            path: '/rbac',
            name: '权限管理',
            icon: <SafetyOutlined />,
            routes: [
              {
                path: '/rbac/users',
                name: '用户管理',
                icon: <TeamOutlined />,
              },
              {
                path: '/rbac/roles',
                name: '角色管理',
                icon: <SafetyOutlined />,
              },
            ],
          },
          {
            path: '/audit',
            name: '审计日志',
            icon: <AuditOutlined />,
            routes: [
              {
                path: '/audit/list',
                name: '操作日志',
                icon: <AuditOutlined />,
              },
            ],
          },
          {
            path: '/helm',
            name: 'Helm 应用',
            icon: <ThunderboltOutlined />,
            routes: [
              {
                path: '/helm/list',
                name: 'Release 列表',
                icon: <ThunderboltOutlined />,
              },
            ],
          },
  ];

  const filteredRoute = useMemo(
    () => ({
      path: '/',
      routes: filterRoutesByPermission(rawRoutes, hasAnyPerm, isPlatformAdmin),
    }),
    [rawRoutes, hasAnyPerm, isPlatformAdmin],
  );

  return (
    <ProLayout
      layout="mix"
      splitMenus={false}
      fixSiderbar
      collapsed={collapsed}
      route={filteredRoute}
      menuItemRender={(itemProps, defaultDom) => {
        return <a onClick={() => navigate(itemProps.path || '/')}>{defaultDom}</a>;
      }}
      avatarProps={{
        src: currentUser?.avatar,
        icon: <UserOutlined />,
      }}
      actionsRender={() => [
        <Select
          key="cluster"
          style={{ width: 220 }}
          placeholder="选择集群"
          value={selectedClusterCode}
          loading={isLoading}
          onChange={(value) => dispatch(setSelectedClusterCode(value))}
          options={clusters.map((c) => ({ value: c.code, label: `${c.name} (${c.code})` }))}
          allowClear
        />,
        <WSStatusBadge key="ws-status" />,
      ]}
      rightContentRender={() => [
        <Dropdown key="user" menu={{ items: userMenuItems }} placement="bottomRight">
          <Space style={{ cursor: 'pointer', padding: '0 8px' }}>
            <Avatar src={currentUser?.avatar} icon={<UserOutlined />} />
            <span style={{ color: 'rgba(0, 0, 0, 0.88)' }}>
              {currentUser?.nickname || currentUser?.username || '用户'}
            </span>
            <DownOutlined style={{ fontSize: 12 }} />
          </Space>
        </Dropdown>,
      ]}
      title="管理控制台"
      pageTitleRender={false}
    >
      <Outlet />
    </ProLayout>
  );
};

export default BasicLayout;
