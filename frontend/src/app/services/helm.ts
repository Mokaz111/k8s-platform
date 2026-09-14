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

export interface HelmRepo {
  name: string;
  url: string;
}

export interface AddRepoBody {
  name: string;
  url: string;
  username?: string;
  password?: string;
}

export interface HelmChart {
  name: string;
  version: string;
  app_version: string;
  description: string;
}

const enc = (s: string): string => encodeURIComponent(s);

export const helmApi = createApi({
  reducerPath: 'helmApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['HelmRelease', 'HelmRepo', 'HelmChart'],
  endpoints: (builder) => ({
    listReleases: builder.query<ListReleasesResponse, ListReleasesParams>({
      query: ({ clusterCode, namespace }) => ({
        url: `/clusters/${enc(clusterCode)}/helm/releases`,
        method: 'GET',
        params: namespace ? { namespace } : undefined,
      }),
      providesTags: [{ type: 'HelmRelease', id: 'LIST' }],
    }),
    installRelease: builder.mutation<{ output: string }, InstallReleaseParams>({
      query: ({ clusterCode, body }) => ({
        url: `/clusters/${enc(clusterCode)}/helm/releases`,
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
        url: `/clusters/${enc(clusterCode)}/helm/releases/${enc(namespace)}/${enc(name)}`,
        method: 'DELETE',
      }),
      invalidatesTags: [{ type: 'HelmRelease', id: 'LIST' }],
    }),
    rollbackRelease: builder.mutation<
      { output: string },
      { clusterCode: string; namespace: string; name: string; revision?: number }
    >({
      query: ({ clusterCode, namespace, name, revision }) => ({
        url: `/clusters/${enc(clusterCode)}/helm/releases/${enc(namespace)}/${enc(name)}/rollback`,
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
        url: `/clusters/${enc(clusterCode)}/helm/releases/${enc(namespace)}/${enc(name)}/history`,
        method: 'GET',
      }),
    }),
    listRepos: builder.query<{ items: HelmRepo[]; total: number }, void>({
      query: () => ({
        url: '/helm/repos',
        method: 'GET',
      }),
      providesTags: [{ type: 'HelmRepo', id: 'LIST' }],
    }),
    addRepo: builder.mutation<{ ok: boolean }, AddRepoBody>({
      query: (body) => ({
        url: '/helm/repos',
        method: 'POST',
        data: body,
      }),
      invalidatesTags: [
        { type: 'HelmRepo', id: 'LIST' },
        { type: 'HelmChart', id: 'LIST' },
      ],
    }),
    removeRepo: builder.mutation<{ ok: boolean }, string>({
      query: (name) => ({
        url: `/helm/repos/${enc(name)}`,
        method: 'DELETE',
      }),
      invalidatesTags: [
        { type: 'HelmRepo', id: 'LIST' },
        { type: 'HelmChart', id: 'LIST' },
      ],
    }),
    updateRepos: builder.mutation<{ ok: boolean }, { name?: string } | void>({
      query: (arg) => ({
        url: '/helm/repos/update',
        method: 'POST',
        params: arg && arg.name ? { name: arg.name } : undefined,
      }),
      invalidatesTags: [
        { type: 'HelmRepo', id: 'LIST' },
        { type: 'HelmChart', id: 'LIST' },
      ],
    }),
    searchCharts: builder.query<
      { items: HelmChart[]; total: number },
      { repo?: string; keyword?: string }
    >({
      query: ({ repo, keyword }) => ({
        url: '/helm/charts',
        method: 'GET',
        params: {
          repo: repo || undefined,
          keyword: keyword || undefined,
        },
      }),
      providesTags: [{ type: 'HelmChart', id: 'LIST' }],
    }),
  }),
});

export const {
  useListReleasesQuery,
  useInstallReleaseMutation,
  useUninstallReleaseMutation,
  useRollbackReleaseMutation,
  useListHistoryQuery,
  useListReposQuery,
  useAddRepoMutation,
  useRemoveRepoMutation,
  useUpdateReposMutation,
  useSearchChartsQuery,
} = helmApi;
