import axios, { AxiosError, AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { message } from 'antd';
import type { AppDispatch } from '@/app/store';
import { logout, setRefreshToken, setToken } from '@/slices/userSlice';
import { setSelectedClusterCode } from '@/slices/appSlice';

let _dispatch: AppDispatch | null = null;

export const setDispatch = (dispatch: AppDispatch) => {
  _dispatch = dispatch;
};

const getToken = (): string | null => {
  return localStorage.getItem('token');
};

interface ErrorResponseData {
  message?: string;
  msg?: string;
  error?: string;
  trace_id?: string;
}

const isCanceled = (error: AxiosError) =>
  error.code === 'ERR_CANCELED' || error.name === 'CanceledError' || error.message === 'canceled';

const handleUnauthorized = () => {
  localStorage.removeItem('token');
  localStorage.removeItem('refresh_token');
  if (_dispatch) {
    _dispatch(logout());
    _dispatch(setSelectedClusterCode(null));
  }
  if (window.location.pathname !== '/login') {
    window.location.href = `/login?redirect=${encodeURIComponent(window.location.pathname + window.location.search)}`;
  }
};

const isAuthPath = (url = '') =>
  url.includes('/auth/login') || url.includes('/auth/refresh') || url.includes('/auth/logout');

let refreshInFlight: Promise<string | null> | null = null;

async function refreshAccessToken(): Promise<string | null> {
  const rt = localStorage.getItem('refresh_token');
  if (!rt || rt === 'undefined' || rt === 'null') {
    return null;
  }
  const base = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1';
  const res = await axios.post(`${base}/auth/refresh`, { refresh_token: rt });
  const body = res.data as { code?: number; data?: { access_token?: string; refresh_token?: string } };
  const data = body?.data;
  if (!data?.access_token) {
    return null;
  }
  localStorage.setItem('token', data.access_token);
  if (data.refresh_token) {
    localStorage.setItem('refresh_token', data.refresh_token);
  }
  if (_dispatch) {
    _dispatch(setToken(data.access_token));
    if (data.refresh_token) {
      _dispatch(setRefreshToken(data.refresh_token));
    }
  }
  return data.access_token;
}

const handleError = (error: AxiosError<ErrorResponseData>) => {
  if (isCanceled(error)) {
    return;
  }
  const response = error.response;
  const status = response?.status;
  const data = response?.data;

  if (status === 401) {
    // 登录/刷新令牌接口本身的 401 是「账号密码错误 / 账号禁用 / refresh_token 无效」，
    // 属于业务失败：只提示，不触发登出跳转（否则用户输错密码毫无反馈）
    const url = error.config?.url || '';
    if (url.includes('/auth/login') || url.includes('/auth/refresh')) {
      message.error(data?.message || '用户名或密码错误');
      return;
    }
    handleUnauthorized();
    return;
  }

  if (error.config?.responseType === 'blob') {
    return;
  }

  const errorMessage =
    data?.message ||
    data?.msg ||
    data?.error ||
    (status === 403 ? '无权限访问' : status === 404 ? '资源不存在' : status === 500 ? '服务器内部错误' : '请求失败');

  message.error(errorMessage);

  if (data?.trace_id) {
    console.debug(`[trace_id=${data.trace_id}] ${errorMessage}`);
  }
};

const request: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api/v1',
  timeout: 30000,
});

request.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = getToken();
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }

    const traceId = (config.headers['X-Trace-Id'] as string) || window.crypto?.randomUUID?.();
    if (traceId) {
      config.headers['X-Trace-Id'] = traceId;
    }

    return config;
  },
  (error) => {
    return Promise.reject(error);
  },
);

request.interceptors.response.use(
  (response: AxiosResponse) => {
    return response;
  },
  async (error: AxiosError<ErrorResponseData>) => {
    const config = error.config as (InternalAxiosRequestConfig & { _retried?: boolean }) | undefined;
    const url = config?.url || '';
    if (error.response?.status === 401 && config && !config._retried && !isAuthPath(url)) {
      try {
        if (!refreshInFlight) {
          refreshInFlight = refreshAccessToken().finally(() => {
            refreshInFlight = null;
          });
        }
        const next = await refreshInFlight;
        if (next) {
          config._retried = true;
          config.headers.Authorization = `Bearer ${next}`;
          return request(config);
        }
      } catch {
        // 刷新失败走统一未授权处理
      }
    }
    handleError(error);
    return Promise.reject(error);
  },
);

export const download = async (
  url: string,
  filename: string,
  params?: Record<string, unknown>,
  method: 'GET' | 'POST' = 'GET',
) => {
  try {
    const response = await request({
      url,
      method,
      params: method === 'GET' ? params : undefined,
      data: method === 'POST' ? params : undefined,
      responseType: 'blob',
    });

    const contentType = String(response.headers['content-type'] || '');
    if (contentType.includes('application/json')) {
      const text = await new Blob([response.data]).text();
      try {
        const body = JSON.parse(text) as { message?: string };
        message.error(body.message || '下载失败');
      } catch {
        message.error('下载失败');
      }
      throw new Error('download rejected');
    }

    const blob = new Blob([response.data]);
    const link = document.createElement('a');
    const objectUrl = URL.createObjectURL(blob);
    link.href = objectUrl;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(objectUrl);
  } catch (error) {
    if (error instanceof AxiosError) {
      if (isCanceled(error)) {
        throw error;
      }
      if (error.response?.status === 401) {
        handleUnauthorized();
        throw error;
      }
      const data = error.response?.data;
      if (data instanceof Blob) {
        try {
          const body = JSON.parse(await data.text()) as { message?: string };
          message.error(body.message || '下载失败');
          throw error;
        } catch (inner) {
          if (inner instanceof AxiosError) {
            throw inner;
          }
        }
      }
    }
    message.error('下载失败');
    throw error;
  }
};

export default request;
