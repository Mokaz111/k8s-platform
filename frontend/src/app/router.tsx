import React, { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, useLocation } from 'react-router-dom';
import { Button, Result, Spin } from 'antd';
import BasicLayout from '@/layouts/BasicLayout';
import Login from '@/pages/Login';
import { useAppDispatch, useAppSelector } from '@/app/store';
import { fetchCurrentUser } from '@/slices/userSlice';
import { usePermission } from '@/hooks/usePermission';
import { ROUTE_PERMS, safeRedirectPath } from '@/app/routePerms';

const Dashboard = lazy(() => import('@/pages/dashboard'));
const ClusterList = lazy(() => import('@/pages/clusters/list'));
const ClusterImport = lazy(() => import('@/pages/clusters/import'));
const ResourceList = lazy(() => import('@/pages/resources/list'));
const ResourceEdit = lazy(
  () => import('@/pages/resources/[code]/[apiVersion]/[kind]/[namespace]/[name]/edit'),
);
const QuotaManagement = lazy(() => import('@/pages/resources/quota'));
const BackupList = lazy(() => import('@/pages/backups/list'));
const BackupCreate = lazy(() => import('@/pages/backups/create'));
const PodLogsPage = lazy(() => import('@/pages/pods/logs'));
const AuditLogList = lazy(() => import('@/pages/audit/list'));
const UserList = lazy(() => import('@/pages/users/list'));
const RoleList = lazy(() => import('@/pages/roles/list'));
const HelmList = lazy(() => import('@/pages/helm/list'));
const HelmRepos = lazy(() => import('@/pages/helm/repos'));
const ClusterCompare = lazy(() => import('@/pages/ops/compare'));

const PageFallback = (
  <div style={{ padding: 80, textAlign: 'center' }}>
    <Spin size="large" />
  </div>
);

const withPage = (node: React.ReactNode) => <Suspense fallback={PageFallback}>{node}</Suspense>;

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

const RequirePerm: React.FC<{ perms: string[]; children: React.ReactNode }> = ({
  perms,
  children,
}) => {
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector((s) => s.user.currentUser);
  const profileStatus = useAppSelector((s) => s.user.profileStatus);
  const { hasAnyPerm, isPlatformAdmin } = usePermission();
  if (profileStatus === 'failed' && !currentUser) {
    return (
      <Result
        status="error"
        title="无法加载权限"
        subTitle="当前会话有效，但未能取得账号权限。请重试或重新登录。"
        extra={
          <Button type="primary" onClick={() => dispatch(fetchCurrentUser())}>
            重试
          </Button>
        }
      />
    );
  }
  if (!currentUser) {
    return PageFallback;
  }
  if (isPlatformAdmin || hasAnyPerm(perms)) {
    return <>{children}</>;
  }
  return <Result status="403" title="无权访问" subTitle="当前账号没有该页面权限" />;
};

const guarded = (path: string, element: React.ReactNode) => {
  const perms = ROUTE_PERMS[path];
  if (!perms) {
    return withPage(element);
  }
  return withPage(<RequirePerm perms={perms}>{element}</RequirePerm>);
};

const RedirectIfAuthed: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const token = useAppSelector((state) => state.user.token);
  if (token) {
    return <Navigate to={safeRedirectPath('/dashboard')} replace />;
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
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: 'dashboard', element: withPage(<Dashboard />) },
      {
        path: 'clusters',
        children: [
          { index: true, element: <Navigate to="/clusters/list" replace /> },
          { path: 'list', element: guarded('/clusters/list', <ClusterList />) },
          { path: 'import', element: guarded('/clusters/import', <ClusterImport />) },
        ],
      },
      {
        path: 'resources',
        children: [
          { index: true, element: <Navigate to="/resources/list" replace /> },
          { path: 'list', element: guarded('/resources/list', <ResourceList />) },
          { path: 'quota', element: guarded('/resources/quota', <QuotaManagement />) },
          { path: 'workloads', element: <Navigate to="/resources/list?tab=workload" replace /> },
          { path: 'workloads/:type', element: <Navigate to="/resources/list?tab=workload" replace /> },
          {
            path: ':code/:apiVersion/:kind/:namespace/:name/edit',
            element: guarded('/resources/edit', <ResourceEdit />),
          },
        ],
      },
      {
        path: 'backups',
        children: [
          { index: true, element: <Navigate to="/backups/list" replace /> },
          { path: 'list', element: guarded('/backups/list', <BackupList />) },
          { path: 'create', element: guarded('/backups/create', <BackupCreate />) },
        ],
      },
      {
        path: 'pods',
        children: [
          { index: true, element: <Navigate to="/pods/logs" replace /> },
          { path: 'logs', element: guarded('/pods/logs', <PodLogsPage />) },
        ],
      },
      {
        path: 'ops',
        children: [
          { index: true, element: <Navigate to="/ops/compare" replace /> },
          { path: 'compare', element: guarded('/ops/compare', <ClusterCompare />) },
        ],
      },
      {
        path: 'rbac',
        children: [
          { index: true, element: <Navigate to="/rbac/users" replace /> },
          { path: 'users', element: guarded('/rbac/users', <UserList />) },
          { path: 'roles', element: guarded('/rbac/roles', <RoleList />) },
        ],
      },
      {
        path: 'audit',
        children: [
          { index: true, element: <Navigate to="/audit/list" replace /> },
          { path: 'list', element: guarded('/audit/list', <AuditLogList />) },
        ],
      },
      {
        path: 'helm',
        children: [
          { index: true, element: <Navigate to="/helm/list" replace /> },
          { path: 'list', element: guarded('/helm/list', <HelmList />) },
          { path: 'repos', element: guarded('/helm/repos', <HelmRepos />) },
        ],
      },
    ],
  },
  {
    path: '*',
    element: <Navigate to="/dashboard" replace />,
  },
]);
