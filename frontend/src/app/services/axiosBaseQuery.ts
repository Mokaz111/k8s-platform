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

export const axiosBaseQuery =
  (): BaseQueryFn<AxiosBaseQueryArgs, unknown, BaseQueryError> =>
  async ({ url, method = 'GET', data, params, headers, responseType }) => {
    try {
      const result = await request({
        url,
        method,
        data,
        params,
        headers,
        responseType,
      });
      return { data: result.data };
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
