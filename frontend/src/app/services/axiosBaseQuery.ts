import { BaseQueryFn } from '@reduxjs/toolkit/query';
import { AxiosError, AxiosRequestConfig } from 'axios';
import request from './request';

export interface AxiosBaseQueryArgs {
  url: string;
  method?: AxiosRequestConfig['method'];
  data?: unknown;
  params?: Record<string, unknown>;
  headers?: Record<string, string>;
  responseType?: AxiosRequestConfig['responseType'];
}

export interface BaseQueryError {
  status?: number;
  data?: unknown;
  message: string;
}

// 后端统一响应 envelope：{ code, message, data, trace_id }
// code === 0 表示成功；axiosBaseQuery 会自动解包 data 字段返回给 RTK Query
// 这样各 service 声明的返回类型即为业务 payload（如 { total, items } 或单对象）
interface ApiEnvelope<T = unknown> {
  code: number;
  message: string;
  data: T;
  trace_id?: string;
}

export const axiosBaseQuery =
  (): BaseQueryFn<AxiosBaseQueryArgs, unknown, BaseQueryError> =>
  async ({ url, method = 'GET', data, params, headers, responseType }, api) => {
    try {
      const result = await request({
        url,
        method,
        data,
        params,
        headers,
        responseType,
        signal: api.signal,
      });
      const body = result.data;
      // 解包统一响应 envelope：成功时返回 data 字段，失败时转为 error
      if (body && typeof body === 'object' && 'code' in (body as Record<string, unknown>)) {
        const env = body as ApiEnvelope;
        if (env.code === 0) {
          return { data: env.data };
        }
        return {
          error: {
            status: result.status,
            data: env,
            message: env.message || 'Request failed',
          },
        };
      }
      // 非 envelope（如原始 blob / 纯文本）直接返回
      return { data: body };
    } catch (axiosError) {
      const err = axiosError as AxiosError<{ message?: string; error?: string; msg?: string }>;
      return {
        error: {
          status: err.response?.status,
          data: err.response?.data,
          message:
            err.response?.data?.message ||
            err.response?.data?.msg ||
            err.response?.data?.error ||
            err.message ||
            'Request failed',
        },
      };
    }
  };
