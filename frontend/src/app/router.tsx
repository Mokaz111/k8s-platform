import { createBrowserRouter, Navigate } from 'react-router-dom';
import NProgress from 'nprogress';
import BasicLayout from '@/layouts/BasicLayout';
import Login from '@/pages/Login';
import Dashboard from '@/pages/dashboard';
import ClusterList from '@/pages/clusters/list';
import ClusterImport from '@/pages/clusters/import';
import ResourceList from '@/pages/resources/list';
import ResourceEdit from '@/pages/resources/[code]/[apiVersion]/[kind]/[namespace]/[name]/edit';
import BackupList from '@/pages/backups/list';
import BackupCreate from '@/pages/backups/create';
import PodLogsPage from '@/pages/pods/logs';
import AuditLogList from '@/pages/audit/list';
import UserList from '@/pages/users/list';
import RoleList from '@/pages/roles/list';
import { store } from '@/app/store';

const authGuard = () => {
  NProgress.start();
  setTimeout(() => NProgress.done(), 200);
  const token = store.getState().user.token;
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return null;
};

const loginGuard = () => {
  NProgress.start();
  setTimeout(() => NProgress.done(), 200);
  const token = store.getState().user.token;
  if (token) {
    return <Navigate to="/" replace />;
  }
  return null;
};

export const router = createBrowserRouter([
  {
    path: '/login',
    element: <Login />,
    loader: loginGuard,
  },
  {
    path: '/',
    element: <BasicLayout />,
    loader: authGuard,
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
    ],
  },
  {
    path: '*',
    element: <Navigate to="/dashboard" replace />,
  },
]);
