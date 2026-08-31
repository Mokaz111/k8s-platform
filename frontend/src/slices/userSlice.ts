import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import request from '@/app/services/request';
import { AppDispatch } from '@/app/store';

export type ScopeType = 'platform' | 'cluster' | 'namespace';

export type ProfileStatus = 'idle' | 'loading' | 'ready' | 'failed';

// 当前用户的角色绑定信息（含数据范围）
export interface CurrentUserRole {
  role_id: number;
  role_code: string;
  role_name: string;
  scope_type: ScopeType;
  cluster_code?: string;
  namespace?: string;
}

export interface CurrentUser {
  id?: number | string;
  username?: string;
  display_name?: string;
  nickname?: string; // 兼容旧字段
  avatar?: string;
  email?: string;
  phone?: string;
  auth_source?: string;
  status?: number;
  last_login_at?: string;
  last_login_ip?: string;
  roles?: CurrentUserRole[];
  perms?: string[]; // 权限点 code 列表（如 cluster:create）
  is_platform_admin?: boolean;
}

export interface UserState {
  token: string | null;
  refreshToken: string | null;
  currentUser: CurrentUser | null;
  profileStatus: ProfileStatus;
}

// 防御历史 bug 写入的脏值 "undefined"（旧版本曾以错误字段名存入 undefined）
const readStoredToken = (key: string): string | null => {
  const v = localStorage.getItem(key);
  return v && v !== 'undefined' && v !== 'null' ? v : null;
};

const initialState: UserState = {
  token: readStoredToken('token'),
  refreshToken: readStoredToken('refresh_token'),
  currentUser: null,
  profileStatus: 'idle',
};

// 后端统一响应 envelope（request.ts 不做解包，这里手动取 data 字段）
interface ApiEnvelope<T> {
  code: number;
  message: string;
  data: T;
  trace_id?: string;
}

interface LoginParams {
  username: string;
  password: string;
}

// 与后端 auth.LoginResult 对齐：access_token / refresh_token / token_type / expires_in / user
interface LoginResponse {
  access_token: string;
  refresh_token?: string;
  token_type?: string;
  expires_in?: number;
  user?: CurrentUser;
}

export const login = createAsyncThunk<
  LoginResponse,
  LoginParams,
  { dispatch: AppDispatch }
>('user/login', async (params) => {
  const res = await request.post<ApiEnvelope<LoginResponse>>('/auth/login', params);
  return res.data.data;
});

// fetchCurrentUser 调 /auth/me（CurrentUserHandler 返回 user + roles + perms）
export const fetchCurrentUser = createAsyncThunk<
  CurrentUser,
  void,
  { dispatch: AppDispatch }
>('user/fetchCurrentUser', async () => {
  const res = await request.get<ApiEnvelope<CurrentUser>>('/auth/me');
  return res.data.data;
});

const persistToken = (key: string, value: string | null) => {
  if (value) {
    localStorage.setItem(key, value);
  } else {
    localStorage.removeItem(key);
  }
};

const userSlice = createSlice({
  name: 'user',
  initialState,
  reducers: {
    setToken(state, action: PayloadAction<string | null>) {
      state.token = action.payload;
      persistToken('token', action.payload);
    },
    setRefreshToken(state, action: PayloadAction<string | null>) {
      state.refreshToken = action.payload;
      persistToken('refresh_token', action.payload);
    },
    setCurrentUser(state, action: PayloadAction<CurrentUser | null>) {
      state.currentUser = action.payload;
    },
    logout(state) {
      state.token = null;
      state.refreshToken = null;
      state.currentUser = null;
      state.profileStatus = 'idle';
      localStorage.removeItem('token');
      localStorage.removeItem('refresh_token');
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.fulfilled, (state, action) => {
        state.token = action.payload.access_token;
        state.refreshToken = action.payload.refresh_token ?? null;
        persistToken('token', state.token);
        persistToken('refresh_token', state.refreshToken);
        // 登录响应仅含 UserBrief（无 perms），完整信息由 fetchCurrentUser 补全
        state.profileStatus = 'loading';
      })
      .addCase(fetchCurrentUser.pending, (state) => {
        state.profileStatus = 'loading';
      })
      .addCase(fetchCurrentUser.fulfilled, (state, action) => {
        state.currentUser = action.payload;
        state.profileStatus = 'ready';
      })
      .addCase(fetchCurrentUser.rejected, (state) => {
        state.profileStatus = 'failed';
      });
  },
});

export const { setToken, setRefreshToken, setCurrentUser, logout } = userSlice.actions;
export default userSlice.reducer;
