import { useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { camerasAPI, Camera } from '../api/client'
import { useAsync } from '../hooks/useApi'
import { useToast } from '../context/ToastContext'
import Hls from 'hls.js'
import { Plus, Trash2, RefreshCw, Eye, Radio, Wifi, WifiOff } from 'lucide-react'

export default function CamerasPage() {
  const navigate = useNavigate()
  const toast = useToast()
  const { data: cameras, loading, error, refetch } = useAsync<Camera[]>(() => camerasAPI.list())
  const [showModal, setShowModal] = useState(false)
  const [form, setForm] = useState({
    name: '', rtsp_url: '', main_stream: '', sub_stream: '',
    ip: '', mac: '', firmware: '', username: '', password: '', wg_ip: '',
  })
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setFormError('')
    try {
      await camerasAPI.create(form)
      setShowModal(false)
      setForm({
        name: '', rtsp_url: '', main_stream: '', sub_stream: '',
        ip: '', mac: '', firmware: '', username: '', password: '', wg_ip: '',
      })
      toast.success(`Камера «${form.name}» добавлена`)
      refetch()
    } catch (err: any) {
      setFormError(err.response?.data?.error || 'Ошибка')
      toast.error('Не удалось добавить камеру')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (e: React.MouseEvent, id: string, name: string) => {
    e.stopPropagation()
    if (!confirm(`Удалить камеру «${name}»?`)) return
    try {
      await camerasAPI.delete(id)
      toast.success(`Камера «${name}» удалена`)
      refetch()
    } catch {
      toast.error('Ошибка удаления')
    }
  }

  if (loading) return <div className="spinner" />

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Камеры</h1>
          <p>Управление подключёнными камерами OpenIPC</p>
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <button className="btn btn-outline btn-sm" onClick={refetch}>
            <RefreshCw size={16} />
            Обновить
          </button>
          <button className="btn btn-primary" onClick={() => setShowModal(true)}>
            <Plus size={18} />
            Добавить камеру
          </button>
        </div>
      </div>

      {error && <p style={{ color: 'var(--danger)', marginBottom: 16 }}>{error}</p>}

      {!cameras || cameras.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', padding: 60, color: 'var(--text-secondary)' }}>
          <p style={{ fontSize: 18, marginBottom: 8 }}>Нет добавленных камер</p>
          <p>Нажмите «Добавить камеру», чтобы подключить устройство OpenIPC</p>
        </div>
      ) : (
        <div className="grid grid-3">
          {cameras.map((cam) => {
            const isOnline = cam.status === 'online' || cam.status === 'recording'
            const hasSubStream = !!(cam.sub_stream)
            const hasMainStream = !!(cam.main_stream || cam.rtsp_url)

            return (
            <div
              key={cam.id}
              className="card camera-card"
              onClick={() => navigate(`/cameras/${cam.id}`)}
              style={{ cursor: 'pointer' }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 12 }}>
                <h3 style={{ fontSize: 16 }}>{cam.name}</h3>
                <span className={`badge badge-${isOnline ? (cam.status === 'recording' ? 'recording' : 'online') : 'offline'}`}>
                  <span className={`badge-dot badge-dot-${isOnline ? 'online' : 'offline'}`} />
                  {cam.status}
                </span>
              </div>

              {/* Превью: живой субпоток, при неудаче — статичный кадр */}
              <div className="video-placeholder" style={{ position: 'relative', minHeight: 160 }}>
                {isOnline && hasSubStream && (
                  <CameraThumb id={cam.id} hlsUrl={`/api/v1/cameras/${cam.id}/hls/sub/index.m3u8`} />
                )}
                {isOnline && !hasSubStream && <SnapshotImage id={cam.id} name={cam.name} />}
                {!isOnline && (
                  <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <WifiOff size={28} style={{ color: 'var(--text-secondary)', opacity: 0.4 }} />
                  </div>
                )}
                <div style={{
                  position: 'absolute', top: 8, left: 8,
                  display: 'flex', gap: 4, flexWrap: 'wrap',
                }}>
                  {hasMainStream && (
                    <span style={{
                      background: 'rgba(0,0,0,0.65)', color: '#fff',
                      fontSize: 10, padding: '2px 6px', borderRadius: 4,
                    }}>main</span>
                  )}
                  {hasSubStream && (
                    <span style={{
                      background: 'rgba(59,130,246,0.7)', color: '#fff',
                      fontSize: 10, padding: '2px 6px', borderRadius: 4,
                    }}>sub</span>
                  )}
                </div>
                <div style={{ position: 'absolute', top: 8, right: 8 }}>
                  {isOnline ? (
                    <span style={{ display: 'flex', alignItems: 'center', gap: 4, background: 'rgba(0,0,0,0.65)', color: '#fff', fontSize: 11, padding: '2px 8px', borderRadius: 4 }}>
                      <Radio size={10} style={{ color: 'var(--danger)' }} />
                      LIVE
                    </span>
                  ) : (
                    <WifiOff size={16} style={{ color: 'var(--text-secondary)', opacity: 0.5 }} />
                  )}
                </div>
              </div>

              <div style={{ marginTop: 12, fontSize: 13, color: 'var(--text-secondary)', display: 'flex', flexDirection: 'column', gap: 2 }}>
                {cam.ip && (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    <Wifi size={12} style={{ color: 'var(--accent)' }} />
                    <span style={{ fontFamily: 'monospace' }}>{cam.ip}</span>
                  </div>
                )}
                {cam.wg_ip && <div>WireGuard IP: {cam.wg_ip}</div>}
                {cam.mac && <div style={{ fontSize: 11, fontFamily: 'monospace' }}>MAC: {cam.mac}</div>}
                {cam.firmware && <div style={{ fontSize: 11 }}>FW: {cam.firmware}</div>}
                {cam.main_stream && <div style={{ fontSize: 11, wordBreak: 'break-all', opacity: 0.7 }}>Main: {cam.main_stream.slice(0, 40)}...</div>}
                {cam.sub_stream && <div style={{ fontSize: 11, wordBreak: 'break-all', opacity: 0.7 }}>Sub: {cam.sub_stream.slice(0, 40)}...</div>}
                {!cam.main_stream && !cam.sub_stream && cam.rtsp_url && <div style={{ fontSize: 11, wordBreak: 'break-all' }}>RTSP: {cam.rtsp_url.slice(0, 35)}...</div>}
                <div style={{ marginTop: 4 }}>Добавлена: {new Date(cam.created_at).toLocaleDateString('ru')}</div>
              </div>

              <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
                <button
                  className="btn btn-outline btn-sm"
                  style={{ flex: 1 }}
                  onClick={(e) => {
                    e.stopPropagation()
                    navigate(`/cameras/${cam.id}`)
                  }}
                >
                  <Eye size={14} />
                  Просмотр
                </button>
                <button
                  className="btn btn-outline btn-sm"
                  style={{ color: 'var(--danger)', borderColor: 'var(--danger)' }}
                  onClick={(e) => handleDelete(e, cam.id, cam.name)}
                  title="Удалить"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          )})}
        </div>
      )}

      {/* Модальное окно добавления */}
      {showModal && (
        <div className="modal-overlay" onClick={() => setShowModal(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 520 }}>
            <h2>Добавить камеру</h2>
            <form onSubmit={handleAdd}>
              <label>Название *</label>
              <input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="Камера входа"
                required
              />
              <div className="grid grid-2" style={{ gap: 10 }}>
                <div>
                  <label>IP-адрес</label>
                  <input
                    value={form.ip}
                    onChange={(e) => setForm({ ...form, ip: e.target.value })}
                    placeholder="192.168.1.75"
                  />
                </div>
                <div>
                  <label>WireGuard IP</label>
                  <input
                    value={form.wg_ip}
                    onChange={(e) => setForm({ ...form, wg_ip: e.target.value })}
                    placeholder="10.99.0.10"
                  />
                </div>
              </div>
              <label>Основной поток (main) — запись + полный экран</label>
              <input
                value={form.main_stream}
                onChange={(e) => setForm({ ...form, main_stream: e.target.value })}
                placeholder="rtsp://192.168.1.75:554/stream=0"
              />
              <label>Доп. поток (sub) — сетка + детекция</label>
              <input
                value={form.sub_stream}
                onChange={(e) => setForm({ ...form, sub_stream: e.target.value })}
                placeholder="rtsp://192.168.1.75:554/stream=1"
              />
              <label>RTSP URL (совместимость)</label>
              <input
                value={form.rtsp_url}
                onChange={(e) => setForm({ ...form, rtsp_url: e.target.value })}
                placeholder="rtsp://192.168.1.75:554/stream=0"
              />
              <div className="grid grid-2" style={{ gap: 10 }}>
                <div>
                  <label>Логин камеры</label>
                  <input
                    value={form.username}
                    onChange={(e) => setForm({ ...form, username: e.target.value })}
                    placeholder="root"
                  />
                </div>
                <div>
                  <label>Пароль камеры</label>
                  <input
                    type="password"
                    value={form.password}
                    onChange={(e) => setForm({ ...form, password: e.target.value })}
                    placeholder="••••••••"
                  />
                </div>
              </div>
              <div className="grid grid-2" style={{ gap: 10 }}>
                <div>
                  <label>MAC-адрес</label>
                  <input
                    value={form.mac}
                    onChange={(e) => setForm({ ...form, mac: e.target.value })}
                    placeholder="aa:bb:cc:dd:ee:ff"
                  />
                </div>
                <div>
                  <label>Версия прошивки</label>
                  <input
                    value={form.firmware}
                    onChange={(e) => setForm({ ...form, firmware: e.target.value })}
                    placeholder="SSC338Q"
                  />
                </div>
              </div>
              {formError && <p style={{ color: 'var(--danger)', marginBottom: 12, fontSize: 13 }}>{formError}</p>}
              <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 16 }}>
                <button type="button" className="btn btn-outline" onClick={() => setShowModal(false)}>
                  Отмена
                </button>
                <button type="submit" className="btn btn-primary" disabled={saving}>
                  {saving ? 'Добавление...' : 'Добавить'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
/**
 * CameraThumb — превью камеры в виде живого субпотока (HLS).
 * Субпоток лёгкий (обычно 640x360), поэтому его можно держать
 * сразу на нескольких карточках без заметной нагрузки.
 *
 * Если поток не поднялся за отведённое время, показываем статичный кадр —
 * так карточка не остаётся пустой, даже когда камера отдаёт поток медленно.
 */
function CameraThumb({ id, hlsUrl }: { id: string; hlsUrl: string }) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    const video = videoRef.current
    if (!video || failed) return

    const token = localStorage.getItem('token')
    if (!token) {
      setFailed(true)
      return
    }

    const u = new URL(hlsUrl, window.location.origin)
    u.searchParams.set('token', token)
    const src = u.pathname + u.search

    let hls: Hls | null = null
    let timer: number | undefined

    if (Hls.isSupported()) {
      hls = new Hls({
        // Превью в списке: минимизируем трафик и нагрузку на камеры.
        enableWorker: true,
        lowLatencyMode: false,
        backBufferLength: 10,
        maxBufferLength: 6,
        maxMaxBufferLength: 12,
        // Камера может отдавать поток с задержкой — не сдаёмся сразу.
        manifestLoadingMaxRetry: 3,
        manifestLoadingRetryDelay: 2000,
      })
      hls.loadSource(src)
      hls.attachMedia(video)
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {})
      })
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (data.fatal) setFailed(true)
      })
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = src
    } else {
      setFailed(true)
    }

    // Страховка: если за 12с кадр так и не пошёл — переключаемся на снапшот.
    timer = window.setTimeout(() => {
      if (!video.videoWidth) setFailed(true)
    }, 12000)

    return () => {
      if (timer) window.clearTimeout(timer)
      hls?.destroy()
    }
  }, [hlsUrl, failed])

  if (failed) return <SnapshotImage id={id} name="" />

  return (
    <video
      ref={videoRef}
      muted
      playsInline
      autoPlay
      style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 8, background: '#000' }}
    />
  )
}

/** SnapshotImage — статичный кадр с камеры (обновляется при монтировании). */
function SnapshotImage({ id, name }: { id: string; name: string }) {
  const [failed, setFailed] = useState(false)
  if (failed) return null

  const token = localStorage.getItem('token')
  const src = `/api/v1/cameras/${id}/snapshot${token ? `?jwt=${encodeURIComponent(token)}` : ''}`

  return (
    <img
      src={src}
      alt={name}
      style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 8 }}
      onError={() => setFailed(true)}
    />
  )
}
