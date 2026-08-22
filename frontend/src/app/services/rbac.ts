import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';

// 通用分页响应（后端 OKList 解包后为 { total, items }）
export interface PagedResponse<T> {
  items: T[];
  total: number;
}

// ---- 权限点 ----
export interface Permission {
  code: string;
  name: string;
  module: string;
  description?: string;
  action?: string;
}

// ---- 角色 ----
export interface Role {
  id: number;
  code: string;
  name: string;
  description?: string;
  builtin: number; // 1=内置 0=自定义
  status: number; // 1=启用 0=禁用
  perm_count?: number;
  user_count?: number;
  created_at?: string;
}

export interface ListRolesParams {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export interface CreateRoleBody {
  code: string;
  name: string;
  description?: string;
  perms?: string[];
}

export interface UpdateRoleBody {
  name?: string;
  description?: string;
  status?: number;
}

export interface RolePermissions {
  role_id: number;
  perms: string[];
}

// ---- 用户 ----
// 后端 UserStatus：1=Enabled 2=Disabled 3=Locked（前端按数值判断，宽松处理）
export type ScopeType = 'platform' | 'cluster' | 'namespace';

export interface User {
  id: number;
  username: string;
  display_name?: string;
  email?: string;
  phone?: string;
  auth_source?: string;
  status: number;
  last_login_at?: string;
  created_at?: string;
  role_count?: number;
}

export interface ListUsersParams {
  page?: number;
  page_size?: number;
  keyword?: string;
  status?: number;
}

export interface CreateUserBody {
  username: string;
  display_name?: string;
  email?: string;
  phone?: string;
  password: string;
  auth_source?: string;
  status?: number;
}

export interface UpdateUserBody {
  display_name?: string;
  email?: string;
  phone?: string;
  status?: number;
}

export interface UserRole {
  id: number;
  user_id: number;
  role_id: number;
  role_code: string;
  role_name: string;
  scope_type: ScopeType;
  cluster_code?: string;
  namespace?: string;
  created_at?: string;
}

export interface BindUserRoleBody {
  role_id: number;
  scope_type: ScopeType;
  cluster_code?: string;
  namespace?: string;
}

// ---- 审计日志 ----
export interface AuditLog {
  id?: number;
  trace_id?: string;
  user_id?: number;
  username?: string;
  client_ip?: string;
  user_agent?: string;
  module?: string;
  action?: string;
  target_type?: string;
  target_id?: string;
  cluster_code?: string;
  namespace?: string;
  status?: string;
  error_msg?: string;
  request_method?: string;
  request_uri?: string;
  request_body?: unknown;
  response_code?: number;
  cost_ms?: number;
  created_at?: string;
}

export interface ListAuditLogsParams {
  page?: number;
  page_size?: number;
  username?: string;
  module?: string;
  action?: string;
  status?: string;
  cluster_code?: string;
  namespace?: string;
  target_type?: string;
  target_id?: string;
  trace_id?: string;
  start_time?: string;
  end_time?: string;
}

export const rbacApi = createApi({
  reducerPath: 'rbacApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['User', 'Role', 'Permission', 'AuditLog'],
  endpoints: (builder) => ({
    // ---- 权限点 ----
    listPermissions: builder.query<Permission[], { module?: string } | void>({
      query: (params) => ({
        url: '/permissions',
        method: 'GET',
        params: params as Record<string, unknown> | undefined,
      }),
      providesTags: [{ type: 'Permission', id: 'LIST' }],
    }),

    // ---- 角色 ----
    listRoles: builder.query<PagedResponse<Role>, ListRolesParams | void>({
      query: (params) => ({
        url: '/roles',
        method: 'GET',
        params: params as Record<string, unknown> | undefined,
      }),
      providesTags: (result) =>
        result
          ? [
              ...result.items.map(({ id }) => ({ type: 'Role' as const, id })),
              { type: 'Role', id: 'LIST' },
            ]
          : [{ type: 'Role', id: 'LIST' }],
    }),
    createRole: builder.mutation<{ code: string }, CreateRoleBody>({
      query: (body) => ({ url: '/roles', method: 'POST', data: body }),
      invalidatesTags: [{ type: 'Role', id: 'LIST' }],
    }),
    updateRole: builder.mutation<{ id: number }, { id: number; body: UpdateRoleBody }>({
      query: ({ id, body }) => ({ url: `/roles/${id}`, method: 'PUT', data: body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'Role', id },
        { type: 'Role', id: 'LIST' },
      ],
    }),
    deleteRole: builder.mutation<{ id: number }, number>({
      query: (id) => ({ url: `/roles/${id}`, method: 'DELETE' }),
      invalidatesTags: (_r, _e, id) => [
        { type: 'Role', id },
        { type: 'Role', id: 'LIST' },
      ],
    }),
    getRolePermissions: builder.query<RolePermissions, number>({
      query: (id) => ({ url: `/roles/${id}/permissions`, method: 'GET' }),
      providesTags: (_r, _e, id) => [{ type: 'Permission', id: `ROLE-${id}` }],
    }),
    setRolePermissions: builder.mutation<
      { id: number; perm_count: number },
      { id: number; perms: string[] }
    >({
      query: ({ id, perms }) => ({
        url: `/roles/${id}/permissions`,
        method: 'POST',
        data: { perms },
      }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'Permission', id: `ROLE-${id}` },
        { type: 'Role', id },
        { type: 'Role', id: 'LIST' },
      ],
    }),

    // ---- 用户 ----
    listUsers: builder.query<PagedResponse<User>, ListUsersParams | void>({
      query: (params) => ({
        url: '/users',
        method: 'GET',
        params: params as Record<string, unknown> | undefined,
      }),
      providesTags: (result) =>
        result
          ? [
              ...result.items.map(({ id }) => ({ type: 'User' as const, id })),
              { type: 'User', id: 'LIST' },
            ]
          : [{ type: 'User', id: 'LIST' }],
    }),
    createUser: builder.mutation<{ id: number; username: string }, CreateUserBody>({
      query: (body) => ({ url: '/users', method: 'POST', data: body }),
      invalidatesTags: [{ type: 'User', id: 'LIST' }],
    }),
    updateUser: builder.mutation<{ id: number }, { id: number; body: UpdateUserBody }>({
      query: ({ id, body }) => ({ url: `/users/${id}`, method: 'PUT', data: body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'User', id },
        { type: 'User', id: 'LIST' },
      ],
    }),
    updateUserStatus: builder.mutation<{ id: number; status: number }, { id: number; status: number }>({
      query: ({ id, status }) => ({
        url: `/users/${id}/status`,
        method: 'PATCH',
        data: { status },
      }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'User', id },
        { type: 'User', id: 'LIST' },
      ],
    }),
    resetPassword: builder.mutation<{ id: number }, { id: number; password: string }>({
      query: ({ id, password }) => ({
        url: `/users/${id}/reset-password`,
        method: 'POST',
        data: { password },
      }),
    }),
    listUserRoles: builder.query<UserRole[], number>({
      query: (id) => ({ url: `/users/${id}/roles`, method: 'GET' }),
      providesTags: (_r, _e, id) => [{ type: 'User', id: `ROLES-${id}` }],
    }),
    bindUserRole: builder.mutation<{ id: number }, { id: number; body: BindUserRoleBody }>({
      query: ({ id, body }) => ({ url: `/users/${id}/roles`, method: 'POST', data: body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'User', id: `ROLES-${id}` },
        { type: 'User', id },
        { type: 'User', id: 'LIST' },
      ],
    }),
    unbindUserRole: builder.mutation<{ deleted: number }, { id: number; body: BindUserRoleBody }>({
      // DELETE 携带 body（后端 UnbindUserRole 从 body 读取 role_id/scope_type/cluster_code/namespace）
      query: ({ id, body }) => ({ url: `/users/${id}/roles`, method: 'DELETE', data: body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'User', id: `ROLES-${id}` },
        { type: 'User', id },
        { type: 'User', id: 'LIST' },
      ],
    }),

    // ---- 审计日志 ----
    listAuditLogs: builder.query<PagedResponse<AuditLog>, ListAuditLogsParams | void>({
      query: (params) => ({
        url: '/audit-logs',
        method: 'GET',
        params: params as Record<string, unknown> | undefined,
      }),
      providesTags: [{ type: 'AuditLog', id: 'LIST' }],
    }),
  }),
});

export const {
  useListPermissionsQuery,
  useListRolesQuery,
  useCreateRoleMutation,
  useUpdateRoleMutation,
  useDeleteRoleMutation,
  useGetRolePermissionsQuery,
  useSetRolePermissionsMutation,
  useListUsersQuery,
  useCreateUserMutation,
  useUpdateUserMutation,
  useUpdateUserStatusMutation,
  useResetPasswordMutation,
  useListUserRolesQuery,
  useBindUserRoleMutation,
  useUnbindUserRoleMutation,
  useListAuditLogsQuery,
} = rbacApi;
