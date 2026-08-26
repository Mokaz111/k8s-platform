import axios, { AxiosError, AxiosInstance, AxiosResponse, InternalAxiosRequestConfig } from 'axios';
import { message } from 'antd';
import type { AppDispatch } from '@/app/store';
import { logout } from '@/slices/userSlice';

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

const handleUnauthorized = () => {
  localStorage.removeItem('token');
  if (_dispatch) {
    _dispatch(logout());
  }
  if (window.location.pathname !== '/login') {
    window.location.href = `/login?redirect=${encodeURIComponent(window.location.pathname + window.location.search)}`;
  }
};

const handleError = (error: AxiosError<ErrorResponseData>) => {
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
  (error: AxiosError<ErrorResponseData>) => {
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
    if (error instanceof AxiosError && error.response?.status === 401) {
      handleUnauthorized();
    } else {
      message.error('下载失败');
    }
    throw error;
  }
};

export default request;
