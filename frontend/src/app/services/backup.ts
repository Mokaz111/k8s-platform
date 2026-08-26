import React from 'react';
import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';
import { download as downloadFile } from './request';

export type StorageType = 'Local' | 'S3' | 'NFS';
export type BackupStatus = 'Running' | 'Success' | 'Failed' | 'Pending';

export type BackupMode = 'single' | 'namespace_batch';

export interface Backup {
  id: string;
  code: string;
  clusterName?: string;
  namespace?: string;
  namespaces?: string[];
  apiVersion: string;
  kind: string;
  kindFilter?: string[];
  name: string;
  mode?: BackupMode;
  backupType?: string; // single / namespace_batch / restore
  storageType: StorageType;
  size?: number;
  status: BackupStatus;
  operator?: string;
  createdAt?: string;
  finishedAt?: string;
  message?: string;
  downloadUrl?: string;
}

export interface ListBackupsParams {
  keyword?: string;
  code?: string;
  namespace?: string;
  kind?: string;
  storageType?: StorageType;
  status?: BackupStatus;
  page?: number;
  size?: number;
}

export interface ListBackupsResponse {
  items: Backup[];
  total: number;
}

export interface CreateBackupBody {
  mode?: 'single' | 'namespace_batch';
  namespace?: string;
  namespaces?: string[];
  // 单对象模式用：
  apiVersion?: string;
  target_kind?: string;
  kind?: string; // 兼容旧字段
  target_name?: string;
  name?: string; // 兼容旧字段
  // 批量模式用：
  kind_filter?: string[];
  storageType: StorageType;
  scope?: 'object' | 'namespace' | 'namespace_batch';
}

export interface CreateBackupParams {
  code: string;
  body: CreateBackupBody;
}

export type RestoreMode = 'overwrite' | 'create-new';

export interface RestoreBackupBody {
  targetCluster?: string;
  targetNamespace?: string;
  mode?: RestoreMode;
}

export interface RestoreParams {
  id: string;
  body?: RestoreBackupBody;
}

export const backupApi = createApi({
  reducerPath: 'backupApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['Backup'],
  endpoints: (builder) => ({
    listBackups: builder.query<ListBackupsResponse, ListBackupsParams | void>({
      query: (params) => ({
        url: '/backups',
        method: 'GET',
        params: params as Record<string, unknown> | undefined,
      }),
      providesTags: (result) =>
        result
          ? [
              ...result.items.map(({ id }) => ({ type: 'Backup' as const, id })),
              { type: 'Backup', id: 'LIST' },
            ]
          : [{ type: 'Backup', id: 'LIST' }],
    }),
    createBackup: builder.mutation<Backup, CreateBackupParams>({
      query: ({ code, body }) => ({
        url: `/clusters/${code}/backups`,
        method: 'POST',
        data: body,
      }),
      invalidatesTags: [{ type: 'Backup', id: 'LIST' }],
    }),
    getBackup: builder.query<Backup, string>({
      query: (id) => ({
        url: `/backups/${id}`,
        method: 'GET',
      }),
      providesTags: (_result, _err, id) => [{ type: 'Backup', id }],
    }),
    restoreBackup: builder.mutation<{ success: boolean; message?: string; taskId?: string }, RestoreParams>({
      query: ({ id, body }) => ({
        url: `/backups/${id}/restore`,
        method: 'POST',
        data: body,
      }),
      invalidatesTags: [{ type: 'Backup', id: 'LIST' }],
    }),
    deleteBackup: builder.mutation<void, string>({
      query: (id) => ({
        url: `/backups/${id}`,
        method: 'DELETE',
      }),
      invalidatesTags: (_result, _err, id) => [
        { type: 'Backup', id },
        { type: 'Backup', id: 'LIST' },
      ],
    }),
  }),
});

export const {
  useListBackupsQuery,
  useCreateBackupMutation,
  useGetBackupQuery,
  useRestoreBackupMutation,
  useDeleteBackupMutation,
} = backupApi;

// ============== downloadBackup: 基于 axios blob 的 hook ===========================
export interface DownloadBackupArgs {
  id: string;
  filename?: string;
}

export const useDownloadBackup = () => {
  const [loading, setLoading] = React.useState(false);

  const trigger = React.useCallback(
    async ({ id, filename }: DownloadBackupArgs): Promise<void> => {
      setLoading(true);
      try {
        const url = `/backups/${id}/download`;
        const dlName = filename || `backup-${id}.yaml`;
        await downloadFile(url, dlName);
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  return [trigger, loading] as const;
};

// 非 hook 版本，用于按钮直接调用
export const downloadBackupById = async (record: Backup): Promise<void> => {
  const isBatch =
    record.mode === 'namespace_batch' ||
    (record.namespaces && record.namespaces.length > 1) ||
    (record.kindFilter && record.kindFilter.length > 1);
  const ext = isBatch ? 'tar.gz' : 'yaml';
  const namePart = record.name || (isBatch ? 'batch' : record.kind) || record.id;
  const filename = `backup-${record.code}-${namePart}-${record.id.slice(0, 8)}.${ext}`;
  const url = record.downloadUrl || `/backups/${record.id}/download`;
  await downloadFile(url, filename);
};
