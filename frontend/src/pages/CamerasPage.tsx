import { useEffect, useState, useCallback } from 'react';
import {
  Table, Button, Space, Tag, Typography, Modal, Form, Input, message, Popconfirm, Tooltip,
} from 'antd';
import { PlusOutlined, ReloadOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { camerasApi } from '../api/client';

const { Title } = Typography;

interface Camera {
  id: string;
  name: string;
  rtsp_url: string;
  is_online: boolean;
  is_recording: boolean;
  is_enabled: boolean;
  location: string;
  manufacturer: string;
}

export default function CamerasPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();
  const navigate = useNavigate();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await camerasApi.list({ limit: 200 });
      setCameras(data);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleAdd = async () => {
    try {
      const values = await form.validateFields();
      setSubmitting(true);
      await camerasApi.create(values);
      message.success('Камера добавлена');
      setModalOpen(false);
      form.resetFields();
      load();
    } catch {
      // validation error
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    await camerasApi.delete(id);
    message.success('Камера удалена');
    load();
  };

  const columns = [
    {
      title: 'Название',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, rec: Camera) => (
        <a onClick={() => navigate(`/cameras/${rec.id}`)}>{name}</a>
      ),
    },
    {
      title: 'RTSP',
      dataIndex: 'rtsp_url',
      key: 'rtsp_url',
      ellipsis: true,
      render: (url: string) => (
        <Tooltip title={url}>
          <span style={{ fontSize: 12, color: '#888' }}>{url}</span>
        </Tooltip>
      ),
    },
    {
      title: 'Производитель',
      dataIndex: 'manufacturer',
      key: 'manufacturer',
      width: 120,
    },
    {
      title: 'Локация',
      dataIndex: 'location',
      key: 'location',
      width: 140,
    },
    {
      title: 'Статус',
      key: 'status',
      width: 160,
      render: (_: unknown, rec: Camera) => (
        <Space>
          <Tag color={rec.is_online ? 'green' : 'red'}>
            {rec.is_online ? 'Онлайн' : 'Офлайн'}
          </Tag>
          {rec.is_recording && <Tag color="blue">Запись</Tag>}
          {!rec.is_enabled && <Tag>Отключена</Tag>}
        </Space>
      ),
    },
    {
      title: 'Действия',
      key: 'actions',
      width: 120,
      render: (_: unknown, rec: Camera) => (
        <Space>
          <Tooltip title="Смотреть">
            <Button
              size="small"
              icon={<PlayCircleOutlined />}
              onClick={() => navigate(`/cameras/${rec.id}`)}
            />
          </Tooltip>
          <Popconfirm
            title="Удалить камеру?"
            onConfirm={() => handleDelete(rec.id)}
          >
            <Button size="small" danger>Удалить</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Title level={4} style={{ margin: 0 }}>Камеры</Title>
        <Space>
          <Button icon={<ReloadOutlined />} onClick={load}>Обновить</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setModalOpen(true)}>
            Добавить
          </Button>
        </Space>
      </div>

      <Table
        columns={columns}
        dataSource={cameras}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 20 }}
      />

      <Modal
        title="Добавить камеру"
        open={modalOpen}
        onCancel={() => { setModalOpen(false); form.resetFields(); }}
        onOk={handleAdd}
        confirmLoading={submitting}
        okText="Добавить"
        cancelText="Отмена"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="Название" rules={[{ required: true }]}>
            <Input placeholder="Камера 1" />
          </Form.Item>
          <Form.Item name="rtsp_url" label="RTSP URL" rules={[{ required: true }]}>
            <Input placeholder="rtsp://192.168.1.100:554/stream=0" />
          </Form.Item>
          <Form.Item name="webrtc_url" label="WebRTC URL">
            <Input placeholder="http://192.168.1.100:1984/api/webrtc" />
          </Form.Item>
          <Form.Item name="location" label="Локация">
            <Input placeholder="Этаж 1, Коридор" />
          </Form.Item>
          <Form.Item name="manufacturer" label="Производитель" initialValue="OpenIPC">
            <Input />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}