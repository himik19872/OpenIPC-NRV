import { Card, Typography, Descriptions, Tag } from 'antd';
import { useSelector } from 'react-redux';
import type { RootState } from '../store';

const { Title } = Typography;

export default function SettingsPage() {
  const user = useSelector((s: RootState) => s.auth.user);

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>Настройки</Title>

      <Card title="Система" bordered={false} style={{ marginBottom: 16 }}>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="Версия">NRV 0.1.0</Descriptions.Item>
          <Descriptions.Item label="Среда">development</Descriptions.Item>
          <Descriptions.Item label="API">/api — FastAPI</Descriptions.Item>
          <Descriptions.Item label="БД">PostgreSQL (asyncpg)</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="Мой профиль" bordered={false}>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="Username">{user?.username}</Descriptions.Item>
          <Descriptions.Item label="Email">{user?.email}</Descriptions.Item>
          <Descriptions.Item label="Имя">{user?.full_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="Роль">
            <Tag color={user?.role === 'admin' ? 'gold' : 'blue'}>{user?.role}</Tag>
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}