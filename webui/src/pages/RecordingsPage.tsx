import { useState } from 'react'
import { recordingsAPI, Recording } from '../api/client'
import { useAsync } from '../hooks/useApi'
import { Play, Trash2, HardDrive } from 'lucide-react'

export default function RecordingsPage() {
  const [page, setPage] = useState(1)
  const { data, loading, refetch } = useAsync<any>(() => recordingsAPI.list({ page, page_size: 20 }), [page])

  const recordings: Recording[] = data?.recordings || []
  const total = data?.total || 0
  const totalPages = Math.ceil(total / 20)

  const handleDelete = async (id: string) => {
    if (!confirm('Удалить запись?')) return
    try {
      await recordingsAPI.delete(id)
      refetch()
    } catch { /* ignore */ }
  }

  const formatDuration = (sec: number) => {
    const m = Math.floor(sec / 60)
    const s = Math.floor(sec % 60)
    return `${m}:${s.toString().padStart(2, '0')}`
  }

  const formatSize = (bytes: number) => {
    if (bytes > 1e9) return `${(bytes / 1e9).toFixed(2)} GB`
    if (bytes > 1e6) return `${(bytes / 1e6).toFixed(1)} MB`
    return `${(bytes / 1e3).toFixed(0)} KB`
  }

  if (loading) return <div className="spinner" />

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Архив записей</h1>
          <p>{total} записей</p>
        </div>
        <button className="btn btn-outline btn-sm" onClick={refetch}>
          Обновить
        </button>
      </div>

      {recordings.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', padding: 60, color: 'var(--text-secondary)' }}>
          <HardDrive size={48} style={{ marginBottom: 16, opacity: 0.3 }} />
          <p>Записи отсутствуют</p>
        </div>
      ) : (
        <div className="card" style={{ padding: 0 }}>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Время</th>
                  <th>Камера</th>
                  <th>Длительность</th>
                  <th>Размер</th>
                  <th>Качество</th>
                  <th>Тип</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {recordings.map((rec) => (
                  <tr key={rec.id}>
                    <td style={{ fontSize: 13, whiteSpace: 'nowrap' }}>
                      {new Date(rec.start_time).toLocaleString('ru')}
                    </td>
                    <td style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                      {rec.camera_id.slice(0, 8)}
                    </td>
                    <td>{formatDuration(rec.duration_sec)}</td>
                    <td>{formatSize(rec.file_size)}</td>
                    <td>{rec.resolution || '—'}</td>
                    <td>
                      {rec.event_triggered ? (
                        <span className="badge badge-recording">По событию</span>
                      ) : (
                        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>Непрерывная</span>
                      )}
                    </td>
                    <td>
                      <div style={{ display: 'flex', gap: 8 }}>
                        <button className="btn btn-outline btn-sm" title="Воспроизвести">
                          <Play size={14} />
                        </button>
                        <button
                          className="btn btn-outline btn-sm"
                          style={{ color: 'var(--danger)', borderColor: 'var(--danger)' }}
                          onClick={() => handleDelete(rec.id)}
                          title="Удалить"
                        >
                          <Trash2 size={14} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

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