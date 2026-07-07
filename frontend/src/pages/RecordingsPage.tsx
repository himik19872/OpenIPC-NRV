import { useState } from 'react';
import { Card, Typography } from 'antd';
import { PlaySquareOutlined } from '@ant-design/icons';

const { Title, Text } = Typography;

export default function RecordingsPage() {
  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>Записи</Title>
      <Card bordered={false}>
        <div style={{ textAlign: 'center', padding: 60, color: '#888' }}>
          <PlaySquareOutlined style={{ fontSize: 48, marginBottom: 16 }} />
          <div><Text type="secondary">Выберите камеру для просмотра её записей</Text></div>
        </div>
      </Card>
    </div>
  );
}