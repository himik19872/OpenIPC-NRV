import { useState } from 'react'
import { recordingsAPI, Recording, TRIGGER_LABELS, TriggerType } from '../api/client'
import { useAsync } from '../hooks/useApi'
import { Play, Trash2, HardDrive, X, Search } from 'lucide-react'

// Цвета бейджей триггеров: распознавание выделено, чтобы записи
// по лицам и номерам было видно сразу.
const TRIGGER_STYLES: Record<string, { bg: string; color: string }> = {
  face: { bg: 'rgba(139, 92, 246, 0.15)', color: '#8b5cf6' },
  plate: { bg: 'rgba(234, 88, 12, 0.15)', color: '#ea580c' },
  line: { bg: 'rgba(6, 182, 212, 0.15)', color: '#06b6d4' },
  object: { bg: 'rgba(34, 197, 94, 0.15)', color: '#22c55e' },
  always: { bg: 'rgba(107, 114, 128, 0.15)', color: '#6b7280' },
  manual: { bg: 'rgba(107, 114, 128, 0.15)', color: '#6b7280' },
}

export default function RecordingsPage() {
  const [page, setPage] = useState(1)
  // Фильтр по причине записи: '', object, line, face, plate...
  const [trigger, setTrigger] = useState<string>('')
  // Поиск по расшифровке триггера (номер машины, имя человека)
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  // Запись, открытая в плеере; null — плеер закрыт
  const [playing, setPlaying] = useState<{ url: string; title: string } | null>(null)
  // Ошибка воспроизведения: чаще всего это HEVC, который браузер не поддерживает
  const [playError, setPlayError] = useState(false)

  const { data, loading, refetch } = useAsync<any>(
    () => recordingsAPI.list({ page, page_size: 20, trigger: trigger || undefined, search: appliedSearch || undefined }),
    [page, trigger, appliedSearch],
  )

  const openPlayer = (rec: Recording) => {
    if (!rec.url) return
    setPlayError(false)
    const cam = rec.camera_name || rec.camera_id.slice(0, 8)
    setPlaying({
      url: rec.url,
      title: `${cam} — ${new Date(rec.start_time).toLocaleString('ru')}`,
    })
  }

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

  const triggerBadge = (rec: Recording) => {
    const t = (rec.trigger_type || 'object') as TriggerType
    const style = TRIGGER_STYLES[t] || TRIGGER_STYLES.object
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <span
          style={{
            display: 'inline-block',
            fontSize: 11,
            padding: '2px 8px',
            borderRadius: 10,
            background: style.bg,
            color: style.color,
            fontWeight: 500,
            whiteSpace: 'nowrap',
          }}
        >
          {TRIGGER_LABELS[t] || t}
        </span>
        {rec.trigger_detail && (
          <span style={{ fontSize: 11, color: 'var(--text-secondary)' }}>{rec.trigger_detail}</span>
        )}
      </div>
    )
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

      {/* Фильтры: поиск нужной записи по причине срабатывания.
          Это основной способ найти «всё, что записалось из-за номера ХХХ». */}
      <div className="card" style={{ marginBottom: 16, display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <label style={{ fontSize: 13, color: 'var(--text-secondary)' }}>Триггер:</label>
          <select
            className="input"
            style={{ width: 180 }}
            value={trigger}
            onChange={(e) => { setTrigger(e.target.value); setPage(1) }}
          >
            <option value="">Все</option>
            <option value="face">Лицо</option>
            <option value="plate">Номер авто</option>
            <option value="object">Объект</option>
            <option value="line">Пересечение линии</option>
            <option value="always">Непрерывная</option>
          </select>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flex: 1, minWidth: 220 }}>
          <Search size={16} style={{ color: 'var(--text-secondary)' }} />
          <input
            className="input"
            style={{ flex: 1 }}
            placeholder="Поиск: номер, имя, класс объекта"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') { setAppliedSearch(search); setPage(1) }
            }}
          />
          <button
            className="btn btn-outline btn-sm"
            onClick={() => { setAppliedSearch(search); setPage(1) }}
          >
            Найти
          </button>
          {(trigger || appliedSearch) && (
            <button
              className="btn btn-outline btn-sm"
              onClick={() => { setTrigger(''); setSearch(''); setAppliedSearch(''); setPage(1) }}
            >
              Сбросить
            </button>
          )}
        </div>
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
                  <th>Триггер</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {recordings.map((rec) => (
                  <tr key={rec.id}>
                    <td style={{ fontSize: 13, whiteSpace: 'nowrap' }}>
                      {new Date(rec.start_time).toLocaleString('ru')}
                    </td>
                    <td style={{ fontSize: 13 }}>
                      {rec.camera_name || (
                        <span style={{ color: 'var(--text-secondary)' }}>
                          {rec.camera_id.slice(0, 8)}
                        </span>
                      )}
                    </td>
                    <td>{formatDuration(rec.duration)}</td>
                    <td>{formatSize(rec.file_size)}</td>
                    <td>{rec.resolution || '—'}</td>
                    <td>{triggerBadge(rec)}</td>
                    <td>
                      <div style={{ display: 'flex', gap: 8 }}>
                        <button
                          className="btn btn-outline btn-sm"
                          title="Воспроизвести"
                          disabled={!rec.url}
                          onClick={() => openPlayer(rec)}
                        >
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

      {/* Плеер: открывается по кнопке воспроизведения */}
      {playing && (
        <div
          onClick={() => setPlaying(null)}
          style={{
            position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.75)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            zIndex: 1000, padding: 20,
          }}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{ background: 'var(--bg-card)', borderRadius: 10, padding: 16, maxWidth: 900, width: '100%' }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <h3 style={{ margin: 0, fontSize: 15 }}>{playing.title}</h3>
              <button className="btn btn-outline btn-sm" onClick={() => setPlaying(null)}>
                <X size={14} />
              </button>
            </div>
            <video
              src={playing.url}
              controls
              autoPlay
              onError={() => setPlayError(true)}
              style={{ width: '100%', borderRadius: 6, background: '#000', maxHeight: '70vh' }}
            />
            {playError && (
              <p style={{ marginTop: 12, marginBottom: 0, fontSize: 13, color: 'var(--danger)' }}>
                Браузер не смог воспроизвести файл. Скорее всего, запись в кодеке HEVC (H.265),
                который браузеры не поддерживают. Скачайте файл и откройте его в VLC.
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  )
}