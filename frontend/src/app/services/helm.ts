import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';

export interface HelmRelease {
  name: string;
  namespace: string;
  revision: string;
  status: string;
  chart: string;
  appVersion: string;
  updated: string;
}

export interface ListReleasesParams {
  clusterCode: string;
  namespace?: string;
}

export interface ListReleasesResponse {
  items: HelmRelease[];
  total: number;
}

export interface InstallReleaseBody {
  release_name: string;
  namespace: string;
  chart_ref: string;
  repo_url?: string;
  values?: string;
  version?: string;
  wait?: boolean;
  dry_run?: boolean;
}

export interface InstallReleaseParams {
  clusterCode: string;
  body: InstallReleaseBody;
}

export interface HelmHistoryItem {
  revision: number;
  status: string;
  chart: string;
  app_version: string;
  description: string;
  updated: string;
}

export const helmApi = createApi({
  reducerPath: 'helmApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['HelmRelease'],
  endpoints: (builder) => ({
    listReleases: builder.query<ListReleasesResponse, ListReleasesParams>({
      query: ({ clusterCode, namespace }) => ({
        url: `/clusters/${clusterCode}/helm/releases`,
        method: 'GET',
        params: namespace ? { namespace } : undefined,
      }),
      providesTags: [{ type: 'HelmRelease', id: 'LIST' }],
    }),
    installRelease: builder.mutation<{ output: string }, InstallReleaseParams>({
      query: ({ clusterCode, body }) => ({
        url: `/clusters/${clusterCode}/helm/releases`,
        method: 'POST',
        data: body,
      }),
      invalidatesTags: [{ type: 'HelmRelease', id: 'LIST' }],
    }),
    uninstallRelease: builder.mutation<
      { output: string },
      { clusterCode: string; namespace: string; name: string }
    >({
      query: ({ clusterCode, namespace, name }) => ({
        url: `/clusters/${clusterCode}/helm/releases/${namespace}/${name}`,
        method: 'DELETE',
      }),
      invalidatesTags: [{ type: 'HelmRelease', id: 'LIST' }],
    }),
    rollbackRelease: builder.mutation<
      { output: string },
      { clusterCode: string; namespace: string; name: string; revision?: number }
    >({
      query: ({ clusterCode, namespace, name, revision }) => ({
        url: `/clusters/${clusterCode}/helm/releases/${namespace}/${name}/rollback`,
        method: 'POST',
        params: revision ? { revision } : undefined,
      }),
      invalidatesTags: [{ type: 'HelmRelease', id: 'LIST' }],
    }),
    listHistory: builder.query<
      { items: HelmHistoryItem[] },
      { clusterCode: string; namespace: string; name: string }
    >({
      query: ({ clusterCode, namespace, name }) => ({
        url: `/clusters/${clusterCode}/helm/releases/${namespace}/${name}/history`,
        method: 'GET',
      }),
    }),
  }),
});

export const {
  useListReleasesQuery,
  useInstallReleaseMutation,
  useUninstallReleaseMutation,
  useRollbackReleaseMutation,
  useListHistoryQuery,
} = helmApi;
