import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import request from '@/app/services/request';
import { AppDispatch } from '@/app/store';

export interface CurrentUser {
  id?: string;
  username?: string;
  nickname?: string;
  avatar?: string;
  email?: string;
}

export interface UserState {
  token: string | null;
  currentUser: CurrentUser | null;
}

const initialState: UserState = {
  token: localStorage.getItem('token'),
  currentUser: null,
};

interface LoginParams {
  username: string;
  password: string;
}

interface LoginResponse {
  token: string;
  user: CurrentUser;
}

export const login = createAsyncThunk<
  LoginResponse,
  LoginParams,
  { dispatch: AppDispatch }
>('user/login', async (params) => {
  const res = await request.post<LoginResponse>('/auth/login', params);
  return res.data;
});

export const fetchCurrentUser = createAsyncThunk<
  CurrentUser,
  void,
  { dispatch: AppDispatch }
>('user/fetchCurrentUser', async () => {
  const res = await request.get<CurrentUser>('/users/me');
  return res.data;
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
        state.currentUser = action.payload.user;
        localStorage.setItem('token', action.payload.token);
      })
      .addCase(fetchCurrentUser.fulfilled, (state, action) => {
        state.currentUser = action.payload;
      });
  },
});

export const { setToken, setCurrentUser, logout } = userSlice.actions;
export default userSlice.reducer;
