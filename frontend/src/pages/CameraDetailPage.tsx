import { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card, Row, Col, Typography, Tag, Descriptions, Button, Space,
  Table, message, Spin, Tabs, Image, Divider, Tooltip,
} from 'antd';
import {
  ArrowLeftOutlined, PlayCircleOutlined, StopOutlined,
  BulbOutlined, CameraOutlined, SunOutlined, SettingOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { camerasApi } from '../api/client';

const { Title, Text } = Typography;

interface CameraDetail {
  id: string;
  name: string;
  rtsp_url: string;
  webrtc_url: string | null;
  manufacturer: string;
  location: string;
  is_online: boolean;
  is_recording: boolean;
  is_enabled: boolean;
  created_at: string;
  snapshot_base64?: string;
  mjpeg_url?: string;
  hls_url?: string;
  ws_video_url?: string;
  majestic_config?: Record<string, unknown>;
}

interface Recording {
  id: string;
  start_time: string;
  end_time: string | null;
  status: string;
  duration: number;
  file_size: number;
}

export default function CameraDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [camera, setCamera] = useState<CameraDetail | null>(null);
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [loading, setLoading] = useState(true);
  const [recording, setRecording] = useState(false);
  const [nightLoading, setNightLoading] = useState(false);
  const [snapshotTs, setSnapshotTs] = useState(Date.now());

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const [camRes, recRes] = await Promise.all([
        camerasApi.openipc.detail(id),
        camerasApi.getRecordings(id, { limit: 50 }),
      ]);
      setCamera(camRes.data);
      setRecordings(recRes.data);
    } catch {
      // fallback to basic camera info
      try {
        const { data } = await camerasApi.get(id);
        setCamera(data);
        const rec = await camerasApi.getRecordings(id, { limit: 50 });
        setRecordings(rec.data);
      } catch {
        message.error('Не удалось загрузить камеру');
      }
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { load(); }, [load]);

  const refreshSnapshot = () => setSnapshotTs(Date.now());

  const handleStartRecording = async () => {
    if (!id) return;
    try {
      setRecording(true);
      await camerasApi.startRecording(id);
      message.success('Запись запущена');
      load();
    } catch {
      message.error('Не удалось запустить запись');
    } finally {
      setRecording(false);
    }
  };

  const handleStopRecording = async (recId: string) => {
    if (!id) return;
    try {
      await camerasApi.stopRecording(id, recId);
      message.success('Запись остановлена');
      load();
    } catch {
      message.error('Ошибка остановки записи');
    }
  };

  const handleNightToggle = async () => {
    if (!id) return;
    setNightLoading(true);
    try {
      await camerasApi.openipc.nightToggle(id);
      message.success('Ночной режим переключён');
    } catch {
      message.error('Не удалось переключить');
    } finally {
      setNightLoading(false);
    }
  };

  const handleIrcut = async () => {
    if (!id) return;
    try {
      await camerasApi.openipc.ircut(id);
      message.success('IR-фильтр переключён');
    } catch {
      message.error('Ошибка');
    }
  };

  const handleLight = async () => {
    if (!id) return;
    try {
      await camerasApi.openipc.light(id);
      message.success('Подсветка переключена');
    } catch {
      message.error('Ошибка');
    }
  };

  if (loading) return <Spin size="large" style={{ display: 'block', marginTop: 100 }} />;
  if (!camera) return <Title level={4}>Камера не найдена</Title>;

  const recColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 100 },
    { title: 'Начало', dataIndex: 'start_time', key: 'start_time', render: (v: string) => new Date(v).toLocaleString() },
    { title: 'Статус', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={s === 'recording' ? 'blue' : 'green'}>{s}</Tag> },
    { title: 'Длит.', dataIndex: 'duration', key: 'duration', render: (v: number) => `${Math.round(v)}с` },
    { title: 'Размер', dataIndex: 'file_size', key: 'file_size', render: (v: number) => `${(v / 1024 / 1024).toFixed(1)} MB` },
    {
      title: '', key: 'act',
      render: (_: unknown, rec: Recording) =>
        rec.status === 'recording' ? (
          <Button size="small" danger onClick={() => handleStopRecording(rec.id)}>Стоп</Button>
        ) : null,
    },
  ];

  const tabItems = [
    {
      key: 'video',
      label: 'Видео',
      children: (
        <Row gutter={[16, 16]}>
          <Col xs={24} md={16}>
            <Card
              title="Прямой эфир"
              extra={
                <Space>
                  <Tag color={camera.is_online ? 'green' : 'red'}>
                    {camera.is_online ? 'Онлайн' : 'Офлайн'}
                  </Tag>
                  <Button
                    type="primary"
                    icon={camera.is_recording ? <StopOutlined /> : <PlayCircleOutlined />}
                    danger={camera.is_recording}
                    loading={recording}
                    onClick={handleStartRecording}
                  >
                    {camera.is_recording ? 'Остановить' : 'Запись'}
                  </Button>
                </Space>
              }
              variant="outlined"
              styles={{ body: { padding: 0 } }}
            >
              <div style={{
                background: '#000', minHeight: 400,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                borderRadius: '0 0 8px 8px',
              }}>
                {camera.is_online ? (
                  <img
                    src={`/api/cameras/${camera.id}/stream`}
                    alt="RTSP Stream"
                    style={{ maxWidth: '100%', maxHeight: '100%' }}
                  />
                ) : (
                  <Text style={{ color: '#666' }}>Нет сигнала</Text>
                )}
              </div>
            </Card>
          </Col>

          <Col xs={24} md={8}>
            {/* Снапшот */}
            <Card
              title={<Space><CameraOutlined /> Снапшот</Space>}
              extra={<Button size="small" icon={<ReloadOutlined />} onClick={refreshSnapshot}>Обновить</Button>}
              variant="outlined"
              styles={{ body: { padding: 8, textAlign: 'center' } }}
            >
              {camera.snapshot_base64 ? (
                <Image src={camera.snapshot_base64} alt="Snapshot" style={{ maxHeight: 200, borderRadius: 4 }} />
              ) : (
                <div style={{ padding: 40, color: '#888' }}>
                  <CameraOutlined style={{ fontSize: 32 }} />
                  <div>Нажмите «Обновить»</div>
                </div>
              )}
            </Card>

            {/* Потоки */}
            <Card title={<Space><SettingOutlined /> Потоки</Space>} variant="outlined" style={{ marginTop: 12 }}>
              <Space direction="vertical" style={{ width: '100%' }}>
                {camera.mjpeg_url && (
                  <Button block size="small" onClick={() => window.open(camera.mjpeg_url, '_blank')}>MJPEG</Button>
                )}
                {camera.hls_url && (
                  <Button block size="small" onClick={() => window.open(camera.hls_url, '_blank')}>HLS</Button>
                )}
                <Button block size="small" onClick={() => window.open(`/api/cameras/${camera.id}/stream`, '_blank')}>
                  RTSP → MJPEG proxy
                </Button>
              </Space>
            </Card>
          </Col>
        </Row>
      ),
    },
    {
      key: 'openipc',
      label: 'OpenIPC',
      children: (
        <Row gutter={[16, 16]}>
          <Col xs={24} md={12}>
            <Card title="Ночной режим" variant="outlined">
              <Space wrap>
                <Tooltip title="Вкл/Выкл ночной режим">
                  <Button
                    icon={<BulbOutlined />}
                    loading={nightLoading}
                    onClick={handleNightToggle}
                  >
                    Переключить
                  </Button>
                </Tooltip>
                <Tooltip title="Механический IR-фильтр">
                  <Button icon={<SettingOutlined />} onClick={handleIrcut}>
                    IR-Cut
                  </Button>
                </Tooltip>
                <Tooltip title="IR-подсветка">
                  <Button icon={<SunOutlined />} onClick={handleLight}>
                    Подсветка
                  </Button>
                </Tooltip>
              </Space>
            </Card>
          </Col>

          <Col xs={24} md={12}>
            <Card title="Информация" variant="outlined">
              <Descriptions column={1} size="small">
                <Descriptions.Item label="RTSP">{camera.rtsp_url}</Descriptions.Item>
                <Descriptions.Item label="Производитель">{camera.manufacturer}</Descriptions.Item>
                <Descriptions.Item label="Локация">{camera.location || '-'}</Descriptions.Item>
                <Descriptions.Item label="WebSocket">{camera.ws_video_url || '-'}</Descriptions.Item>
                <Descriptions.Item label="Создана">{new Date(camera.created_at).toLocaleString()}</Descriptions.Item>
              </Descriptions>
            </Card>

            {camera.majestic_config && (
              <Card title="Majestic Config" variant="outlined" style={{ marginTop: 12 }}>
                <pre style={{ fontSize: 11, maxHeight: 300, overflow: 'auto' }}>
                  {JSON.stringify(camera.majestic_config, null, 2)}
                </pre>
              </Card>
            )}
          </Col>
        </Row>
      ),
    },
    {
      key: 'recordings',
      label: 'Записи',
      children: (
        <Table columns={recColumns} dataSource={recordings} rowKey="id" pagination={{ pageSize: 10 }} size="small" />
      ),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/cameras')}>Назад</Button>
      </Space>

      <Title level={4}>
        <CameraOutlined style={{ marginRight: 8 }} />
        {camera.name}
      </Title>

      <Tabs defaultActiveKey="video" items={tabItems} />
    </div>
  );
}