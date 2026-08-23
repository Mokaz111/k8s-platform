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
  page?: number;
  size?: number;
}

export interface ListVersionsResponse {
  items: ResourceVersion[];
  total: number;
}

export interface GetVersionDiffParams {
  code: string;
  apiVersion: string;
  kind: string;
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
  apiVersion: string;
  kind: string;
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
      query: ({ code, apiVersion, kind, namespace, name, page, size }) => ({
        url: `/clusters/${code}/versions`,
        method: 'GET',
        params: {
          apiVersion,
          kind,
          namespace,
          name,
          page,
          size,
        } as Record<string, unknown>,
      }),
      providesTags: [{ type: 'Version', id: 'LIST' }],
    }),
    getVersionDiff: builder.query<VersionDiffResponse, GetVersionDiffParams>({
      query: ({ code, apiVersion, kind, namespace, name, seqA, seqB }) => ({
        url: `/clusters/${code}/versions/diff`,
        method: 'GET',
        params: {
          apiVersion,
          kind,
          namespace,
          name,
          seqA,
          seqB,
        } as Record<string, unknown>,
      }),
    }),
    rollbackVersion: builder.mutation<RollbackResponse, RollbackVersionParams>({
      query: ({ code, apiVersion, kind, namespace, name, seq }) => ({
        url: `/clusters/${code}/versions/rollback`,
        method: 'POST',
        data: { apiVersion, kind, namespace, name, seq },
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
