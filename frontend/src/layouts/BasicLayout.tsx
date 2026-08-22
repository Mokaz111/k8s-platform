import React, { useEffect } from 'react';
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
} from '@ant-design/icons';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { logout } from '@/slices/userSlice';
import { setSelectedClusterCode } from '@/slices/appSlice';
import { selectWSStatus } from '@/slices/wsSlice';
import { useListClustersQuery } from '@/app/services/cluster';
import { useWebSocketLifecycle } from '@/hooks/useWebSocket';

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

  // WebSocket 全局连接生命周期：token 存在时自动建立连接
  useWebSocketLifecycle();

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

  return (
    <ProLayout
      layout="mix"
      splitMenus={false}
      fixSiderbar
      collapsed={collapsed}
      route={{
        path: '/',
        routes: [
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
        ],
      }}
      menuItemRender={(itemProps, defaultDom) => {
        return <a onClick={() => navigate(itemProps.path)}>{defaultDom}</a>;
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
