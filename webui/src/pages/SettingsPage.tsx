import { useEffect, useState } from 'react'
import { HardDrive, Save, Loader2, Camera as CameraIcon, Server, AlertCircle } from 'lucide-react'
import { settingsAPI, type ServerSettings, type StorageConfig } from '../api/client'
import { useToast } from '../context/ToastContext'

/** Блок настроек одного хранилища (записи или снимки). */
function StorageSection({
  title,
  icon,
  hint,
  value,
  onChange,
}: {
  title: string
  icon: React.ReactNode
  hint: string
  value: StorageConfig
  onChange: (v: StorageConfig) => void
}) {
  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <h3 style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4, fontSize: 15 }}>
        {icon}
        {title}
      </h3>
      <p style={{ fontSize: 12, color: 'var(--text-secondary)', margin: '0 0 14px' }}>{hint}</p>

      <label style={{ fontSize: 12, display: 'block', marginBottom: 12 }}>
        Хранилище
        <select
          className="input"
          value={value.backend}
          onChange={(e) => onChange({ ...value, backend: e.target.value as 'minio' | 'local' })}
          style={{ width: '100%', marginTop: 4 }}
        >
          <option value="minio">MinIO / S3 (объектное хранилище)</option>
          <option value="local">Локальный диск сервера</option>
        </select>
      </label>

      {value.backend === 'local' ? (
        <label style={{ fontSize: 12, display: 'block', marginBottom: 12 }}>
          Каталог на диске
          <input
            className="input"
            value={value.local_path}
            onChange={(e) => onChange({ ...value, local_path: e.target.value })}
            placeholder="/var/lib/nvr/recordings"
            style={{ width: '100%', marginTop: 4 }}
          />
          <span style={{ display: 'block', color: 'var(--text-secondary)', marginTop: 4 }}>
            Путь внутри контейнера бэкенда. Убедитесь, что каталог смонтирован как volume.
          </span>
        </label>
      ) : (
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8, padding: 10, borderRadius: 6, background: 'rgba(120,140,255,0.06)', fontSize: 12, marginBottom: 12 }}>
          <Server size={14} style={{ marginTop: 2, flexShrink: 0 }} />
          <span>
            Файлы сохраняются в бакет MinIO из настроек окружения бэкенда
            (<code>MINIO_BUCKET</code>). Ссылки выдаются временными (presigned).
          </span>
        </div>
      )}

      <label style={{ fontSize: 12, display: 'block' }}>
        Глубина хранения, дней
        <input
          type="number"
          min={0}
          max={3650}
          className="input"
          value={value.retention_days}
          onChange={(e) => onChange({ ...value, retention_days: Math.max(0, Math.min(3650, parseInt(e.target.value) || 0)) })}
          style={{ width: 120, marginTop: 4 }}
        />
        <span style={{ display: 'block', color: 'var(--text-secondary)', marginTop: 4 }}>
          Файлы старше указанного срока удаляются автоматически. 0 — хранить бессрочно.
        </span>
      </label>
    </div>
  )
}

export default function SettingsPage() {
  const { success, error } = useToast()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [settings, setSettings] = useState<ServerSettings | null>(null)
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    let cancelled = false
    settingsAPI.get()
      .then((res) => {
        if (cancelled) return
        setSettings(res.data)
        setDirty(false)
      })
      .catch((e) => {
        if (!cancelled) error(e.response?.data?.error || 'Не удалось загрузить настройки сервера')
      })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [error])

  const patch = (key: keyof ServerSettings, value: StorageConfig) => {
    setSettings((s) => (s ? { ...s, [key]: value } : s))
    setDirty(true)
  }

  const save = async () => {
    if (!settings) return
    setSaving(true)
    try {
      const res = await settingsAPI.update(settings)
      setSettings(res.data)
      setDirty(false)
      success('Настройки сервера сохранены')
    } catch (e: any) {
      error(e.response?.data?.error || 'Не удалось сохранить настройки')
    } finally {
      setSaving(false)
    }
  }

  if (loading || !settings) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: 'var(--text-secondary)' }}>
        <Loader2 size={16} className="spin" />
        Загрузка настроек...
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20, flexWrap: 'wrap', gap: 8 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 24 }}>Настройки сервера</h1>
          <p style={{ margin: '4px 0 0', color: 'var(--text-secondary)', fontSize: 13 }}>
            Где хранить записи и снимки, сколько их держать
          </p>
        </div>
        <button className="btn btn-primary" onClick={save} disabled={saving || !dirty}>
          {saving ? <Loader2 size={16} className="spin" /> : <Save size={16} />}
          {dirty ? 'Сохранить' : 'Сохранено'}
        </button>
      </div>

      <StorageSection
        title="Записи видео"
        icon={<HardDrive size={18} style={{ color: 'var(--accent)' }} />}
        hint="Куда складывать видеоархив, записанный по детекции или постоянно"
        value={settings.storage}
        onChange={(v) => patch('storage', v)}
      />

      <StorageSection
        title="Снимки событий"
        icon={<CameraIcon size={18} style={{ color: 'var(--accent)' }} />}
        hint="Кадры, сохраняемые в момент срабатывания детекции"
        value={settings.snapshots}
        onChange={(v) => patch('snapshots', v)}
      />

      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8, padding: 12, borderRadius: 6, background: 'rgba(255,180,80,0.08)', fontSize: 12 }}>
        <AlertCircle size={14} style={{ marginTop: 2, flexShrink: 0, color: 'var(--warning)' }} />
        <span>
          Выбор хранилища влияет на то, куда будут писаться файлы. Автоматическая
          очистка по глубине хранения подключается вместе с воркером записи.
        </span>
      </div>
    </div>
  )
}
