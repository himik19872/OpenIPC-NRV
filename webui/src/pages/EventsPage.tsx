import { useState } from 'react'
import { eventsAPI, DetectionEvent } from '../api/client'
import { useAsync } from '../hooks/useApi'
import { AlertTriangle, Car, User, Dog, Package, Eye } from 'lucide-react'

const classIcons: Record<string, any> = {
  person: User,
  car: Car,
  truck: Car,
  dog: Dog,
  package: Package,
}

const classColors: Record<string, string> = {
  person: '#5b7fff',
  car: '#ff9f0a',
  truck: '#ff6b35',
  dog: '#34c759',
  package: '#af52de',
}

export default function EventsPage() {
  const [page, setPage] = useState(1)
  const { data, loading, refetch } = useAsync<any>(() => eventsAPI.list({ page, page_size: 20 }), [page])

  const events: DetectionEvent[] = data?.events || []
  const total = data?.total || 0
  const totalPages = Math.ceil(total / 20)

  if (loading) return <div className="spinner" />

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>События детекции</h1>
          <p>{total} событий • AI-распознавание объектов</p>
        </div>
        <button className="btn btn-outline btn-sm" onClick={refetch}>
          Обновить
        </button>
      </div>

      {events.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', padding: 60, color: 'var(--text-secondary)' }}>
          <AlertTriangle size={48} style={{ marginBottom: 16, opacity: 0.3 }} />
          <p>Пока нет событий детекции</p>
        </div>
      ) : (
        <div className="card" style={{ padding: 0 }}>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Время</th>
                  <th>Объект</th>
                  <th>Точность</th>
                  <th>Камера</th>
                  <th>Снимок</th>
                </tr>
              </thead>
              <tbody>
                {events.map((ev) => {
                  const Icon = classIcons[ev.object_class] || Eye
                  const color = classColors[ev.object_class] || 'var(--text-secondary)'
                  return (
                    <tr key={ev.id}>
                      <td style={{ whiteSpace: 'nowrap', fontSize: 13 }}>
                        {new Date(ev.timestamp).toLocaleString('ru')}
                      </td>
                      <td>
                        <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <Icon size={18} style={{ color }} />
                          <span style={{ textTransform: 'capitalize' }}>{ev.object_class}</span>
                          {ev.track_id && (
                            <span style={{ fontSize: 11, color: 'var(--text-secondary)' }}>
                              ID:{ev.track_id}
                            </span>
                          )}
                        </span>
                      </td>
                      <td>
                        <span style={{
                          color: ev.confidence > 0.7 ? 'var(--success)' : ev.confidence > 0.4 ? 'var(--warning)' : 'var(--danger)',
                          fontWeight: 600,
                        }}>
                          {(ev.confidence * 100).toFixed(0)}%
                        </span>
                      </td>
                      <td style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                        {ev.camera_name || ev.camera_id.slice(0, 8)}
                      </td>
                      <td>
                        {ev.thumbnail_path ? (
                          <img src={ev.thumbnail_path} alt="thumb" style={{ width: 80, height: 45, borderRadius: 4, objectFit: 'cover' }} />
                        ) : (
                          <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>—</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Пагинация */}
      {totalPages > 1 && (
        <div style={{ display: 'flex', justifyContent: 'center', gap: 8, marginTop: 20 }}>
          <button className="btn btn-outline btn-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>
            ← Назад
          </button>
          <span style={{ padding: '6px 12px', fontSize: 14, color: 'var(--text-secondary)' }}>
            {page} / {totalPages}
          </span>
          <button className="btn btn-outline btn-sm" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
            Вперёд →
          </button>
        </div>
      )}
    </div>
  )
}