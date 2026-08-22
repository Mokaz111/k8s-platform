import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';

export interface Cluster {
  code: string;
  name: string;
  description?: string;
  version?: string;
  nodes?: number;
  status: 'Online' | 'Offline' | string;
  lastSyncTime?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface ListClustersParams {
  keyword?: string;
  page?: number;
  size?: number;
}

export interface ListClustersResponse {
  items: Cluster[];
  total: number;
}

export interface ImportClusterData {
  name: string;
  code: string;
  description?: string;
  kubeconfig?: string;
}

export interface UpdateClusterData {
  name?: string;
  description?: string;
}

export interface PingResponse {
  success: boolean;
  message?: string;
  version?: string;
  nodes?: number;
}

export interface TempPingData {
  kubeconfig_text?: string;
}

export const clusterApi = createApi({
  reducerPath: 'clusterApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['Cluster'],
  endpoints: (builder) => ({
    listClusters: builder.query<ListClustersResponse, ListClustersParams | void>({
      query: (params) => ({
        url: '/clusters',
        method: 'GET',
        params: params as Record<string, unknown> | undefined,
      }),
      providesTags: (result) =>
        result
          ? [
              ...result.items.map(({ code }) => ({ type: 'Cluster' as const, id: code })),
              { type: 'Cluster', id: 'LIST' },
            ]
          : [{ type: 'Cluster', id: 'LIST' }],
    }),
    getCluster: builder.query<Cluster, string>({
      query: (code) => ({
        url: `/clusters/${code}`,
        method: 'GET',
      }),
      providesTags: (_result, _err, code) => [{ type: 'Cluster', id: code }],
    }),
    importCluster: builder.mutation<Cluster, ImportClusterData>({
      query: (data) => ({
        url: '/clusters',
        method: 'POST',
        data,
      }),
      invalidatesTags: [{ type: 'Cluster', id: 'LIST' }],
    }),
    updateCluster: builder.mutation<Cluster, { code: string; data: UpdateClusterData }>({
      query: ({ code, data }) => ({
        url: `/clusters/${code}`,
        method: 'PUT',
        data,
      }),
      invalidatesTags: (_result, _err, { code }) => [
        { type: 'Cluster', id: code },
        { type: 'Cluster', id: 'LIST' },
      ],
    }),
    deleteCluster: builder.mutation<void, string>({
      query: (code) => ({
        url: `/clusters/${code}`,
        method: 'DELETE',
      }),
      invalidatesTags: (_result, _err, code) => [
        { type: 'Cluster', id: code },
        { type: 'Cluster', id: 'LIST' },
      ],
    }),
    pingCluster: builder.mutation<PingResponse, string>({
      query: (code) => ({
        url: `/clusters/${code}/ping`,
        method: 'POST',
      }),
      invalidatesTags: (_result, _err, code) => [{ type: 'Cluster', id: code }],
    }),
    tempPing: builder.mutation<PingResponse, TempPingData>({
      query: (data) => ({
        url: '/clusters/_/ping',
        method: 'POST',
        data,
      }),
    }),
  }),
});

export const {
  useListClustersQuery,
  useGetClusterQuery,
  useImportClusterMutation,
  useUpdateClusterMutation,
  useDeleteClusterMutation,
  usePingClusterMutation,
  useTempPingMutation,
} = clusterApi;
