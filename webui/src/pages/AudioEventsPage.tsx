import { useState } from 'react'
import { audioAPI, type AudioEvent } from '../api/client'
import { useAsync } from '../hooks/useApi'
import {
  Volume2, RefreshCw, Loader2, Mic, Music,
  AlertTriangle, Siren, Dog, Car, Zap, GlassWater,
} from 'lucide-react'

/**
 * Значки по классам звуковых событий.
 *
 * Иконка помогает мгновенно отличить тревожные события (выстрел, крик)
 * от бытовых (музыка, речь), поэтому у каждого класса она своя.
 */
const CLASS_ICONS: Record<string, React.ReactNode> = {
  speech: <Mic size={16} />,
  shout: <Mic size={16} />,
  scream: <AlertTriangle size={16} />,
  gunshot: <Zap size={16} />,
  explosion: <Zap size={16} />,
  glass_break: <GlassWater size={16} />,
  dog: <Dog size={16} />,
  car_alarm: <Car size={16} />,
  alarm: <Siren size={16} />,
  music: <Music size={16} />,
}

/** Классы, которые считаются тревожными — подсвечиваем красным. */
const ALERT_CLASSES = new Set(['gunshot', 'explosion', 'scream', 'glass_break', 'shout', 'alarm'])

const CLASS_LABELS: Record<string, string> = {
  speech: 'Речь',
  shout: 'Крик',
  scream: 'Вопль',
  gunshot: 'Выстрел',
  explosion: 'Взрыв',
  glass_break: 'Разбитое стекло',
  dog: 'Лай собаки',
  car_alarm: 'Автосигнализация',
  alarm: 'Сирена',
  music: 'Музыка',
}

export default function AudioEventsPage() {
  const [page, setPage] = useState(1)
  const [classFilter, setClassFilter] = useState('')

  const { data, loading, error, refetch } = useAsync(
    () => audioAPI.events({
      page,
      page_size: 50,
      ...(classFilter ? { event_class: classFilter } : {}),
    }),
    [page, classFilter],
  )

  const events = data?.events || []
  const total = data?.total || 0
  const pages = Math.max(1, Math.ceil(total / 50))

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>События звука</h1>
          <p style={{ color: 'var(--text-secondary)', marginTop: 4 }}>
            Звуки, распознанные YAMNet: выстрелы, крики, разбитое стекло и другие
          </p>
        </div>
        <button className="btn btn-outline btn-sm" onClick={() => refetch()}>
          <RefreshCw size={16} />
          Обновить
        </button>
      </div>

      {/* Фильтр по классу */}
      <div className="card" style={{ marginBottom: 16, display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>Класс:</span>
        <button
          className={`btn btn-sm ${classFilter === '' ? 'btn-primary' : 'btn-outline'}`}
          onClick={() => { setClassFilter(''); setPage(1) }}
        >
          Все
        </button>
        {Object.entries(CLASS_LABELS).map(([value, label]) => (
          <button
            key={value}
            className={`btn btn-sm ${classFilter === value ? 'btn-primary' : 'btn-outline'}`}
            onClick={() => { setClassFilter(value); setPage(1) }}
            style={{ display: 'flex', alignItems: 'center', gap: 5 }}
          >
            {CLASS_ICONS[value]}
            {label}
          </button>
        ))}
      </div>

      <div className="card">
        {loading ? (
          <div style={{ textAlign: 'center', padding: 32, color: 'var(--text-secondary)' }}>
            <Loader2 size={24} className="spin" />
          </div>
        ) : error ? (
          <div style={{ textAlign: 'center', padding: 32, color: 'var(--danger)' }}>
            Не удалось загрузить события: {error}
          </div>
        ) : events.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-secondary)' }}>
            <Volume2 size={40} style={{ opacity: 0.2, marginBottom: 12 }} />
            <p>Звуковых событий нет</p>
            <p style={{ fontSize: 13, marginTop: 6 }}>
              Включите «Распознавать звуковые события» во вкладке «Звук» карточки камеры
            </p>
          </div>
        ) : (
          <>
            <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 12 }}>
              Всего событий: {total}
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Время</th>
                    <th>Класс</th>
                    <th>Уверенность</th>
                    <th>Громкость</th>
                    <th>Длительность</th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((ev: AudioEvent) => {
                    const alert = ALERT_CLASSES.has(ev.event_class)
                    return (
                      <tr key={ev.id}>
                        <td style={{ fontSize: 13, whiteSpace: 'nowrap' }}>
                          {new Date(ev.timestamp).toLocaleString('ru')}
                        </td>
                        <td>
                          <span style={{
                            display: 'flex', alignItems: 'center', gap: 6,
                            color: alert ? 'var(--danger)' : undefined,
                            fontWeight: alert ? 600 : undefined,
                          }}>
                            {CLASS_ICONS[ev.event_class] || <Volume2 size={16} />}
                            {CLASS_LABELS[ev.event_class] || ev.event_class}
                          </span>
                        </td>
                        <td>
                          {/* Полоса уверенности: нагляднее числа при просмотре ленты */}
                          <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                            <span style={{
                              width: 60, height: 6, borderRadius: 3,
                              background: 'var(--border)', overflow: 'hidden',
                            }}>
                              <span style={{
                                display: 'block',
                                width: `${ev.confidence * 100}%`,
                                height: '100%',
                                background: ev.confidence > 0.6 ? 'var(--success)' : 'var(--warning)',
                              }} />
                            </span>
                            <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                              {(ev.confidence * 100).toFixed(0)}%
                            </span>
                          </span>
                        </td>
                        <td style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                          {ev.loudness_db != null ? `${ev.loudness_db.toFixed(0)} дБ` : '—'}
                        </td>
                        <td style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                          {ev.duration_sec != null ? `${ev.duration_sec.toFixed(1)} с` : '—'}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {/* Постраничная навигация */}
            {pages > 1 && (
              <div style={{ display: 'flex', gap: 8, alignItems: 'center', justifyContent: 'center', marginTop: 16 }}>
                <button className="btn btn-outline btn-sm" disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}>
                  Назад
                </button>
                <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                  Страница {page} из {pages}
                </span>
                <button className="btn btn-outline btn-sm" disabled={page >= pages}
                  onClick={() => setPage((p) => p + 1)}>
                  Вперёд
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
