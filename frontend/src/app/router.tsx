import React from 'react';
import { createBrowserRouter, Navigate, useLocation } from 'react-router-dom';
import BasicLayout from '@/layouts/BasicLayout';
import Login from '@/pages/Login';
import Dashboard from '@/pages/dashboard';
import ClusterList from '@/pages/clusters/list';
import ClusterImport from '@/pages/clusters/import';
import ResourceList from '@/pages/resources/list';
import ResourceEdit from '@/pages/resources/[code]/[apiVersion]/[kind]/[namespace]/[name]/edit';
import QuotaManagement from '@/pages/resources/quota';
import BackupList from '@/pages/backups/list';
import BackupCreate from '@/pages/backups/create';
import PodLogsPage from '@/pages/pods/logs';
import AuditLogList from '@/pages/audit/list';
import UserList from '@/pages/users/list';
import RoleList from '@/pages/roles/list';
import HelmList from '@/pages/helm/list';
import { useAppSelector } from '@/app/store';

/**
 * 路由鉴权守卫：未登录跳转登录页并携带回跳地址。
 * 注意：react-router v6 的 loader 返回 JSX 不会被渲染，守卫必须用包装组件实现。
 */
const RequireAuth: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const token = useAppSelector((state) => state.user.token);
  const location = useLocation();
  if (!token) {
    return (
      <Navigate
        to={`/login?redirect=${encodeURIComponent(location.pathname + location.search)}`}
        replace
      />
    );
  }
  return <>{children}</>;
};

// 已登录用户访问 /login 时重定向到首页
const RedirectIfAuthed: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const token = useAppSelector((state) => state.user.token);
  if (token) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
};

export const router = createBrowserRouter([
  {
    path: '/login',
    element: (
      <RedirectIfAuthed>
        <Login />
      </RedirectIfAuthed>
    ),
  },
  {
    path: '/',
    element: (
      <RequireAuth>
        <BasicLayout />
      </RequireAuth>
    ),
    children: [
      {
        index: true,
        element: <Navigate to="/dashboard" replace />,
      },
      {
        path: 'dashboard',
        element: <Dashboard />,
      },
      {
        path: 'clusters',
        children: [
          {
            index: true,
            element: <Navigate to="/clusters/list" replace />,
          },
          {
            path: 'list',
            element: <ClusterList />,
          },
          {
            path: 'import',
            element: <ClusterImport />,
          },
        ],
      },
      {
        path: 'resources',
        children: [
          {
            index: true,
            element: <Navigate to="/resources/list" replace />,
          },
          {
            path: 'list',
            element: <ResourceList />,
          },
          {
            path: 'quota',
            element: <QuotaManagement />,
          },
          {
            path: 'workloads',
            element: <Navigate to="/resources/list?tab=workload" replace />,
          },
          {
            path: 'workloads/:type',
            element: <Navigate to="/resources/list?tab=workload" replace />,
          },
          {
            path: ':code/:apiVersion/:kind/:namespace/:name/edit',
            element: <ResourceEdit />,
          },
        ],
      },
      {
        path: 'backups',
        children: [
          {
            index: true,
            element: <Navigate to="/backups/list" replace />,
          },
          {
            path: 'list',
            element: <BackupList />,
          },
          {
            path: 'create',
            element: <BackupCreate />,
          },
        ],
      },
      {
        path: 'pods',
        children: [
          {
            index: true,
            element: <Navigate to="/pods/logs" replace />,
          },
          {
            path: 'logs',
            element: <PodLogsPage />,
          },
        ],
      },
      {
        path: 'rbac',
        children: [
          {
            index: true,
            element: <Navigate to="/rbac/users" replace />,
          },
          {
            path: 'users',
            element: <UserList />,
          },
          {
            path: 'roles',
            element: <RoleList />,
          },
        ],
      },
      {
        path: 'audit',
        children: [
          {
            index: true,
            element: <Navigate to="/audit/list" replace />,
          },
          {
            path: 'list',
            element: <AuditLogList />,
          },
        ],
      },
      {
        path: 'helm',
        children: [
          {
            index: true,
            element: <Navigate to="/helm/list" replace />,
          },
          {
            path: 'list',
            element: <HelmList />,
          },
        ],
      },
    ],
  },
  {
    path: '*',
    element: <Navigate to="/dashboard" replace />,
  },
]);
