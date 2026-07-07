import { useEffect, useState, useCallback } from 'react';
import { Table, Button, Typography, message, Popconfirm, Tag, Space } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { usersApi } from '../api/client';

const { Title } = Typography;

interface User {
  id: string;
  username: string;
  email: string;
  full_name: string;
  role: string;
  is_active: boolean;
  created_at: string;
}

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await usersApi.list({ limit: 200 });
      setUsers(data);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDelete = async (id: string) => {
    await usersApi.delete(id);
    message.success('Пользователь удалён');
    load();
  };

  const columns = [
    { title: 'Username', dataIndex: 'username', key: 'username' },
    { title: 'Email', dataIndex: 'email', key: 'email' },
    { title: 'Имя', dataIndex: 'full_name', key: 'full_name' },
    {
      title: 'Роль', dataIndex: 'role', key: 'role',
      render: (r: string) => <Tag color={r === 'admin' ? 'gold' : 'blue'}>{r}</Tag>,
    },
    {
      title: 'Активен', dataIndex: 'is_active', key: 'is_active',
      render: (v: boolean) => <Tag color={v ? 'green' : 'red'}>{v ? 'Да' : 'Нет'}</Tag>,
    },
    {
      title: 'Создан', dataIndex: 'created_at', key: 'created_at',
      render: (v: string) => new Date(v).toLocaleDateString(),
    },
    {
      title: '', key: 'act',
      render: (_: unknown, rec: User) => (
        <Popconfirm title="Удалить пользователя?" onConfirm={() => handleDelete(rec.id)}>
          <Button size="small" danger>Удалить</Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>Пользователи</Title>
        <Button icon={<ReloadOutlined />} onClick={load}>Обновить</Button>
      </div>
      <Table columns={columns} dataSource={users} rowKey="id" loading={loading} pagination={{ pageSize: 20 }} />
    </div>
  );
}