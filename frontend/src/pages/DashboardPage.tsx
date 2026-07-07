import { useEffect, useState } from 'react';
import { Row, Col, Card, Statistic, Typography, Spin } from 'antd';
import {
  VideoCameraOutlined,
  PlaySquareOutlined,
  HddOutlined,
  AlertOutlined,
} from '@ant-design/icons';
import { camerasApi, systemApi } from '../api/client';

const { Title } = Typography;

interface Stats {
  disk: { total_gb: number; used_gb: number; free_gb: number; percent_used: number };
}

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [cameraCount, setCameraCount] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      systemApi.stats(),
      camerasApi.list({ limit: 1 }),
    ]).then(([statsRes, camRes]) => {
      setStats(statsRes.data);
      setCameraCount(camRes.data.length);
    }).finally(() => setLoading(false));
  }, []);

  if (loading) return <Spin size="large" style={{ display: 'block', marginTop: 100 }} />;

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>Дашборд</Title>

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="Камер"
              value={cameraCount}
              prefix={<VideoCameraOutlined />}
              valueStyle={{ color: '#1677ff' }}
            />
          </Card>
        </Col>

        <Col xs={24} sm={12} md={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="Активных записей"
              value={0}
              prefix={<PlaySquareOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>

        <Col xs={24} sm={12} md={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="Свободно на диске"
              value={stats?.disk?.free_gb ?? 0}
              suffix="GB"
              prefix={<HddOutlined />}
              precision={1}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>

        <Col xs={24} sm={12} md={6}>
          <Card bordered={false} hoverable>
            <Statistic
              title="Событий сегодня"
              value={0}
              prefix={<AlertOutlined />}
              valueStyle={{ color: '#fa8c16' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
        <Col span={24}>
          <Card title="Загрузка диска" bordered={false}>
            <div style={{
              background: '#f5f5f5', borderRadius: 8, height: 24, overflow: 'hidden',
            }}>
              <div style={{
                width: `${stats?.disk?.percent_used ?? 0}%`,
                height: '100%',
                background: 'linear-gradient(90deg, #1677ff, #52c41a, #fa8c16, #ff4d4f)',
                transition: 'width 0.5s',
                borderRadius: 8,
              }} />
            </div>
            <div style={{ marginTop: 8, textAlign: 'right', color: '#888' }}>
              Использовано: {stats?.disk?.used_gb ?? 0} / {stats?.disk?.total_gb ?? 0} GB
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  );
}