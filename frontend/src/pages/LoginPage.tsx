import { useEffect } from 'react';
import { Card, Form, Input, Button, Typography, Alert, App } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { useDispatch, useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { login, clearError } from '../store/authSlice';
import type { RootState, AppDispatch } from '../store';

const { Title, Text } = Typography;

export default function LoginPage() {
  const dispatch = useDispatch<AppDispatch>();
  const navigate = useNavigate();
  const { loading, error, token } = useSelector((s: RootState) => s.auth);

  useEffect(() => {
    if (token) navigate('/');
  }, [token, navigate]);

  const onFinish = (values: { username: string; password: string }) => {
    dispatch(login(values));
  };

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'linear-gradient(135deg, #1677ff 0%, #0958d9 100%)',
    }}>
      <Card style={{ width: 400, borderRadius: 12 }} bordered={false}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title level={2} style={{ marginBottom: 4 }}>NRV</Title>
          <Text type="secondary">Network Video Recorder</Text>
        </div>

        {error && (
          <Alert
            message={error}
            type="error"
            closable
            style={{ marginBottom: 16 }}
            onClose={() => dispatch(clearError())}
          />
        )}

        <Form name="login" onFinish={onFinish} size="large" layout="vertical">
          <Form.Item name="username" rules={[{ required: true, message: 'Введите имя пользователя' }]}>
            <Input prefix={<UserOutlined />} placeholder="Имя пользователя" autoFocus />
          </Form.Item>

          <Form.Item name="password" rules={[{ required: true, message: 'Введите пароль' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="Пароль" />
          </Form.Item>

          <Form.Item>
            <Button type="primary" htmlType="submit" loading={loading} block>
              Войти
            </Button>
          </Form.Item>
        </Form>

        <Text type="secondary" style={{ display: 'block', textAlign: 'center', fontSize: 12 }}>
          Версия 0.1.0 • OpenIPC NVR
        </Text>
      </Card>
    </div>
  );
}