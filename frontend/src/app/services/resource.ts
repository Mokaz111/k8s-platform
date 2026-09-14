import { createApi } from '@reduxjs/toolkit/query/react';
import { axiosBaseQuery } from './axiosBaseQuery';

export interface KubernetesResource {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    uid?: string;
    resourceVersion?: string;
    creationTimestamp?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    [key: string]: unknown;
  };
  spec?: Record<string, unknown>;
  status?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface ListResourcesParams {
  code: string;
  apiVersion: string;
  kind: string;
  namespace?: string;
  page?: number;
  size?: number;
  keyword?: string;
}

export interface ListResourcesResponse {
  items: KubernetesResource[];
  total: number;
  page: number;
  size: number;
}

export interface GetResourceParams {
  code: string;
  apiVersion: string;
  kind: string;
  namespace?: string;
  name: string;
}

export interface CreateResourceParams {
  code: string;
  apiVersion: string;
  kind: string;
  yaml: string;
}

export interface UpdateResourceParams extends GetResourceParams {
  yaml: string;
}

// apiVersion 含斜杠（如 apps/v1、networking.k8s.io/v1），后端开启 UseRawPath 按原始路径匹配
// 单段参数，因此前端必须 encodeURIComponent 一次（apps%2Fv1），否则会被当成多个路径段导致 404
const enc = (s: string): string => encodeURIComponent(s);

export const resourceApi = createApi({
  reducerPath: 'resourceApi',
  baseQuery: axiosBaseQuery(),
  tagTypes: ['Resource'],
  endpoints: (builder) => ({
    listResources: builder.query<ListResourcesResponse, ListResourcesParams>({
      query: ({ code, apiVersion, kind, namespace, page, size, keyword }) => ({
        url: `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}`,
        method: 'GET',
        params: {
          namespace,
          page,
          size,
          keyword,
        } as Record<string, unknown>,
      }),
      providesTags: [{ type: 'Resource', id: 'LIST' }],
    }),
    getResource: builder.query<KubernetesResource, GetResourceParams>({
      query: ({ code, apiVersion, kind, namespace, name }) => ({
        url: namespace
          ? `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}/${enc(namespace)}/${enc(name)}`
          : `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}/${enc(name)}`,
        method: 'GET',
      }),
      providesTags: (_result, _err, params) => [
        { type: 'Resource', id: `${params.code}-${params.apiVersion}-${params.kind}-${params.namespace || ''}-${params.name}` },
      ],
    }),
    createResource: builder.mutation<KubernetesResource, CreateResourceParams>({
      query: ({ code, apiVersion, kind, yaml }) => ({
        url: `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}`,
        method: 'POST',
        data: { yaml },
      }),
      invalidatesTags: [{ type: 'Resource', id: 'LIST' }],
    }),
    updateResource: builder.mutation<KubernetesResource, UpdateResourceParams>({
      query: ({ code, apiVersion, kind, namespace, name, yaml }) => ({
        url: namespace
          ? `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}/${enc(namespace)}/${enc(name)}`
          : `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}/${enc(name)}`,
        method: 'PUT',
        data: { yaml },
      }),
      invalidatesTags: (_result, _err, params) => [
        { type: 'Resource', id: 'LIST' },
        { type: 'Resource', id: `${params.code}-${params.apiVersion}-${params.kind}-${params.namespace || ''}-${params.name}` },
      ],
    }),
    deleteResource: builder.mutation<void, GetResourceParams>({
      query: ({ code, apiVersion, kind, namespace, name }) => ({
        url: namespace
          ? `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}/${enc(namespace)}/${enc(name)}`
          : `/clusters/${enc(code)}/resources/${enc(apiVersion)}/${enc(kind)}/${enc(name)}`,
        method: 'DELETE',
      }),
      invalidatesTags: (_result, _err, params) => [
        { type: 'Resource', id: 'LIST' },
        { type: 'Resource', id: `${params.code}-${params.apiVersion}-${params.kind}-${params.namespace || ''}-${params.name}` },
      ],
    }),
    listNamespaces: builder.query<string[], string>({
      query: (code) => ({
        url: `/clusters/${enc(code)}/resources/v1/Namespace`,
        method: 'GET',
      }),
      transformResponse: (response: ListResourcesResponse | { items: KubernetesResource[] }) => {
        const items = 'items' in response ? response.items : [];
        return items.map((ns) => ns.metadata.name);
      },
    }),
  }),
});

export const {
  useListResourcesQuery,
  useGetResourceQuery,
  useLazyGetResourceQuery,
  useCreateResourceMutation,
  useUpdateResourceMutation,
  useDeleteResourceMutation,
  useListNamespacesQuery,
} = resourceApi;
