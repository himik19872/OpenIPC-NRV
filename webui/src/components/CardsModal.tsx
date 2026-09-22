import { useEffect, useState } from 'react'
import { acsAPI, ACSCard, ACSController } from '../api/client'
import {
  Plus,
  Trash2,
  Pencil,
  RefreshCw,
  Download,
  Upload,
  Radio,
  X,
} from 'lucide-react'

// Управление картами доступа контроллера.
//
// Показаны два независимых списка:
//   - карты сервера — общий справочник, источник истины;
//   - карты контроллера — то, что реально лежит в памяти устройства.
//
// Они могут расходиться: карты, заведённые на устройстве в обход сервера,
// в серверном списке не появятся, пока их не перенесут кнопкой
// «Забрать с контроллера». Списки рядом, чтобы расхождение было видно.

interface Props {
  controller: ACSController
  onClose: () => void
}

export function CardsModal({ controller, onClose }: Props) {
  const [tab, setTab] = useState<'server' | 'device'>('server')
  const [serverCards, setServerCards] = useState<ACSCard[]>([])
  const [deviceCards, setDeviceCards] = useState<ACSCard[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  // Отдельный контроллер не поддерживает работу с картами: у вендорских
  // устройств карты живут только на самом приборе.
  const [unsupported, setUnsupported] = useState(false)

  // Режим обучения: контроллер запоминает номер поднесённой карты.
  const [learning, setLearning] = useState(false)
  const [learnName, setLearnName] = useState('')

  const [editing, setEditing] = useState<ACSCard | null>(null)
  const [form, setForm] = useState({
    facility: 0,
    card: 0,
    name: '',
    group: '',
    access: 0,
    active: true,
  })

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      // Серверный список обязателен, а список с контроллера может быть
      // недоступен — тогда показываем только серверный.
      const server = await acsAPI.listCards(controller.id)
      setServerCards(server.data || [])
    } catch {
      setError('Не удалось загрузить карты с сервера')
    }

    try {
      const device = await acsAPI.listDeviceCards(controller.id)
      setDeviceCards(device.data || [])
      setUnsupported(false)
    } catch (e: any) {
      const msg = e?.response?.data?.error || ''
      if (msg.includes('не поддерживает')) {
        setUnsupported(true)
      }
      setDeviceCards([])
    }

    try {
      const st = await acsAPI.cardLearnState(controller.id)
      setLearning(st.data.learning)
    } catch { /* состояние обучения не критично */ }

    setLoading(false)
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [controller.id])

  const resetForm = () => {
    setEditing(null)
    setForm({ facility: 0, card: 0, name: '', group: '', access: 0, active: true })
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy('save')
    setError('')
    setNotice('')

    const payload = {
      controller_id: controller.id,
      facility: form.facility,
      card: form.card,
      name: form.name,
      group: form.group,
      access: form.access,
      active: form.active,
    }

    try {
      if (editing?.id) {
        await acsAPI.updateCard(editing.id, payload)
      } else {
        await acsAPI.createCard(payload)
      }
      resetForm()
      await load()
    } catch (e: any) {
      const msg = e?.response?.data?.error
      // Карта может сохраниться на сервере и не дойти до контроллера.
      // Тогда это предупреждение, а не отказ.
      if (e?.response?.status === 202) {
        setNotice(msg || 'Карта сохранена на сервере, но не выдана на контроллер')
      } else {
        setError(msg || 'Не удалось сохранить карту')
      }
      await load()
    } finally {
      setBusy('')
    }
  }

  const handleDelete = async (card: ACSCard) => {
    if (!card.id) return
    if (!window.confirm(`Удалить карту ${card.facility}:${card.card}${card.name ? ` (${card.name})` : ''}?`)) {
      return
    }
    setBusy('delete-' + card.id)
    setError('')
    setNotice('')
    try {
      await acsAPI.deleteCard(card.id)
      await load()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось удалить карту')
      await load()
    } finally {
      setBusy('')
    }
  }

  const handleQuickToggle = async (card: ACSCard) => {
    if (!card.id) return
    setBusy('toggle-' + card.id)
    setError('')
    try {
      await acsAPI.updateCard(card.id, {
        controller_id: controller.id,
        facility: card.facility,
        card: card.card,
        name: card.name,
        group: card.group || '',
        access: card.access,
        active: !card.active,
      })
      await load()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось изменить карту')
      await load()
    } finally {
      setBusy('')
    }
  }

  const handleSync = async () => {
    setBusy('sync')
    setError('')
    setNotice('')
    try {
      const r = await acsAPI.syncCards(controller.id)
      setNotice(`Выдано карт на контроллер: ${r.data.cards}`)
      await load()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось выдать карты на контроллер')
    } finally {
      setBusy('')
    }
  }

  const handleImport = async () => {
    if (!window.confirm('Заменить карты на сервере картами с контроллера?')) return
    setBusy('import')
    setError('')
    setNotice('')
    try {
      const r = await acsAPI.importCards(controller.id)
      setNotice(`Перенесено карт с контроллера: ${r.data.cards}`)
      await load()
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось перенести карты')
    } finally {
      setBusy('')
    }
  }

  const handleLearn = async () => {
    if (!learnName.trim()) {
      setError('Укажите имя владельца карты')
      return
    }
    setBusy('learn')
    setError('')
    setNotice('')
    try {
      await acsAPI.startCardLearn(controller.id, learnName.trim())
      setLearning(true)
      setNotice(`Поднесите карту к считывателю. Она будет сохранена как «${learnName.trim()}», затем нажмите «Забрать с контроллера».`)
    } catch (e: any) {
      setError(e?.response?.data?.error || 'Не удалось включить режим обучения')
    } finally {
      setBusy('')
    }
  }

  const handleLearnCancel = async () => {
    setBusy('learn')
    try {
      await acsAPI.cancelCardLearn(controller.id)
      setLearning(false)
      setNotice('')
    } catch { /* ignore */ }
    finally {
      setBusy('')
    }
  }

  // Карты, которые есть на контроллере, но не заведены на сервере.
  const serverKeys = new Set(serverCards.map((c) => `${c.facility}:${c.card}`))
  const onlyOnDevice = deviceCards.filter((c) => !serverKeys.has(`${c.facility}:${c.card}`))

  const renderTable = (cards: ACSCard[], isDevice: boolean) => (
    <div className="table-wrap" style={{ maxHeight: 320, overflow: 'auto' }}>
      <table>
        <thead>
          <tr>
            <th>Карта</th>
            <th>Владелец</th>
            <th>Группа</th>
            <th>Доступ</th>
            <th>Статус</th>
            {!isDevice && <th style={{ width: 100 }} />}
          </tr>
        </thead>
        <tbody>
          {cards.length === 0 ? (
            <tr>
              <td colSpan={isDevice ? 5 : 6} style={{ textAlign: 'center', color: 'var(--text-secondary)', padding: 24 }}>
                {isDevice ? 'На контроллере нет карт' : 'Нет карт'}
              </td>
            </tr>
          ) : (
            cards.map((c, i) => {
              const onServer = serverKeys.has(`${c.facility}:${c.card}`)
              return (
                <tr key={c.id || `${c.facility}-${c.card}-${i}`}>
                  <td style={{ fontFamily: 'monospace', whiteSpace: 'nowrap' }}>
                    {c.facility}:{c.card}
                  </td>
                  <td>{c.name || <span style={{ color: 'var(--text-secondary)' }}>без имени</span>}</td>
                  <td style={{ color: 'var(--text-secondary)' }}>{c.group || '—'}</td>
                  <td style={{ fontSize: 13 }}>
                    {c.access === 1 ? 'По расписанию' : 'Постоянный'}
                  </td>
                  <td>
                    <span className={`badge badge-${c.active ? 'online' : 'offline'}`}>
                      <span className={`badge-dot badge-dot-${c.active ? 'online' : 'offline'}`} />
                      {c.active ? 'активна' : 'заблокирована'}
                    </span>
                    {isDevice && !onServer && (
                      <span className="badge badge-offline" style={{ marginLeft: 6 }} title="Карта есть на контроллере, но не заведена на сервере">
                        нет на сервере
                      </span>
                    )}
                  </td>
                  {!isDevice && (
                    <td>
                      <div style={{ display: 'flex', gap: 4 }}>
                        <button
                          className="btn btn-outline btn-sm"
                          title={c.active ? 'Заблокировать' : 'Разблокировать'}
                          disabled={busy === 'toggle-' + c.id}
                          onClick={() => handleQuickToggle(c)}
                        >
                          {c.active ? 'Блок' : 'Разблок'}
                        </button>
                        <button
                          className="btn btn-outline btn-sm"
                          title="Изменить"
                          onClick={() => {
                            setEditing(c)
                            setForm({
                              facility: c.facility,
                              card: c.card,
                              name: c.name || '',
                              group: c.group || '',
                              access: c.access,
                              active: c.active,
                            })
                          }}
                        >
                          <Pencil size={14} />
                        </button>
                        <button
                          className="btn btn-outline btn-sm"
                          title="Удалить"
                          disabled={busy === 'delete-' + c.id}
                          onClick={() => handleDelete(c)}
                        >
                          <Trash2 size={14} />
                        </button>
                      </div>
                    </td>
                  )}
                </tr>
              )
            })
          )}
        </tbody>
      </table>
    </div>
  )

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div
        className="modal"
        style={{ maxWidth: 900, width: '95%', maxHeight: '90vh', overflow: 'auto' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ marginBottom: 4 }}>Карты доступа</h2>
          <button className="btn btn-outline btn-sm" onClick={onClose}><X size={16} /></button>
        </div>
        <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 0 }}>
          {controller.name} — {controller.ip}:{controller.port}
        </p>

        {unsupported && (
          <div className="card" style={{ padding: 12, marginBottom: 12, borderLeft: '3px solid var(--warning)' }}>
            <div style={{ fontSize: 13 }}>
              Контроллер не поддерживает управление картами через сервер —
              список ниже содержит только серверный справочник.
            </div>
          </div>
        )}

        {error && (
          <div className="card" style={{ padding: 12, marginBottom: 12, borderLeft: '3px solid var(--danger)' }}>
            <div style={{ fontSize: 13, color: 'var(--danger)' }}>{error}</div>
          </div>
        )}
        {notice && (
          <div className="card" style={{ padding: 12, marginBottom: 12, borderLeft: '3px solid var(--success)' }}>
            <div style={{ fontSize: 13 }}>{notice}</div>
          </div>
        )}

        {/* Действия */}
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 16 }}>
          <button className="btn btn-outline btn-sm" onClick={load} disabled={loading}>
            <RefreshCw size={14} /> Обновить
          </button>
          {!unsupported && (
            <>
              <button className="btn btn-outline btn-sm" onClick={handleSync} disabled={busy === 'sync'}>
                <Upload size={14} /> Выдать на контроллер
              </button>
              <button className="btn btn-outline btn-sm" onClick={handleImport} disabled={busy === 'import'}>
                <Download size={14} /> Забрать с контроллера
              </button>
              {learning ? (
                <button className="btn btn-outline btn-sm" onClick={handleLearnCancel} disabled={busy === 'learn'}>
                  <X size={14} /> Отменить обучение
                </button>
              ) : (
                <button className="btn btn-outline btn-sm" onClick={handleLearn} disabled={busy === 'learn'}>
                  <Radio size={14} /> Обучить карту
                </button>
              )}
            </>
          )}
        </div>

        {!unsupported && (
          <div className="card" style={{ padding: 12, marginBottom: 16 }}>
            <label style={{ marginTop: 0 }}>Имя владельца для режима обучения</label>
            <div style={{ display: 'flex', gap: 8 }}>
              <input
                value={learnName}
                onChange={(e) => setLearnName(e.target.value)}
                placeholder="ФИО сотрудника"
              />
              <button className="btn btn-primary" onClick={handleLearn} disabled={busy === 'learn' || !learnName.trim()}>
                Включить
              </button>
            </div>
            {learning && (
              <div style={{ fontSize: 12, color: 'var(--warning)', marginTop: 8 }}>
                Контроллер ждёт карту. Поднесите её к считывателю.
              </div>
            )}
          </div>
        )}

        {/* Табы */}
        <div style={{ display: 'flex', gap: 4, marginBottom: 12 }}>
          <button
            className={`btn ${tab === 'server' ? 'btn-primary' : 'btn-outline'} btn-sm`}
            onClick={() => setTab('server')}
          >
            На сервере ({serverCards.length})
          </button>
          <button
            className={`btn ${tab === 'device' ? 'btn-primary' : 'btn-outline'} btn-sm`}
            onClick={() => setTab('device')}
          >
            На контроллере ({deviceCards.length})
          </button>
        </div>

        {loading ? (
          <div className="spinner" />
        ) : tab === 'server' ? (
          renderTable(serverCards, false)
        ) : (
          <>
            {onlyOnDevice.length > 0 && (
              <div style={{ fontSize: 13, color: 'var(--warning)', marginBottom: 8 }}>
                {onlyOnDevice.length} карт(ы) есть только на контроллере — их можно перенести на сервер кнопкой «Забрать с контроллера».
              </div>
            )}
            {renderTable(deviceCards, true)}
          </>
        )}

        {/* Форма карты */}
        <form onSubmit={handleSubmit} style={{ marginTop: 20, borderTop: '1px solid var(--border)', paddingTop: 16 }}>
          <h3 style={{ fontSize: 15, marginBottom: 12 }}>
            {editing ? 'Изменить карту' : 'Добавить карту'}
          </h3>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div>
              <label>Facility (0–255)</label>
              <input
                type="number"
                min={0}
                max={255}
                value={form.facility}
                onChange={(e) => setForm({ ...form, facility: +e.target.value })}
                required
              />
            </div>
            <div>
              <label>Номер карты (0–65535)</label>
              <input
                type="number"
                min={0}
                max={65535}
                value={form.card}
                onChange={(e) => setForm({ ...form, card: +e.target.value })}
                required
              />
            </div>
            <div>
              <label>Владелец</label>
              <input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="ФИО"
              />
            </div>
            <div>
              <label>Группа</label>
              <input
                value={form.group}
                onChange={(e) => setForm({ ...form, group: e.target.value })}
                placeholder="Отдел"
              />
            </div>
          </div>

          <label>Тип доступа</label>
          <select value={form.access} onChange={(e) => setForm({ ...form, access: +e.target.value })}>
            <option value={0}>Постоянный</option>
            <option value={1}>Только по расписанию</option>
          </select>

          <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 12 }}>
            <input
              type="checkbox"
              checked={form.active}
              onChange={(e) => setForm({ ...form, active: e.target.checked })}
              style={{ width: 'auto' }}
            />
            Карта активна
          </label>

          <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 16 }}>
            {editing && (
              <button type="button" className="btn btn-outline" onClick={resetForm}>
                Отмена
              </button>
            )}
            <button type="submit" className="btn btn-primary" disabled={busy === 'save'}>
              <Plus size={16} />
              {busy === 'save' ? 'Сохранение...' : editing ? 'Сохранить' : 'Добавить'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
