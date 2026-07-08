import { useEffect, useState } from 'react';
import {
  Card, Typography, Button, Input, Space, Table, Tag, message, Spin, Alert, Descriptions, Row, Col, Modal, Result,
} from 'antd';
import { SearchOutlined, WifiOutlined, PlusOutlined, ScanOutlined } from '@ant-design/icons';
import { scannerApi, camerasApi } from '../api/client';

const { Title, Text } = Typography;

interface FoundCamera {
  ip_address: string;
  model: string;
  firmware: string;
  rtsp_main_url: string;
  snapshot_url: string;
  is_online: boolean;
}

export default function NetworkScannerPage() {
  const [subnet, setSubnet] = useState('');
  const [scanning, setScanning] = useState(false);
  const [found, setFound] = useState<FoundCamera[]>([]);
  const [adding, setAdding] = useState<string | null>(null);

  useEffect(() => {
    scannerApi.getSubnet().then(({ data }) => setSubnet(data.subnet));
  }, []);

  const handleScan = async () => {
    setScanning(true);
    setFound([]);
    try {
      const { data } = await scannerApi.scan(subnet);
      setFound(data.cameras);
      message.success(`Найдено камер: ${data.found}`);
    } catch {
      message.error('Ошибка сканирования');
    } finally {
      setScanning(false);
    }
  };

  const handleAdd = async (ip: string) => {
    setAdding(ip);
    try {
      await scannerApi.addFound(ip);
      message.success('Камера добавлена!');
      setFound((prev) => prev.filter((c) => c.ip_address !== ip));
    } catch (err: any) {
      message.error(err.response?.data?.detail || 'Ошибка');
    } finally {
      setAdding(null);
    }
  };

  const columns = [
    { title: 'IP', dataIndex: 'ip_address', key: 'ip', render: (v: string) => <Text code>{v}</Text> },
    { title: 'Модель', dataIndex: 'model', key: 'model' },
    { title: 'Прошивка', dataIndex: 'firmware', key: 'firmware', render: (v: string) => v || '-' },
    {
      title: 'Статус', dataIndex: 'is_online', key: 'status',
      render: (v: boolean) => <Tag color={v ? 'green' : 'red'}>{v ? 'Онлайн' : '—'}</Tag>,
    },
    {
      title: '', key: 'act',
      render: (_: unknown, rec: FoundCamera) => (
        <Button
          type="primary"
          icon={<PlusOutlined />}
          size="small"
          loading={adding === rec.ip_address}
          onClick={() => handleAdd(rec.ip_address)}
        >
          Добавить
        </Button>
      ),
    },
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 8 }}>
        <ScanOutlined style={{ marginRight: 8 }} />
        Поиск камер в сети
      </Title>
      <Text type="secondary" style={{ display: 'block', marginBottom: 20 }}>
        Автоматически обнаруживает камеры OpenIPC через Majestic API
      </Text>

      <Card variant="outlined" style={{ marginBottom: 20 }}>
        <Row gutter={16} align="middle">
          <Col flex="auto">
            <Input
              size="large"
              value={subnet}
              onChange={(e) => setSubnet(e.target.value)}
              placeholder="192.168.1.0/24"
              prefix={<WifiOutlined />}
              onPressEnter={handleScan}
            />
          </Col>
          <Col>
            <Button
              type="primary"
              size="large"
              icon={<SearchOutlined />}
              loading={scanning}
              onClick={handleScan}
            >
              Сканировать
            </Button>
          </Col>
        </Row>
      </Card>

      {scanning && (
        <Card variant="outlined" style={{ marginBottom: 20, textAlign: 'center', padding: 40 }}>
          <Spin size="large" />
          <div style={{ marginTop: 16 }}>
            <Text type="secondary">Сканирую подсеть {subnet}... Это может занять до минуты.</Text>
          </div>
        </Card>
      )}

      {!scanning && found.length === 0 && subnet && (
        <Card variant="outlined" style={{ textAlign: 'center', padding: 40 }}>
          <Text type="secondary">Нажмите «Сканировать» для поиска камер.</Text>
          <div style={{ marginTop: 8 }}>
            <Text type="secondary" style={{ fontSize: 12 }}>
              root / 12345 — стандартные учётные данные OpenIPC
            </Text>
          </div>
        </Card>
      )}

      {found.length > 0 && (
        <Card
          title={`Найдено: ${found.length} камер(ы)`}
          variant="outlined"
        >
          <Table columns={columns} dataSource={found} rowKey="ip_address" pagination={false} size="small" />
        </Card>
      )}
    </div>
  );
}