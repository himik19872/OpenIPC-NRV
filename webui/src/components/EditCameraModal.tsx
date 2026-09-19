import { useState } from 'react'
import { X, Save, Loader2, PlugZap, CheckCircle2, XCircle } from 'lucide-react'
import { camerasAPI, type Camera, type StreamProbeResult } from '../api/client'

interface Props {
  camera: Camera
  onClose: () => void
  onSaved: () => void
}

/**
 * Модальное окно редактирования камеры.
 * Креды (username/password) хранятся в settings и приходят с бэкенда,
 * поэтому их можно отредактировать, не вводя заново.
 */
export default function EditCameraModal({ camera, onClose, onSaved }: Props) {
  const [form, setForm] = useState({
    name: camera.name || '',
    ip: camera.ip || '',
    mac: camera.mac || '',
    firmware: camera.firmware || '',
    main_stream: camera.main_stream || '',
    sub_stream: camera.sub_stream || '',
    rtsp_url: camera.rtsp_url || '',
    wg_ip: camera.wg_ip || '',
    username: (camera.settings?.username as string) || '',
    password: (camera.settings?.password as string) || '',
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [ptz, setPtz] = useState(!!camera.ptz)
  // Результаты проверки по каждому потоку: 'main' и 'sub'.
  const [probe, setProbe] = useState<Record<string, StreamProbeResult | null>>({})
  const [probing, setProbing] = useState<string | null>(null)

  /**
   * Проверяет поток перед сохранением.
   *
   * Отправляем адрес БЕЗ встроенных кредов и логин/пароль отдельно:
   * бэкенд соберёт ту же строку, что потом получит MediaMTX, поэтому проверка
   * отражает реальный результат, а не только факт доступности камеры.
   */
  const runProbe = async (kind: 'main' | 'sub') => {
    const url = kind === 'main' ? form.main_stream : form.sub_stream
    if (!url.trim()) {
      setProbe(prev => ({ ...prev, [kind]: { ok: false, message: 'адрес не заполнен', has_audio: false } }))
      return
    }
    setProbing(kind)
    try {
      const res = await camerasAPI.probeStream({
        rtsp_url: url,
        username: form.username,
        password: form.password,
      })
      setProbe(prev => ({ ...prev, [kind]: res.data }))
    } catch {
      setProbe(prev => ({ ...prev, [kind]: { ok: false, message: 'не удалось выполнить проверку', has_audio: false } }))
    } finally {
      setProbing(null)
    }
  }

  const set = (field: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(prev => ({ ...prev, [field]: e.target.value }))

  /**
   * Обновляет поле потока и, если из адреса виден хост, подставляет IP.
   *
   * IP нужен для PTZ, SSH-команд и снапшотов — без него часть функций
   * камеры не работает. Тянуть его вручную из RTSP-строки неудобно,
   * а адрес всегда содержит нужный хост.
   */
  const setStream = (field: 'main_stream' | 'sub_stream') => (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setForm(prev => ({ ...prev, [field]: value }))
    // Ранее выполненная проверка относилась к старому адресу — сбрасываем,
    // чтобы не показывать устаревший результат.
    setProbe(prev => ({ ...prev, [field === 'main_stream' ? 'main' : 'sub']: null }))

    if (field !== 'main_stream') return
    const host = extractHost(value)
    if (host) setForm(prev => ({ ...prev, ip: host }))
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)

    // Отправляем только изменённые поля — бэкенд перерегистрирует потоки
    // в MediaMTX, если поменялись адреса или креды.
    const payload: Record<string, string> = {}
    if (form.name !== camera.name) payload.name = form.name
    if (form.ip !== (camera.ip || '')) payload.ip = form.ip
    if (form.mac !== (camera.mac || '')) payload.mac = form.mac
    if (form.firmware !== (camera.firmware || '')) payload.firmware = form.firmware
    if (form.main_stream !== (camera.main_stream || '')) payload.main_stream = form.main_stream
    if (form.sub_stream !== (camera.sub_stream || '')) payload.sub_stream = form.sub_stream
    if (form.rtsp_url !== (camera.rtsp_url || '')) payload.rtsp_url = form.rtsp_url
    if (form.wg_ip !== (camera.wg_ip || '')) payload.wg_ip = form.wg_ip
    // ptz — булево поле, тип payload расширяем через any при отправке.
    const ptzChanged = ptz !== !!camera.ptz

    const oldUser = (camera.settings?.username as string) || ''
    const oldPass = (camera.settings?.password as string) || ''
    if (form.username !== oldUser) payload.username = form.username
    if (form.password !== oldPass) payload.password = form.password

    if (Object.keys(payload).length === 0 && !ptzChanged) {
      onClose()
      return
    }

    try {
      await camerasAPI.update(camera.id, ptzChanged ? { ...payload, ptz } : payload)
      onSaved()
      onClose()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось сохранить изменения')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 640 }}>
        <div className="modal-header">
          <h2 style={{ fontSize: 18 }}>Редактирование камеры</h2>
          <button className="btn btn-outline btn-sm" onClick={onClose} aria-label="Закрыть">
            <X size={16} />
          </button>
        </div>

        <div className="modal-body" style={{ maxHeight: '65vh', overflowY: 'auto' }}>
          <Field label="Название">
            <input className="input" value={form.name} onChange={set('name')} />
          </Field>

          <div className="grid grid-2" style={{ gap: 12 }}>
            <Field label="IP-адрес">
              <input className="input" value={form.ip} onChange={set('ip')} placeholder="192.168.1.10" />
            </Field>
            <Field label="WireGuard IP">
              <input className="input" value={form.wg_ip} onChange={set('wg_ip')} placeholder="10.99.0.2" />
            </Field>
          </div>

          <div className="grid grid-2" style={{ gap: 12 }}>
            <Field label="MAC-адрес">
              <input className="input" value={form.mac} onChange={set('mac')} placeholder="aa:bb:cc:dd:ee:ff" />
            </Field>
            <Field label="Прошивка">
              <input className="input" value={form.firmware} onChange={set('firmware')} />
            </Field>
          </div>

          <h3 style={{ fontSize: 14, margin: '16px 0 8px' }}>Потоки</h3>
          <Field label="Основной поток (main)">
            <input className="input" value={form.main_stream} onChange={setStream('main_stream')}
              placeholder="rtsp://192.168.1.10/stream=0" />
          </Field>
          <ProbeRow
            result={probe.main ?? null}
            busy={probing === 'main'}
            onProbe={() => runProbe('main')}
          />
          <Field label="Дополнительный поток (sub)">
            <input className="input" value={form.sub_stream} onChange={setStream('sub_stream')}
              placeholder="rtsp://192.168.1.10/stream=1" />
          </Field>
          <ProbeRow
            result={probe.sub ?? null}
            busy={probing === 'sub'}
            onProbe={() => runProbe('sub')}
          />
          <Field label="RTSP URL (fallback)">
            <input className="input" value={form.rtsp_url} onChange={set('rtsp_url')} />
          </Field>

          <h3 style={{ fontSize: 14, margin: '16px 0 8px' }}>Учётные данные камеры</h3>
          <p style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 8 }}>
            Используются для RTSP и SSH-команд (перезапуск стримера, ребут).
          </p>
          <div className="grid grid-2" style={{ gap: 12 }}>
            <Field label="Логин">
              <input className="input" value={form.username} onChange={set('username')} autoComplete="off" />
            </Field>
            <Field label="Пароль">
              <input className="input" type="password" value={form.password} onChange={set('password')} autoComplete="new-password" />
            </Field>
          </div>

          <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 6, cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={ptz}
              onChange={e => setPtz(e.target.checked)}
              style={{ width: 'auto', marginBottom: 0 }}
            />
            <span style={{ fontSize: 13 }}>
              Поворотная камера (PTZ) — показывать пульт управления
            </span>
          </label>

          {error && (
            <div style={{
              marginTop: 12, padding: '8px 12px', borderRadius: 6, fontSize: 13,
              background: 'rgba(255,59,92,0.1)', color: 'var(--danger)',
            }}>
              {error}
            </div>
          )}
        </div>

        <div className="modal-footer">
          <button className="btn btn-outline btn-sm" onClick={onClose} disabled={saving}>
            Отмена
          </button>
          <button className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving}>
            {saving ? <Loader2 size={14} className="spin" /> : <Save size={14} />}
            {saving ? 'Сохранение...' : 'Сохранить'}
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label style={{ display: 'block', marginBottom: 10 }}>
      <span style={{ display: 'block', fontSize: 12, color: 'var(--text-secondary)', marginBottom: 4 }}>
        {label}
      </span>
      {children}
    </label>
  )
}

/**
 * Извлекает хост из RTSP-адреса: «rtsp://user:pass@192.168.1.10:554/stream=0»
 * → «192.168.1.10». Возвращает пустую строку, если хост не распознан
 * (например, пользователь ещё не закончил ввод).
 */
function extractHost(url: string): string {
  const m = url.match(/^rtsps?:\/\/(?:[^@\/]*@)?([^:\/?#]+)/i)
  if (!m) return ''
  const host = m[1]
  // Принимаем только IP: имена хостов в это поле не подходят.
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(host) ? host : ''
}

/**
 * Кнопка проверки потока и её результат.
 *
 * Нужна, чтобы оператор убедился в правильности адреса и пароля ДО сохранения:
 * иначе камера сохранится «офлайн», и причину придётся искать в логах.
 */
function ProbeRow({
  result, busy, onProbe,
}: {
  result: StreamProbeResult | null
  busy: boolean
  onProbe: () => void
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8, marginBottom: 10, marginTop: -4 }}>
      <button
        className="btn btn-outline btn-sm"
        onClick={onProbe}
        disabled={busy}
        style={{ padding: '4px 10px', fontSize: 12, flexShrink: 0 }}
        type="button"
      >
        {busy ? <Loader2 size={13} className="spin" /> : <PlugZap size={13} />}
        {busy ? 'Проверка...' : 'Проверить'}
      </button>

      {result && (
        <span style={{
          display: 'flex', alignItems: 'center', gap: 5, fontSize: 12,
          color: result.ok ? 'var(--success)' : 'var(--danger)',
          lineHeight: 1.4, paddingTop: 3,
        }}>
          {result.ok ? <CheckCircle2 size={13} /> : <XCircle size={13} />}
          {result.message}
        </span>
      )}
    </div>
  )
}
