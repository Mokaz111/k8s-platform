import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';

export interface ResourceVersion {
  seq: number;
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
  resourceVersion?: string;
  operation?: 'CREATE' | 'UPDATE' | 'DELETE' | string;
  operator?: string;
  yaml?: string;
  diff?: unknown;
  createdAt?: string;
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

export interface GetVersionDiffParams {
  code: string;
  gvk: string;
  namespace?: string;
  name: string;
  seqA: number;
  seqB: number;
}

export interface VersionDiffResponse {
  seqA: number;
  seqB: number;
  yamlA?: string;
  yamlB?: string;
  diff?: string;
}

export interface RollbackVersionParams {
  code: string;
  gvk: string;
  namespace?: string;
  name: string;
  seq: number;
}

export interface RollbackResponse {
  success: boolean;
  message?: string;
  newSeq?: number;
}

export const versionApi = createApi({
  reducerPath: 'versionApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['Version'],
  endpoints: (builder) => ({
    listVersions: builder.query<ListVersionsResponse, ListVersionsParams>({
      query: ({ code, apiVersion, kind, namespace, name }) => ({
        url: namespace
          ? `/clusters/${code}/versions/${apiVersion}/${kind}/${namespace}/${name}`
          : `/clusters/${code}/versions/${apiVersion}/${kind}/${name}`,
        method: 'GET',
      }),
      providesTags: [{ type: 'Version', id: 'LIST' }],
    }),
    getVersionDiff: builder.query<VersionDiffResponse, GetVersionDiffParams>({
      query: ({ code, gvk, namespace, name, seqA, seqB }) => ({
        url: namespace
          ? `/clusters/${code}/versions/diff/${gvk}/${namespace}/${name}`
          : `/clusters/${code}/versions/diff/${gvk}/${name}`,
        method: 'GET',
        params: { seqA, seqB } as Record<string, unknown>,
      }),
    }),
    rollbackVersion: builder.mutation<RollbackResponse, RollbackVersionParams>({
      query: ({ code, gvk, namespace, name, seq }) => ({
        url: namespace
          ? `/clusters/${code}/versions/rollback/${gvk}/${namespace}/${name}`
          : `/clusters/${code}/versions/rollback/${gvk}/${name}`,
        method: 'POST',
        data: { seq },
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
