import { useEffect, useState, useCallback } from 'react';
import {
  Table, Button, Space, Tag, Typography, Modal, Form, Input, message, Popconfirm, Tooltip,
  Switch, InputNumber,
} from 'antd';
import { PlusOutlined, ReloadOutlined, PlayCircleOutlined, EditOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { camerasApi } from '../api/client';

const { Title } = Typography;

interface Camera {
  id: string;
  name: string;
  description: string;
  rtsp_main_url: string;
  rtsp_sub_url?: string;
  webrtc_url?: string;
  openipc_user: string;
  openipc_password: string;
  latitude?: number;
  longitude?: number;
  location: string;
  manufacturer: string;
  model: string;
  firmware: string;
  is_online: boolean;
  is_recording: boolean;
  is_enabled: boolean;
}

export default function CamerasPage() {
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingCamera, setEditingCamera] = useState<Camera | null>(null);
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

  const openAdd = () => {
    setEditingCamera(null);
    form.resetFields();
    form.setFieldsValue({ manufacturer: 'OpenIPC', openipc_user: 'root', openipc_password: '12345' });
    setModalOpen(true);
  };

  const openEdit = (cam: Camera) => {
    setEditingCamera(cam);
    form.setFieldsValue({
      name: cam.name,
      description: cam.description || '',
      rtsp_main_url: cam.rtsp_main_url,
      rtsp_sub_url: cam.rtsp_sub_url || '',
      webrtc_url: cam.webrtc_url || '',
      openipc_user: cam.openipc_user || 'root',
      openipc_password: cam.openipc_password || '12345',
      latitude: cam.latitude,
      longitude: cam.longitude,
      location: cam.location || '',
      manufacturer: cam.manufacturer || 'OpenIPC',
      model: cam.model || '',
      firmware: cam.firmware || '',
      is_enabled: cam.is_enabled,
    });
    setModalOpen(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      setSubmitting(true);
      if (editingCamera) {
        await camerasApi.update(editingCamera.id, values);
        message.success('Камера обновлена');
      } else {
        await camerasApi.create(values);
        message.success('Камера добавлена');
      }
      setModalOpen(false);
      form.resetFields();
      setEditingCamera(null);
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
  };  const columns = [
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
      dataIndex: 'rtsp_main_url',
      key: 'rtsp_main_url',
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
          <Tooltip title="Редактировать">
            <Button
              size="small"
              icon={<EditOutlined />}
              onClick={() => openEdit(rec)}
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
          <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
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
        title={editingCamera ? 'Редактировать камеру' : 'Добавить камеру'}
        open={modalOpen}
        onCancel={() => { setModalOpen(false); form.resetFields(); setEditingCamera(null); }}
        onOk={handleSubmit}
        confirmLoading={submitting}
        okText={editingCamera ? 'Сохранить' : 'Добавить'}
        cancelText="Отмена"
        width={640}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="Название" rules={[{ required: true }]}>
            <Input placeholder="Камера 1" />
          </Form.Item>
          <Form.Item name="description" label="Описание">
            <Input.TextArea rows={2} placeholder="Опционально" />
          </Form.Item>

          <Form.Item name="location" label="Локация">
            <Input placeholder="Этаж 1, Коридор" />
          </Form.Item>

          {/* ---- RTSP потоки ---- */}
          <Form.Item name="rtsp_main_url" label="RTSP основной поток (H.264, высокое качество)" rules={[{ required: true }]}>
            <Input placeholder="rtsp://root:12345@192.168.1.x/stream=0" />
          </Form.Item>
          <Form.Item name="rtsp_sub_url" label="RTSP дополнительный поток (низкое качество, опционально)"
            tooltip="Используется в сетке LiveView для снижения нагрузки">
            <Input placeholder="rtsp://root:12345@192.168.1.x/stream=1" />
          </Form.Item>

          {/* ---- WebRTC ---- */}
          <Form.Item name="webrtc_url" label="WebRTC URL (опционально)">
            <Input placeholder="http://192.168.1.100:1984/api/webrtc" />
          </Form.Item>

          {/* ---- Аудио/Авторизация ---- */}
          <Form.Item name="openipc_user" label="Пользователь камеры (OpenIPC)" initialValue="root">
            <Input placeholder="root" />
          </Form.Item>
          <Form.Item name="openipc_password" label="Пароль камеры" initialValue="12345">
            <Input.Password placeholder="12345" />
          </Form.Item>

          {/* ---- Производитель/Модель ---- */}
          <Form.Item name="manufacturer" label="Производитель" initialValue="OpenIPC">
            <Input />
          </Form.Item>
          <Form.Item name="model" label="Модель">
            <Input placeholder="IPC-xxx" />
          </Form.Item>
          <Form.Item name="firmware" label="Прошивка">
            <Input placeholder="Версия" />
          </Form.Item>

          {/* ---- Геолокация ---- */}
          <Form.Item name="latitude" label="Широта">
            <InputNumber style={{ width: '100%' }} placeholder="55.7558" />
          </Form.Item>
          <Form.Item name="longitude" label="Долгота">
            <InputNumber style={{ width: '100%' }} placeholder="37.6173" />
          </Form.Item>

          {/* ---- Включена ---- */}
          <Form.Item name="is_enabled" label="Камера включена" valuePropName="checked" initialValue={true}>
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}