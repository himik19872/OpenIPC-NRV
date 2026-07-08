import { useEffect, useState, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Card, Button, Select, Space, Typography, Modal, Spin, Empty, Row, Col, Tag, message, Slider, Segmented, Tooltip, Popover,
} from 'antd';
import {
  AppstoreOutlined, FullscreenOutlined, FullscreenExitOutlined, ReloadOutlined,
  CameraOutlined, BorderOutlined, PauseCircleOutlined, PlayCircleOutlined,
} from '@ant-design/icons';
import { camerasApi } from '../api/client';

const { Title, Text } = Typography;

interface Camera {
  id: string;
  name: string;
  rtsp_main_url: string;
  rtsp_sub_url?: string;
  is_online: boolean;
  location: string;
  manufacturer: string;
}

type GridSize = '2x2' | '2x3' | '3x3' | '4x4' | '5x5';

const GRID_CONFIG: Record<GridSize, { cols: number; rows: number }> = {
  '2x2': { cols: 2, rows: 2 },
  '2x3': { cols: 3, rows: 2 },
  '3x3': { cols: 3, rows: 3 },
  '4x4': { cols: 4, rows: 4 },
  '5x5': { cols: 5, rows: 5 },
};

type CellCamera = string | null; // camera_id or null (empty)

export default function LiveViewPage() {
  const navigate = useNavigate();
  const [cameras, setCameras] = useState<Camera[]>([]);
  const [loading, setLoading] = useState(true);
  const [gridSize, setGridSize] = useState<GridSize>('3x3');
  const [grid, setGrid] = useState<CellCamera[]>([]);
  const [fullscreenCam, setFullscreenCam] = useState<string | null>(null);
  const [pausedCells, setPausedCells] = useState<Set<string>>(new Set());

  const { cols, rows } = GRID_CONFIG[gridSize];
  const totalCells = cols * rows;

  // Загрузка списка камер
  const loadCameras = useCallback(async () => {
    try {
      const { data } = await camerasApi.list({ limit: 200 });
      setCameras(data.filter((c: Camera) => c.is_online));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadCameras(); }, [loadCameras]);

  // Инициализация сетки из localStorage
  useEffect(() => {
    const saved = localStorage.getItem('nrv_liveview_grid');
    if (saved) {
      try {
        const parsed = JSON.parse(saved) as { size: GridSize; cells: CellCamera[] };
        setGridSize(parsed.size);
        setGrid(parsed.cells || []);
      } catch { /* ignore */ }
    }
  }, []);

  // Сохраняем сетку при изменении
  useEffect(() => {
    if (grid.length > 0) {
      localStorage.setItem('nrv_liveview_grid', JSON.stringify({ size: gridSize, cells: grid }));
    }
  }, [grid, gridSize]);

  // Сброс сетки при смене размера
  const handleGridSizeChange = (size: GridSize) => {
    setGridSize(size);
    const newTotal = GRID_CONFIG[size].cols * GRID_CONFIG[size].rows;
    const padded = [...grid];
    while (padded.length < newTotal) padded.push(null);
    setGrid(padded.slice(0, newTotal));
  };

  // Назначить камеру в ячейку
  const setCell = (index: number, cameraId: string | null) => {
    const newGrid = [...grid];
    while (newGrid.length < totalCells) newGrid.push(null);
    newGrid[index] = cameraId;
    setGrid(newGrid.slice(0, totalCells));
  };

  // Получить имя камеры по id
  const getCameraName = (cid: string) => cameras.find((c) => c.id === cid)?.name || cid.slice(0, 8);

  // Получить свежий токен (через refresh или напрямую из login)
  const getFreshToken = useCallback(async (): Promise<string> => {
    let token = localStorage.getItem('access_token');
    if (token) return token;

    // Пробуем refresh
    const refresh = localStorage.getItem('refresh_token');
    if (refresh) {
      try {
        const res = await fetch('/api/auth/refresh', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refresh }),
        });
        if (res.ok) {
          const data = await res.json();
          localStorage.setItem('access_token', data.access_token);
          localStorage.setItem('refresh_token', data.refresh_token);
          return data.access_token;
        }
      } catch { /* ignore */ }
    }

    // Если refresh не сработал — редирект на логин
    localStorage.clear();
    window.location.href = '/login';
    return '';
  }, []);

  // Поток для ячейки — H.264 через ffmpeg -c copy (БЕЗ перекодирования!)
  //   stream=1 = доп. поток (низкое качество) — для сетки (снижаем нагрузку)
  //   stream=0 = основной поток (высокое качество) — для fullscreen
  const getStreamUrl = (cameraId: string, streamIdx: number) => {
    const token = localStorage.getItem('access_token') || '';
    return `/api/cameras/${cameraId}/stream/mse?stream=${streamIdx}&token=${token}`;
  };

  // Форсируем перезагрузку видео при смене потока (sub↔main)
  const gridVideoRefs = useRef<Map<string, HTMLVideoElement>>(new Map());
  const fullscreenVideoRef = useRef<HTMLVideoElement | null>(null);

  // При открытии fullscreen — перезагружаем видео с основным потоком
  useEffect(() => {
    if (fullscreenCam && fullscreenVideoRef.current) {
      fullscreenVideoRef.current.load();
      fullscreenVideoRef.current.play().catch(() => {});
    }
  }, [fullscreenCam]);

  // Переключение паузы ячейки
  const togglePause = (cameraId: string) => {
    setPausedCells((prev) => {
      const next = new Set(prev);
      if (next.has(cameraId)) next.delete(cameraId);
      else next.add(cameraId);
      return next;
    });
  };

  const fullscreenCamera = fullscreenCam ? cameras.find((c) => c.id === fullscreenCam) : null;

  if (loading) return <Spin size="large" style={{ display: 'block', marginTop: 100 }} />;

  return (
    <div>
      {/* Toolbar */}
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        marginBottom: 12, flexWrap: 'wrap', gap: 8,
      }}>
        <Space>
          <AppstoreOutlined style={{ fontSize: 18, color: '#1677ff' }} />
          <Title level={4} style={{ margin: 0 }}>Live View</Title>
          <Tag color="green">{cameras.length} онлайн</Tag>
        </Space>

        <Space wrap>
          <Segmented<GridSize>
            value={gridSize}
            onChange={handleGridSizeChange}
            options={['2x2', '2x3', '3x3', '4x4', '5x5']}
          />
          <Button icon={<ReloadOutlined />} onClick={loadCameras}>Обновить</Button>
        </Space>
      </div>

      {/* Сетка */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: `repeat(${cols}, 1fr)`,
        gridTemplateRows: `repeat(${rows}, 1fr)`,
        gap: 6,
        aspectRatio: `${cols} / ${rows}`,
        backgroundColor: '#1a1a2e',
        borderRadius: 8,
        padding: 6,
        overflow: 'hidden',
        minHeight: 500,
      }}>
        {Array.from({ length: totalCells }).map((_, idx) => {
          const camId = grid[idx];
          const cam = camId ? cameras.find((c) => c.id === camId) : null;
          const isPaused = camId ? pausedCells.has(camId) : false;

          return (
            <div
              key={idx}
              style={{
                position: 'relative',
                backgroundColor: cam ? '#000' : '#2a2a3e',
                borderRadius: 4,
                overflow: 'hidden',
                cursor: cam ? 'pointer' : 'default',
                border: cam ? '1px solid #333' : '1px dashed #555',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                minHeight: 0,
              }}
            >
              {cam ? (
                <>
                  {/* H.264 MSE стрим — БЕЗ перекодирования! */}
                  {!isPaused && (
                    <video
                      src={getStreamUrl(cam.id, 1)}
                      autoPlay
                      playsInline
                      muted
                      loop
                      style={{
                        width: '100%',
                        height: '100%',
                        objectFit: 'cover',
                      }}
                      onClick={() => setFullscreenCam(cam.id)}
                    />
                  )}

                  {/* Оверлей с именем и кнопками */}
                  <div style={{
                    position: 'absolute',
                    bottom: 0,
                    left: 0,
                    right: 0,
                    background: 'linear-gradient(transparent, rgba(0,0,0,0.8))',
                    padding: '4px 8px',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}>
                    <Text style={{ color: '#fff', fontSize: 11, fontWeight: 500 }} ellipsis>
                      {cam.name}
                    </Text>
                    <Space size={4}>
                      <Button
                        type="text" size="small"
                        icon={isPaused ? <PlayCircleOutlined /> : <PauseCircleOutlined />}
                        style={{ color: '#fff' }}
                        onClick={(e) => { e.stopPropagation(); togglePause(cam.id); }}
                      />
                      <Tooltip title="Развернуть">
                        <Button
                          type="text" size="small"
                          icon={<FullscreenOutlined />}
                          style={{ color: '#fff' }}
                          onClick={(e) => { e.stopPropagation(); setFullscreenCam(cam.id); }}
                        />
                      </Tooltip>
                    </Space>
                  </div>

                  {/* Индикатор записи (если камера пишет) */}
                  {cam.is_online && (
                    <div style={{
                      position: 'absolute', top: 6, right: 6,
                      width: 8, height: 8, borderRadius: '50%',
                      backgroundColor: '#52c41a',
                      boxShadow: '0 0 4px #52c41a',
                    }} />
                  )}
                </>
              ) : (
                /* Пустая ячейка — выбор камеры */
                <Select
                  showSearch
                  placeholder="Выбрать камеру"
                  size="small"
                  style={{ width: 140 }}
                  filterOption={(input, option) =>
                    (option?.label as string)?.toLowerCase().includes(input.toLowerCase())
                  }
                  value={null}
                  onChange={(val) => setCell(idx, val)}
                  options={cameras
                    .filter((c) => !grid.includes(c.id))
                    .map((c) => ({ value: c.id, label: c.name }))}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Fullscreen Modal */}
      <Modal
        title={
          <Space>
            <CameraOutlined />
            {fullscreenCamera?.name}
            {fullscreenCamera?.is_online && <Tag color="green">Онлайн</Tag>}
          </Space>
        }
        open={!!fullscreenCam}
        onCancel={() => setFullscreenCam(null)}
        footer={null}
        width="95vw"
        styles={{
          body: { padding: 0, background: '#000', minHeight: '70vh' },
          header: { background: '#111', color: '#fff', borderBottom: '1px solid #333' },
        }}
        closeIcon={<FullscreenExitOutlined style={{ color: '#fff', fontSize: 18 }} />}
        destroyOnClose
      >
        {fullscreenCam && (
          <div style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            minHeight: 'calc(90vh - 120px)',
            background: '#000',
          }}>
            <video
              key={fullscreenCam}
              ref={fullscreenVideoRef}
              src={getStreamUrl(fullscreenCam, 0)}
              autoPlay
              playsInline
              controls
              style={{
                maxWidth: '100%',
                maxHeight: 'calc(90vh - 120px)',
                objectFit: 'contain',
              }}
            />
          </div>
        )}
      </Modal>
    </div>
  );
}