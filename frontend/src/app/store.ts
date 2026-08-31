import { configureStore, Middleware } from '@reduxjs/toolkit';
import { TypedUseSelectorHook, useDispatch, useSelector } from 'react-redux';
import userReducer, { logout } from '@/slices/userSlice';
import appReducer, { setSelectedClusterCode } from '@/slices/appSlice';
import wsReducer from '@/slices/wsSlice';
import { clusterApi } from '@/app/services/cluster';
import { resourceApi } from '@/app/services/resource';
import { versionApi } from '@/app/services/version';
import { backupApi } from '@/app/services/backup';
import { rbacApi } from '@/app/services/rbac';
import { helmApi } from '@/app/services/helm';

const allApis = [clusterApi, resourceApi, versionApi, backupApi, rbacApi, helmApi];

// 登出时清空全部 RTK Query 缓存：
// 否则换账号登录后，同参数 query 会直接命中上一个账号（权限范围内）的缓存数据
const resetApiCacheOnLogout: Middleware = (api) => (next) => (action) => {
  const result = next(action);
  if (logout.match(action)) {
    api.dispatch(setSelectedClusterCode(null));
    allApis.forEach((apiSlice) => api.dispatch(apiSlice.util.resetApiState()));
  }
  return result;
};

export const store = configureStore({
  reducer: {
    user: userReducer,
    app: appReducer,
    ws: wsReducer,
    [clusterApi.reducerPath]: clusterApi.reducer,
    [resourceApi.reducerPath]: resourceApi.reducer,
    [versionApi.reducerPath]: versionApi.reducer,
    [backupApi.reducerPath]: backupApi.reducer,
    [rbacApi.reducerPath]: rbacApi.reducer,
    [helmApi.reducerPath]: helmApi.reducer,
  },
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware()
      .concat(clusterApi.middleware)
      .concat(resourceApi.middleware)
      .concat(versionApi.middleware)
      .concat(backupApi.middleware)
      .concat(rbacApi.middleware)
      .concat(helmApi.middleware)
      .concat(resetApiCacheOnLogout),
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

export const useAppDispatch: () => AppDispatch = useDispatch;
export const useAppSelector: TypedUseSelectorHook<RootState> = useSelector;
