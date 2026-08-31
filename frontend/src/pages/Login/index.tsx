import React from 'react';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { LoginForm, ProFormText } from '@ant-design/pro-components';
import { message } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAppDispatch } from '@/app/store';
import { login, fetchCurrentUser } from '@/slices/userSlice';
import { setDispatch } from '@/app/services/request';
import { safeRedirectPath } from '@/app/routePerms';

type LoginParams = {
  username: string;
  password: string;
  autoLogin?: boolean;
};

const Login: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const [searchParams] = useSearchParams();
  const redirect = safeRedirectPath(searchParams.get('redirect'));

  React.useEffect(() => {
    setDispatch(dispatch);
  }, [dispatch]);

  const handleSubmit = async (values: LoginParams) => {
    try {
      await dispatch(
        login({
          username: values.username,
          password: values.password,
        }),
      ).unwrap();
      // 登录响应仅含 UserBrief（无 perms），跳转前先拉取当前用户与权限点，
      // 保证进入首页后菜单/按钮权限立即可用；失败不阻塞跳转（BasicLayout 会兜底重拉）
      try {
        await dispatch(fetchCurrentUser()).unwrap();
      } catch {
        message.warning('已登录，但权限信息加载失败，进入后可重试');
      }
      message.success('登录成功');
      navigate(redirect, { replace: true });
    } catch {
      // 错误已由 request.ts 拦截器统一 toast（含登录 401 的账号密码错误提示）
    }
  };

  return (
    <div
      style={{
        backgroundColor: 'white',
        height: '100vh',
        width: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundImage:
          'radial-gradient(circle at 20% 50%, rgba(22, 119, 255, 0.08), transparent 50%), radial-gradient(circle at 80% 50%, rgba(22, 119, 255, 0.08), transparent 50%)',
      }}
    >
      <LoginForm
        title="管理控制台"
        subTitle="统一管理平台"
        initialValues={{ autoLogin: true }}
        onFinish={handleSubmit}
        actions={
          <div
            style={{
              textAlign: 'center',
              color: 'rgba(0, 0, 0, 0.45)',
              fontSize: 12,
            }}
          >
            版权所有 © 2026
          </div>
        }
      >
        <ProFormText
          name="username"
          fieldProps={{
            size: 'large',
            prefix: <UserOutlined className={'prefixIcon'} />,
          }}
          placeholder="用户名"
          rules={[{ required: true, message: '请输入用户名!' }]}
        />
        <ProFormText.Password
          name="password"
          fieldProps={{
            size: 'large',
            prefix: <LockOutlined className={'prefixIcon'} />,
          }}
          placeholder="密码"
          rules={[{ required: true, message: '请输入密码!' }]}
        />
        <div
          style={{
            marginBlockEnd: 24,
          }}
        />
      </LoginForm>
    </div>
  );
};

export default Login;
