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
  kubeconfig_text?: string;
  kubeconfig_base64?: string;
  labels?: string;
}

export interface UpdateClusterData {
  name?: string;
  description?: string;
  kubeconfig_text?: string;
  kubeconfig_base64?: string;
  labels?: string;
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

export interface StreamPodLogsParams {
  clusterCode: string;
  namespace: string;
  pod: string;
  container?: string;
  follow?: boolean;
  tail?: number;
}

export interface StreamPodLogsResponse {
  channel: string;
  follow: boolean;
  tail: number;
  container: string;
  pod: string;
  namespace: string;
  cluster: string;
  ws_endpoint: string;
  action: 'subscribe';
  hint: string;
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
    streamPodLogs: builder.query<StreamPodLogsResponse, StreamPodLogsParams>({
      query: ({ clusterCode, namespace, pod, container, follow, tail }) => {
        const params: Record<string, unknown> = {};
        if (container) params.container = container;
        if (follow !== undefined) params.follow = follow ? '1' : '0';
        if (tail !== undefined && tail > 0) params.tail = String(tail);
        return {
          url: `/clusters/${clusterCode}/pods/${namespace}/${pod}/logs`,
          method: 'GET',
          params,
        };
      },
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
  useStreamPodLogsQuery,
  useLazyStreamPodLogsQuery,
} = clusterApi;
