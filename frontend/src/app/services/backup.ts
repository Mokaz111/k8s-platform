import React from 'react';
import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';
import { download as downloadFile } from './request';

/** 后端存储类型（normalizeStorageType 归一化为小写） */
export type StorageType = 'local' | 'nfs' | 's3';

/** 后端 models.BackupTaskStatus：pending / running / success / failed / cancelled */
export type BackupStatus = 'pending' | 'running' | 'success' | 'failed' | 'cancelled';

export type BackupMode = 'single' | 'namespace_batch';

/**
 * 与后端 models.BackupTask 的 JSON 输出对齐（snake_case）：
 * backup_type 取值 single / namespace_batch / restore（恢复任务）
 */
export interface Backup {
  id: number;
  cluster_code: string;
  namespace?: string | null;
  target_kind?: string | null;
  target_name?: string | null;
  backup_type: string;
  storage_type: string;
  storage_path?: string;
  status: BackupStatus;
  size_bytes?: number | null;
  operator?: string;
  operator_id?: number;
  started_at?: string | null;
  completed_at?: string | null;
  error_message?: string | null;
  object_count?: number;
  created_at?: string;
  updated_at?: string;
}

/**
 * 后端 ListBackups 支持的过滤参数（handler/backup.go ListBackups）：
 * cluster_code / status / keyword / backup_type + 分页
 */
export interface ListBackupsParams {
  keyword?: string;
  cluster_code?: string;
  status?: BackupStatus;
  backup_type?: string;
  page?: number;
  size?: number;
}

export interface ListBackupsResponse {
  items: Backup[];
  total: number;
}

/** 与后端 createBackupReq 对齐（snake_case） */
export interface CreateBackupBody {
  mode?: BackupMode;
  namespace?: string;
  namespaces?: string[];
  target_kind?: string;
  target_name?: string;
  kind_filter?: string[];
  storage_type?: StorageType;
}

export interface CreateBackupParams {
  code: string;
  body: CreateBackupBody;
}

export type RestoreMode = 'overwrite' | 'create-new';

/** 与后端 restoreBackupReq 对齐：target_cluster_code / mode / target_namespace */
export interface RestoreBackupBody {
  target_cluster_code?: string;
  target_namespace?: string;
  mode?: RestoreMode;
}

export interface RestoreParams {
  id: number;
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
        url: `/clusters/${encodeURIComponent(code)}/backups`,
        method: 'POST',
        data: body,
      }),
      invalidatesTags: [{ type: 'Backup', id: 'LIST' }],
    }),
    getBackup: builder.query<Backup, number>({
      query: (id) => ({
        url: `/backups/${id}`,
        method: 'GET',
      }),
      providesTags: (_result, _err, id) => [{ type: 'Backup', id }],
    }),
    /** 恢复任务提交后返回新建的 BackupTask（id 即任务 id，可接入 WS 进度） */
    restoreBackup: builder.mutation<Backup, RestoreParams>({
      query: ({ id, body }) => ({
        url: `/backups/${id}/restore`,
        method: 'POST',
        data: body,
      }),
      invalidatesTags: [{ type: 'Backup', id: 'LIST' }],
    }),
    deleteBackup: builder.mutation<void, number>({
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
  id: number;
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
  const isBatch = record.backup_type === 'namespace_batch';
  const ext = isBatch ? 'tar.gz' : 'yaml';
  const namePart = record.target_name || (isBatch ? 'batch' : record.target_kind) || record.id;
  const filename = `backup-${record.cluster_code}-${namePart}-${record.id}.${ext}`;
  const url = `/backups/${record.id}/download`;
  await downloadFile(url, filename);
};
