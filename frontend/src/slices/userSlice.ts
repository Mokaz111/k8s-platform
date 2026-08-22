import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import request from '@/app/services/request';
import { AppDispatch } from '@/app/store';

export type ScopeType = 'platform' | 'cluster' | 'namespace';

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
  currentUser: CurrentUser | null;
}

const initialState: UserState = {
  token: localStorage.getItem('token'),
  currentUser: null,
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

interface LoginResponse {
  token: string;
  refresh_token?: string;
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

const userSlice = createSlice({
  name: 'user',
  initialState,
  reducers: {
    setToken(state, action: PayloadAction<string | null>) {
      state.token = action.payload;
      if (action.payload) {
        localStorage.setItem('token', action.payload);
      } else {
        localStorage.removeItem('token');
      }
    },
    setCurrentUser(state, action: PayloadAction<CurrentUser | null>) {
      state.currentUser = action.payload;
    },
    logout(state) {
      state.token = null;
      state.currentUser = null;
      localStorage.removeItem('token');
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(login.fulfilled, (state, action) => {
        state.token = action.payload.token;
        localStorage.setItem('token', action.payload.token);
        // 后端登录响应可能不包含 user 详情，则保留 null，由 fetchCurrentUser 补全
        if (action.payload.user) {
          state.currentUser = action.payload.user;
        }
      })
      .addCase(fetchCurrentUser.fulfilled, (state, action) => {
        state.currentUser = action.payload;
      });
  },
});

export const { setToken, setCurrentUser, logout } = userSlice.actions;
export default userSlice.reducer;
