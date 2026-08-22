import React from 'react';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { LoginForm, ProFormText } from '@ant-design/pro-components';
import { message } from 'antd';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAppDispatch } from '@/app/store';
import { login } from '@/slices/userSlice';
import { setDispatch } from '@/app/services/request';

type LoginParams = {
  username: string;
  password: string;
  autoLogin?: boolean;
};

const Login: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const [searchParams] = useSearchParams();
  const redirect = searchParams.get('redirect') || '/';

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
      message.success('登录成功');
      setTimeout(() => {
        navigate(redirect, { replace: true });
      }, 300);
    } catch (e) {
      // error already handled in request interceptor
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
