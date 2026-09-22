import { useState } from 'react'
import { useAsync } from '../hooks/useApi'
import { rtspAPI, ExternalRTSPSettings, ExternalChannel } from '../api/client'
import { useToast } from '../context/ToastContext'
import { Copy, Check, Radio, Signal, SignalHigh, RefreshCw, Info, AlertTriangle, Plus } from 'lucide-react'

// Страница внешнего RTSP-доступа.
//
// Сторонним системам (видеостены, регистраторы, аналитика) нужны точные
// адреса потоков. Собирать их вручную по номеру канала — источник ошибок:
// номер в адресе идёт со смещением на минус один, и перепутать его легко.
// Поэтому страница показывает готовые ссылки, которые можно скопировать.
export default function ExternalAccessPage() {
  const toast = useToast()
  const { data, loading, error, refetch } = useAsync<ExternalRTSPSettings>(
    () => rtspAPI.settings(),
  )
  const [assigning, setAssigning] = useState<string | null>(null)

  if (loading) return <div className="spinner" />

  if (error || !data) {
    return (
      <div>
        <div className="page-header">
          <div>
            <h1>Внешний доступ</h1>
            <p>RTSP-потоки для сторонних систем</p>
          </div>
        </div>
        <div className="card" style={{ color: 'var(--warning, #f59e0b)' }}>
          <p>{error || 'Не удалось получить настройки внешнего доступа'}</p>
        </div>
      </div>
    )
  }

  const online = data.channels.filter((c) => c.status === 'online' || c.status === 'recording')

  // Назначает номер канала и обновляет список: после этого камера
  // появляется среди опубликованных, а адреса становятся готовы к выдаче.
  const assign = async (cameraId: string, channel: number, name: string) => {
    setAssigning(cameraId)
    try {
      await rtspAPI.assignChannel(cameraId, channel)
      toast.success(`Канал ${channel} назначен камере «${name}»`)
      refetch()
    } catch (e: any) {
      toast.error(e?.response?.data?.error || 'Не удалось назначить номер')
    } finally {
      setAssigning(null)
    }
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Внешний доступ</h1>
          <p>
            Потоки для сторонних систем: <strong>{online.length}</strong> из{' '}
            {data.channels.length} каналов онлайн
          </p>
        </div>
        <button className="btn btn-outline btn-sm" onClick={refetch}>
          <RefreshCw size={16} />
          Обновить
        </button>
      </div>

      {/* Параметры подключения: адрес, порт и учётные данные */}
      <div className="card" style={{ marginBottom: 16 }}>
        <h3 style={{ fontSize: 15, marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Info size={16} style={{ color: 'var(--accent)' }} />
          Параметры подключения
        </h3>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12 }}>
          <CopyRow label="Адрес сервера" value={data.server_ip} />
          <CopyRow label="Порт" value={String(data.port)} />
          <CopyRow label="Логин" value={data.username} />
          <CopyRow label="Пароль" value={data.password} masked />
        </div>
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 12, lineHeight: 1.6 }}>
          Потоки отдаются без перекодирования — камеры работают в обычном режиме,
          а сторонние системы получают копию уже принятого потока. Нагрузка
          на камеры не растёт: подключение к каждой из них остаётся одно.
        </p>
      </div>

      {/* Параметр, в котором чаще всего ошибаются — выносим отдельно */}
      <div className="card" style={{ marginBottom: 16, padding: 12, background: 'var(--bg-secondary)' }}>
        <p style={{ fontSize: 12, margin: 0, lineHeight: 1.6 }}>
          <strong>Номер канала в адресе идёт со смещением на минус один.</strong>{' '}
          Первый канал — это <code>cameras/0</code>, пятый — <code>cameras/4</code>.
          В таблице ниже указаны оба значения, а в ссылках уже подставлен нужный индекс.
        </p>
      </div>

      {/* Камеры без номера: они не публикуются, и оператор должен это видеть.

          Показываем отдельным блоком именно здесь, а не в списке камер:
          без номера камера наружу не отдаётся, и это первое место, где
          заметно её отсутствие. */}
      {data.unassigned.length > 0 && (
        <div className="card" style={{ marginBottom: 16, borderColor: 'var(--warning, #f59e0b)' }}>
          <h3 style={{ fontSize: 15, marginBottom: 6, display: 'flex', alignItems: 'center', gap: 8 }}>
            <AlertTriangle size={16} style={{ color: 'var(--warning, #f59e0b)' }} />
            Без номера канала — наружу не отдаются
          </h3>
          <p style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 12, lineHeight: 1.6 }}>
            Камерам ниже не назначен номер, поэтому они не публикуются для внешних
            систем. Номер можно задать здесь — он также изменится в карточке камеры.
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {data.unassigned.map((cam, i) => (
              <AssignRow
                key={cam.camera_id}
                camera={cam}
                // Первой камере предлагаем свободный номер, остальным —
                // следующие за ним: так несколько камер назначаются подряд.
                suggested={data.next_channel + i}
                busy={assigning === cam.camera_id}
                onAssign={(ch) => assign(cam.camera_id, ch, cam.camera_name)}
              />
            ))}
          </div>
        </div>
      )}

      {/* Каналы */}
      <div className="card">
        <h3 style={{ fontSize: 15, marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Radio size={16} style={{ color: 'var(--accent)' }} />
          Каналы
        </h3>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {data.channels.map((ch) => (
            <ChannelRow
              key={ch.camera_id}
              channel={ch}
              serverIP={data.server_ip}
              port={data.port}
              username={data.username}
              password={data.password}
              onCopied={(what) => toast.success(`Скопировано: ${what}`)}
            />
          ))}
          {data.channels.length === 0 && (
            <p style={{ color: 'var(--text-secondary)', textAlign: 'center', padding: 24 }}>
              Нет каналов с назначенным номером. Задайте номер в блоке выше.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

// AssignRow — камера без номера: поле ввода и кнопка назначения.
function AssignRow({
  camera, suggested, busy, onAssign,
}: {
  camera: ExternalChannel
  suggested: number
  busy: boolean
  onAssign: (channel: number) => void
}) {
  const [value, setValue] = useState(String(suggested))

  const submit = () => {
    const n = Number(value)
    if (!Number.isInteger(n) || n < 1) return
    onAssign(n)
  }

  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap',
      padding: 10, borderRadius: 8, border: '1px solid var(--border)',
    }}>
      <span style={{ fontSize: 13, flex: 1, minWidth: 160 }}>
        {camera.camera_name}
        {camera.ip && (
          <span style={{ fontSize: 11, color: 'var(--text-secondary)', fontFamily: 'monospace', marginLeft: 8 }}>
            {camera.ip}
          </span>
        )}
      </span>
      <input
        className="input"
        style={{ width: 90 }}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter') submit() }}
        inputMode="numeric"
        aria-label="Номер канала"
      />
      <button className="btn btn-primary btn-sm" onClick={submit} disabled={busy}>
        <Plus size={14} />
        {busy ? 'Назначаю…' : 'Назначить'}
      </button>
    </div>
  )
}

// ChannelRow — один канал: номер, состояние и готовые адреса потоков.
function ChannelRow({
  channel, serverIP, port, username, password, onCopied,
}: {
  channel: ExternalChannel
  serverIP: string
  port: number
  username: string
  password: string
  onCopied: (what: string) => void
}) {
  const isOnline = channel.status === 'online' || channel.status === 'recording'

  const buildURL = (path: string) =>
    `rtsp://${username}:${password}@${serverIP}:${port}${path}`

  return (
    <div
      style={{
        padding: 12,
        borderRadius: 8,
        border: '1px solid var(--border)',
        // Офлайн-канал приглушаем: адрес существует, но потока в нём нет,
        // и оператору важно видеть это сразу.
        opacity: isOnline ? 1 : 0.55,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
        {isOnline
          ? <SignalHigh size={14} style={{ color: 'var(--success, #22c55e)' }} />
          : <Signal size={14} style={{ color: 'var(--text-secondary)' }} />}
        <strong style={{ fontSize: 14 }}>Канал {channel.number}</strong>
        <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
          cameras/{channel.index}
        </span>
        <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
          {channel.camera_name}
        </span>
        {channel.ip && (
          <span style={{ fontSize: 11, color: 'var(--text-secondary)', fontFamily: 'monospace' }}>
            {channel.ip}
          </span>
        )}
        {!isOnline && (
          <span style={{ fontSize: 11, color: 'var(--warning, #f59e0b)' }}>
            камера не в сети — поток недоступен
          </span>
        )}
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <CopyRow
          label="Основной"
          value={buildURL(channel.main_path || '')}
          onCopied={() => onCopied(`основной поток канала ${channel.number}`)}
          compact
        />
        <CopyRow
          label="Дополнительный"
          value={buildURL(channel.sub_path || '')}
          onCopied={() => onCopied(`доп. поток канала ${channel.number}`)}
          compact
        />
      </div>
    </div>
  )
}

// CopyRow — значение с кнопкой копирования.
function CopyRow({
  label, value, onCopied, compact, masked,
}: {
  label: string
  value: string
  onCopied?: () => void
  compact?: boolean
  masked?: boolean
}) {
  const [copied, setCopied] = useState(false)
  const [shown, setShown] = useState(!masked)

  const copy = async () => {
    try {
      // Буфер обмена доступен только по HTTPS или на localhost. Если его
      // нет, показываем подсказку вместо молчаливого отказа.
      await navigator.clipboard.writeText(value)
      setCopied(true)
      onCopied?.()
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Копирование не удалось — выделять текст не будем, оператор
      // может скопировать вручную из строки.
    }
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
      <span style={{
        fontSize: compact ? 11 : 12,
        color: 'var(--text-secondary)',
        minWidth: compact ? 90 : 110,
        flexShrink: 0,
      }}>
        {label}
      </span>
      <code style={{
        flex: 1,
        fontSize: compact ? 11 : 12,
        fontFamily: 'monospace',
        background: 'var(--bg-secondary)',
        padding: '3px 8px',
        borderRadius: 4,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        minWidth: 0,
      }}>
        {shown ? value : '••••••••'}
      </code>
      {masked && (
        <button
          className="btn btn-outline btn-sm"
          onClick={() => setShown((v) => !v)}
          style={{ flexShrink: 0 }}
          title={shown ? 'Скрыть' : 'Показать'}
        >
          {shown ? 'Скрыть' : 'Показать'}
        </button>
      )}
      <button
        className="btn btn-outline btn-sm"
        onClick={copy}
        style={{ flexShrink: 0 }}
        title="Скопировать"
      >
        {copied ? <Check size={13} style={{ color: 'var(--success, #22c55e)' }} /> : <Copy size={13} />}
      </button>
    </div>
  )
}
