import { useEffect, useRef, useState } from 'react'
import {
  acsAPI,
  ACSController,
  FirmwareImage,
  FirmwareInfo,
  OTAUpdate,
} from '../api/client'
import { Upload, Download, Trash2, RefreshCw, X, AlertTriangle, CheckCircle } from 'lucide-react'

// Обновление прошивки контроллера СКУД по OTA.
//
// Процесс долгий: образ передаётся на устройство, оно перезагружается и
// не отвечает, пока не поднимется. Поэтому состояние опрашивается с
// паузами, а не ожидается одним запросом.

interface Props {
  controller: ACSController
  onClose: () => void
}

export function FirmwareModal({ controller, onClose }: Props) {
  const [images, setImages] = useState<FirmwareImage[]>([])
  const [current, setCurrent] = useState<FirmwareInfo | null>(null)
  const [update, setUpdate] = useState<OTAUpdate | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [selected, setSelected] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)
  const pollRef = useRef<number | null>(null)

  const load = async () => {
    try {
      const list = await acsAPI.listFirmwares()
      setImages(list.data || [])
    } catch {
      setError('Не удалось получить список прошивок')
    }

    try {
      const info = await acsAPI.getFirmwareVersion(controller.id)
      setCurrent(info.data)
    } catch {
      // Версия недоступна, если контроллер офлайн или прошивка старая.
      setCurrent(null)
    }

    try {
      const u = await acsAPI.getUpdateState(controller.id)
      if (u.data && u.data.state !== 'idle') {
        setUpdate(u.data)
      }
    } catch { /* состояния может не быть */ }

    setLoading(false)
  }

  useEffect(() => {
    load()
    return () => {
      if (pollRef.current) window.clearInterval(pollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [controller.id])

  // Опрос состояния, пока обновление идёт.
  useEffect(() => {
    if (update?.state !== 'running') {
      if (pollRef.current) {
        window.clearInterval(pollRef.current)
        pollRef.current = null
      }
      return
    }
    if (pollRef.current) return

    pollRef.current = window.setInterval(async () => {
      try {
        const r = await acsAPI.getUpdateState(controller.id)
        if (r.data && r.data.state !== 'idle') {
          setUpdate(r.data)
          if (r.data.state !== 'running') {
            // Обновление закончилось — обновляем версию на экране.
            load()
          }
        }
      } catch { /* пропускаем неудачный опрос */ }
    }, 3000)

    return () => {
      if (pollRef.current) {
        window.clearInterval(pollRef.current)
        pollRef.current = null
      }
    }
  }, [update?.state, controller.id])

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setBusy('upload')
    setError('')
    try {
      // Тело запроса — сам файл, имя передаётся заголовком: так образ
      // уходит одним потоком без multipart-обёртки.
      await acsAPI.uploadFirmware(file)
      if (fileRef.current) fileRef.current.value = ''
      await load()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось загрузить прошивку')
    } finally {
      setBusy('')
    }
  }

  const handleDelete = async (name: string) => {
    if (!window.confirm(`Удалить прошивку ${name}?`)) return
    setBusy('del-' + name)
    setError('')
    try {
      await acsAPI.deleteFirmware(name)
      await load()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось удалить прошивку')
    } finally {
      setBusy('')
    }
  }

  const handleInstall = async () => {
    if (!selected) return
    if (!window.confirm(
      `Прошить контроллер «${controller.name}» прошивкой ${selected}?\n\n` +
      'Устройство перезагрузится и будет недоступно около минуты.'
    )) return

    setBusy('install')
    setError('')
    try {
      const r = await acsAPI.startFirmwareUpdate(controller.id, selected)
      setUpdate(r.data)
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось запустить обновление')
    } finally {
      setBusy('')
    }
  }

  const running = update?.state === 'running'

  const stepLabel = (step?: string) => {
    switch (step) {
      case 'upload': return 'Передача образа на контроллер'
      case 'reboot': return 'Контроллер перезагружается'
      case 'verify': return 'Проверка версии после обновления'
      case 'done': return 'Обновление завершено'
      default: return step || ''
    }
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal"
        style={{ maxWidth: 720, maxHeight: '90vh', overflow: 'auto' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ marginBottom: 4 }}>Прошивка контроллера</h2>
          <button className="btn btn-outline btn-sm" onClick={onClose}><X size={16} /></button>
        </div>
        <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 0 }}>
          {controller.name} — {controller.ip}
        </p>

        {error && (
          <div className="card" style={{ padding: 12, marginBottom: 12, borderLeft: '3px solid var(--danger)' }}>
            <div style={{ fontSize: 13, color: 'var(--danger)' }}>{error}</div>
          </div>
        )}

        {/* Установленная версия */}
        <div className="card" style={{ padding: 12, marginBottom: 16 }}>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>Установлено на контроллере</div>
          {loading ? (
            <div className="spinner" />
          ) : current?.version ? (
            <div style={{ fontSize: 15 }}>
              <strong>Версия {current.version}</strong>
              {current.build && (
                <span style={{ fontSize: 12, color: 'var(--text-secondary)', marginLeft: 8 }}>
                  сборка {current.build}
                </span>
              )}
            </div>
          ) : (
            <div style={{ fontSize: 13 }}>
              <span style={{ color: 'var(--warning)' }}>Версия не определяется.</span>{' '}
              На контроллере прошивка, не сообщающая версию, либо он недоступен.
              Проверить результат OTA по версии не получится — после обновления
              версия должна появиться.
            </div>
          )}
        </div>

        {/* Ход обновления */}
        {update && (
          <div
            className="card"
            style={{
              padding: 12,
              marginBottom: 16,
              borderLeft: `3px solid ${
                update.state === 'failed' ? 'var(--danger)'
                : update.state === 'done' ? 'var(--success)'
                : 'var(--warning)'
              }`,
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 14 }}>
              {update.state === 'running' && <RefreshCw size={16} className="spin" />}
              {update.state === 'done' && <CheckCircle size={16} style={{ color: 'var(--success)' }} />}
              {update.state === 'failed' && <AlertTriangle size={16} style={{ color: 'var(--danger)' }} />}
              <strong>
                {update.state === 'running' ? 'Обновление выполняется'
                  : update.state === 'done' ? 'Обновление завершено'
                  : 'Обновление не удалось'}
              </strong>
            </div>
            {running && (
              <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 6 }}>
                {stepLabel(update.step)}
              </div>
            )}
            <div style={{ fontSize: 13, marginTop: 6 }}>{update.message}</div>
            {(update.from_version || update.to_version) && (
              <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 6 }}>
                {update.from_version && <>было: {update.from_version || '—'} </>}
                {update.to_version && <>→ стало: {update.to_version}</>}
              </div>
            )}
          </div>
        )}

        {/* Загрузка образа */}
        <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
          <input
            ref={fileRef}
            type="file"
            accept=".bin"
            style={{ display: 'none' }}
            onChange={handleUpload}
          />
          <button
            className="btn btn-outline btn-sm"
            onClick={() => fileRef.current?.click()}
            disabled={busy === 'upload'}
          >
            <Upload size={14} />
            {busy === 'upload' ? 'Загрузка...' : 'Загрузить образ .bin'}
          </button>
          <button className="btn btn-outline btn-sm" onClick={load} disabled={loading}>
            <RefreshCw size={14} /> Обновить
          </button>
        </div>

        {/* Список образов */}
        <div className="table-wrap" style={{ maxHeight: 260, overflow: 'auto' }}>
          <table>
            <thead>
              <tr>
                <th style={{ width: 40 }} />
                <th>Прошивка</th>
                <th>Версия</th>
                <th>Размер</th>
                <th style={{ width: 50 }} />
              </tr>
            </thead>
            <tbody>
              {images.length === 0 ? (
                <tr>
                  <td colSpan={5} style={{ textAlign: 'center', color: 'var(--text-secondary)', padding: 24 }}>
                    Нет загруженных прошивок. Загрузите файл .bin, собранный из проекта прошивки.
                  </td>
                </tr>
              ) : (
                images.map((img) => (
                  <tr key={img.name}>
                    <td>
                      <input
                        type="radio"
                        name="firmware"
                        checked={selected === img.name}
                        onChange={() => setSelected(img.name)}
                      />
                    </td>
                    <td style={{ fontSize: 13 }}>{img.name}</td>
                    <td style={{ fontSize: 13 }}>{img.version || '—'}</td>
                    <td style={{ fontSize: 13, whiteSpace: 'nowrap' }}>
                      {(img.size / 1024).toFixed(0)} КБ
                    </td>
                    <td>
                      <button
                        className="btn btn-outline btn-sm"
                        title="Удалить образ"
                        disabled={busy === 'del-' + img.name || running}
                        onClick={() => handleDelete(img.name)}
                      >
                        <Trash2 size={14} />
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 16 }}>
          <button type="button" className="btn btn-outline" onClick={onClose}>Закрыть</button>
          <button
            className="btn btn-primary"
            onClick={handleInstall}
            disabled={!selected || running || busy === 'install'}
          >
            <Download size={16} />
            {running ? 'Обновление идёт...' : 'Прошить контроллер'}
          </button>
        </div>

        <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 12 }}>
          Во время прошивки контроллер не управляет замком: обновление
          прерывает его работу. Выполняйте его, когда проход не нужен.
        </div>
      </div>
    </div>
  )
}
