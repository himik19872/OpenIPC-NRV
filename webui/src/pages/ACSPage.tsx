import { useState } from 'react'
import { acsAPI, ACSController, ACSEvent } from '../api/client'
import { useAsync } from '../hooks/useApi'
import { Plus, Shield, DoorOpen, Unlock, RefreshCw } from 'lucide-react'

export default function ACSPage() {
  const [tab, setTab] = useState<'controllers' | 'events'>('controllers')

  const {
    data: controllers,
    loading: ctrlLoading,
    refetch: refetchCtrl,
  } = useAsync<ACSController[]>(() => acsAPI.listControllers())

  const {
    data: eventsData,
    loading: eventsLoading,
    refetch: refetchEvents,
  } = useAsync<any>(() => acsAPI.listEvents({ page_size: 50 }))

  const acsEvents: ACSEvent[] = eventsData?.events || []

  const [showModal, setShowModal] = useState(false)
  const [form, setForm] = useState({ name: '', vendor: 'hikvision', ip: '', port: 80, login: '', password: '' })
  const [saving, setSaving] = useState(false)

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await acsAPI.createController(form)
      setShowModal(false)
      setForm({ name: '', vendor: 'hikvision', ip: '', port: 80, login: '', password: '' })
      refetchCtrl()
    } catch { /* ignore */ } finally {
      setSaving(false)
    }
  }

  const handleOpenDoor = async (ctrlID: string, doorID: string) => {
    try {
      await acsAPI.openDoor(ctrlID, doorID)
    } catch { /* ignore */ }
  }

  const eventLabels: Record<string, string> = {
    access_granted: 'Доступ разрешён',
    access_denied: 'Доступ запрещён',
    door_forced: 'Взлом двери',
  }

  const eventColors: Record<string, string> = {
    access_granted: 'var(--success)',
    access_denied: 'var(--danger)',
    door_forced: 'var(--warning)',
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>СКУД</h1>
          <p>Управление контроллерами доступа</p>
        </div>
        <button className="btn btn-primary" onClick={() => setShowModal(true)}>
          <Plus size={18} />
          Добавить контроллер
        </button>
      </div>

      {/* Табы */}
      <div style={{ display: 'flex', gap: 4, marginBottom: 24 }}>
        <button
          className={`btn ${tab === 'controllers' ? 'btn-primary' : 'btn-outline'} btn-sm`}
          onClick={() => setTab('controllers')}
        >
          Контроллеры
        </button>
        <button
          className={`btn ${tab === 'events' ? 'btn-primary' : 'btn-outline'} btn-sm`}
          onClick={() => setTab('events')}
        >
          События
        </button>
      </div>

      {tab === 'controllers' && (
        <>
          {ctrlLoading ? <div className="spinner" /> : !controllers || controllers.length === 0 ? (
            <div className="card" style={{ textAlign: 'center', padding: 60, color: 'var(--text-secondary)' }}>
              <Shield size={48} style={{ marginBottom: 16, opacity: 0.3 }} />
              <p>Нет добавленных контроллеров СКУД</p>
            </div>
          ) : (
            <div className="grid grid-2">
              {controllers.map((ctrl) => (
                <div key={ctrl.id} className="card">
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                    <h3 style={{ fontSize: 16 }}>{ctrl.name}</h3>
                    <span className={`badge badge-${ctrl.status === 'online' ? 'online' : 'offline'}`}>
                      <span className={`badge-dot badge-dot-${ctrl.status === 'online' ? 'online' : 'offline'}`} />
                      {ctrl.status}
                    </span>
                  </div>
                  <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 12 }}>
                    <div>Производитель: <strong>{ctrl.vendor}</strong></div>
                    <div>IP: {ctrl.ip}:{ctrl.port}</div>
                    <div>Добавлен: {new Date(ctrl.created_at).toLocaleDateString('ru')}</div>
                  </div>
                  <button
                    className="btn btn-outline btn-sm"
                    onClick={() => handleOpenDoor(ctrl.id, 'door_1')}
                  >
                    <Unlock size={14} />
                    Открыть дверь
                  </button>
                </div>
              ))}
            </div>
          )}
        </>
      )}

      {tab === 'events' && (
        <>
          {eventsLoading ? <div className="spinner" /> : acsEvents.length === 0 ? (
            <div className="card" style={{ textAlign: 'center', padding: 60, color: 'var(--text-secondary)' }}>
              <DoorOpen size={48} style={{ marginBottom: 16, opacity: 0.3 }} />
              <p>Нет событий СКУД</p>
            </div>
          ) : (
            <div className="card" style={{ padding: 0 }}>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Время</th>
                      <th>Событие</th>
                      <th>Дверь</th>
                      <th>Карта</th>
                    </tr>
                  </thead>
                  <tbody>
                    {acsEvents.map((ev) => (
                      <tr key={ev.id}>
                        <td style={{ whiteSpace: 'nowrap', fontSize: 13 }}>
                          {new Date(ev.timestamp).toLocaleString('ru')}
                        </td>
                        <td>
                          <span style={{ color: eventColors[ev.event_type] || 'var(--text-primary)' }}>
                            {eventLabels[ev.event_type] || ev.event_type}
                          </span>
                        </td>
                        <td style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{ev.door_id}</td>
                        <td style={{ fontSize: 13, fontFamily: 'monospace' }}>{ev.card_number || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </>
      )}

      {/* Модальное окно добавления контроллера */}
      {showModal && (
        <div className="modal-overlay" onClick={() => setShowModal(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>Добавить контроллер СКУД</h2>
            <form onSubmit={handleAdd}>
              <label>Название</label>
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />

              <label>Производитель</label>
              <select value={form.vendor} onChange={(e) => setForm({ ...form, vendor: e.target.value })}>
                <option value="hikvision">Hikvision</option>
                <option value="dahua">Dahua</option>
                <option value="promwad">Promwad</option>
                <option value="skud">SKUD (ESP32-P4)</option>
              </select>

              <label>IP-адрес</label>
              <input value={form.ip} onChange={(e) => setForm({ ...form, ip: e.target.value })} placeholder="192.168.1.100" required />

              <label>Порт</label>
              <input type="number" value={form.port} onChange={(e) => setForm({ ...form, port: +e.target.value })} />

              <label>Логин</label>
              <input value={form.login} onChange={(e) => setForm({ ...form, login: e.target.value })} required />

              <label>Пароль</label>
              <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required />

              <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 8 }}>
                <button type="button" className="btn btn-outline" onClick={() => setShowModal(false)}>Отмена</button>
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