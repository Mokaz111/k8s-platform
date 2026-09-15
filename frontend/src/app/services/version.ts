import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';

/**
 * 与后端 models.ResourceSnapshot 的 JSON 输出对齐（snake_case）
 * （GET /versions 返回 OKList → { items, total }）
 */
export interface ResourceVersion {
  id: number;
  cluster_code: string;
  namespace?: string;
  api_version: string;
  kind: string;
  name: string;
  version_seq: number;
  raw_yaml?: string;
  change_summary?: string;
  operator?: string;
  source?: string; // ui / rollback / backup
  operator_id?: number;
  created_at?: string;
  updated_at?: string;
}

export interface ListVersionsParams {
  code: string;
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
}

export interface ListVersionsResponse {
  items: ResourceVersion[];
  total: number;
}

/** 0 表示集群中的当前对象，而不是历史快照序号 */
export const CURRENT_VERSION_SEQ = 0;

export function versionSeqLabel(seq: number): string {
  return seq === CURRENT_VERSION_SEQ ? '当前版本' : `#${seq}`;
}

export interface GetVersionDiffParams {
  code: string;
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
  /** 历史序号；0 表示当前集群 YAML */
  seqA: number;
  /** 历史序号；0 表示当前集群 YAML */
  seqB: number;
}

/** GET /versions/diff 响应（后端 handler/version.go DiffVersions） */
export interface VersionDiffResponse {
  seq_a: number;
  seq_b: number;
  yaml_a?: string;
  yaml_b?: string;
}

export interface RollbackVersionParams {
  code: string;
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
  seq: number;
}

/** POST /versions/rollback 响应 */
export interface RollbackResponse {
  rolled_back: boolean;
  seq: number;
  object?: unknown;
}

export const versionApi = createApi({
  reducerPath: 'versionApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['Version'],
  endpoints: (builder) => ({
    // 后端路由：GET /api/v1/versions?cluster_code=&namespace=&api_version=&kind=&name=
    listVersions: builder.query<ListVersionsResponse, ListVersionsParams>({
      query: ({ code, apiVersion, kind, namespace, name }) => ({
        url: '/versions',
        method: 'GET',
        params: {
          cluster_code: code,
          namespace,
          api_version: apiVersion,
          kind,
          name,
        } as Record<string, unknown>,
      }),
      providesTags: [{ type: 'Version', id: 'LIST' }],
    }),
    // 后端路由：GET /api/v1/versions/diff?cluster_code=&...&seq_a=&seq_b=
    getVersionDiff: builder.query<VersionDiffResponse, GetVersionDiffParams>({
      query: ({ code, apiVersion, kind, namespace, name, seqA, seqB }) => ({
        url: '/versions/diff',
        method: 'GET',
        params: {
          cluster_code: code,
          namespace,
          api_version: apiVersion,
          kind,
          name,
          seq_a: seqA,
          seq_b: seqB,
        } as Record<string, unknown>,
      }),
    }),
    // 后端路由：POST /api/v1/versions/rollback（body 为 snake_case）
    rollbackVersion: builder.mutation<RollbackResponse, RollbackVersionParams>({
      query: ({ code, apiVersion, kind, namespace, name, seq }) => ({
        url: '/versions/rollback',
        method: 'POST',
        data: {
          cluster_code: code,
          namespace,
          api_version: apiVersion,
          kind,
          name,
          seq,
        },
      }),
      invalidatesTags: [{ type: 'Version', id: 'LIST' }],
    }),
  }),
});

export const {
  useListVersionsQuery,
  useLazyGetVersionDiffQuery,
  useRollbackVersionMutation,
} = versionApi;
