import { configureStore } from '@reduxjs/toolkit';
import { TypedUseSelectorHook, useDispatch, useSelector } from 'react-redux';
import userReducer from '@/slices/userSlice';
import appReducer from '@/slices/appSlice';
import wsReducer from '@/slices/wsSlice';
import { clusterApi } from '@/app/services/cluster';
import { resourceApi } from '@/app/services/resource';
import { versionApi } from '@/app/services/version';
import { backupApi } from '@/app/services/backup';

export const store = configureStore({
  reducer: {
    user: userReducer,
    app: appReducer,
    ws: wsReducer,
    [clusterApi.reducerPath]: clusterApi.reducer,
    [resourceApi.reducerPath]: resourceApi.reducer,
    [versionApi.reducerPath]: versionApi.reducer,
    [backupApi.reducerPath]: backupApi.reducer,
  },
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware()
      .concat(clusterApi.middleware)
      .concat(resourceApi.middleware)
      .concat(versionApi.middleware)
      .concat(backupApi.middleware),
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

export const useAppDispatch: () => AppDispatch = useDispatch;
export const useAppSelector: TypedUseSelectorHook<RootState> = useSelector;
