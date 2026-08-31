import React from 'react';
import ReactDOM from 'react-dom/client';
import { Provider } from 'react-redux';
import { RouterProvider } from 'react-router-dom';
import { ConfigProvider, App as AntdApp } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import 'dayjs/locale/zh-cn';
import dayjs from 'dayjs';

import { store } from '@/app/store';
import { router } from '@/app/router';
import { setDispatch } from '@/app/services/request';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import './index.scss';

dayjs.locale('zh-cn');

setDispatch(store.dispatch);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <Provider store={store}>
      <ConfigProvider
        locale={zhCN}
        theme={{
          token: {
            colorPrimary: '#1677ff',
          },
        }}
      >
        <AntdApp>
          <ErrorBoundary>
            <RouterProvider router={router} />
          </ErrorBoundary>
        </AntdApp>
      </ConfigProvider>
    </Provider>
  </React.StrictMode>,
);
