import { useState, useEffect, useCallback } from 'react'
import { audioAPI, type AudioSettings, type AudioStatus } from '../api/client'
import { useToast } from '../context/ToastContext'
import TalkPanel from './TalkPanel'
import { Volume2, VolumeX, Mic, MicOff, Loader2, Activity, Save } from 'lucide-react'

interface Props {
  cameraId: string
}

/**
 * Панель настроек звука камеры.
 *
 * Показывает состояние звуковой дорожки (какой кодек отдаёт камера и идёт ли
 * перекодирование) и позволяет включить/выключить звук, задать громкость
 * и подготовить детекцию звуковых событий.
 */
export default function AudioSettingsPanel({ cameraId }: Props) {
  const toast = useToast()
  const [settings, setSettings] = useState<AudioSettings | null>(null)
  const [status, setStatus] = useState<AudioStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)

  const load = useCallback(async () => {
    try {
      // Настройки и статус запрашиваем параллельно: это независимые данные,
      // и последовательные запросы вдвое замедляют открытие вкладки.
      const [s, st] = await Promise.all([
        audioAPI.getSettings(cameraId),
        audioAPI.status(cameraId).catch(() => null),
      ])
      setSettings(s.data)
      setStatus(st ? st.data : null)
      setDirty(false)
    } catch {
      toast.error('Не удалось загрузить настройки звука')
    } finally {
      setLoading(false)
    }
  }, [cameraId, toast])

  useEffect(() => { load() }, [load])

  const patch = (p: Partial<AudioSettings>) => {
    setSettings((prev) => (prev ? { ...prev, ...p } : prev))
    setDirty(true)
  }

  const save = async () => {
    if (!settings) return
    setSaving(true)
    try {
      const updated = await audioAPI.updateSettings(cameraId, {
        has_microphone: settings.has_microphone,
        enabled: settings.enabled,
        volume: settings.volume,
        source_codec: settings.source_codec,
        transcode: settings.transcode,
        detect_audio: settings.detect_audio,
        audio_events: settings.audio_events,
        audio_threshold: settings.audio_threshold,
        speaker_enabled: settings.speaker_enabled,
        speaker_codec: settings.speaker_codec,
      })
      setSettings(updated.data)
      setDirty(false)
      toast.success('Настройки звука сохранены')
      // Статус мог измениться (запустилось или остановилось перекодирование),
      // поэтому перечитываем его после сохранения.
      audioAPI.status(cameraId).then((r) => setStatus(r.data)).catch(() => {})
    } catch {
      toast.error('Не удалось сохранить настройки звука')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-secondary)' }}>
        <Loader2 size={20} className="spin" /> Загрузка настроек звука...
      </div>
    )
  }

  if (!settings) {
    return <p style={{ color: 'var(--text-secondary)' }}>Настройки звука недоступны</p>
  }

  const codecLabel: Record<string, string> = {
    pcm_alaw: 'G.711 A-law',
    pcm_mulaw: 'G.711 µ-law',
    g711: 'G.711',
    opus: 'Opus',
    aac: 'AAC',
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 }}>
        <h3 style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 15, margin: 0 }}>
          <Volume2 size={18} style={{ color: 'var(--accent)' }} />
          Звук с камеры
        </h3>
        <button className="btn btn-primary btn-sm" onClick={save} disabled={!dirty || saving}>
          {saving ? <Loader2 size={14} className="spin" /> : <Save size={14} />}
          Сохранить
        </button>
      </div>

      {/* Состояние потока — важнее всего для диагностики: показывает, какой
          кодек отдаёт камера и работает ли перекодирование в AAC. */}
      <div style={{ marginBottom: 20, padding: 12, background: 'rgba(0,0,0,0.02)', borderRadius: 8 }}>
        <StatusRow
          label="Дорожка в потоке"
          value={status?.available
            ? <span style={{ color: 'var(--success)' }}>есть</span>
            : <span style={{ color: 'var(--text-secondary)' }}>нет</span>}
        />
        <StatusRow
          label="Кодек камеры"
          value={<span style={{ fontFamily: 'monospace' }}>
            {status?.codec ? (codecLabel[status.codec] || status.codec) : '—'}
          </span>}
        />
        <StatusRow
          label="Перекодирование"
          value={status?.transcoding
            ? <span style={{ display: 'flex', alignItems: 'center', gap: 6, color: 'var(--success)' }}>
                <Activity size={14} /> идёт (G.711 → AAC)
              </span>
            : <span style={{ color: 'var(--text-secondary)' }}>
                {settings.transcode ? 'остановлено' : 'не требуется'}
              </span>}
        />
        <StatusRow
          label="Звук в HLS"
          value={status?.hls_has_audio
            ? <span style={{ color: 'var(--success)' }}>воспроизводится</span>
            : <span style={{ color: 'var(--warning)' }}>недоступен</span>}
        />
      </div>

      {/* Основные переключатели */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 20 }}>
        <Toggle
          label="У микрофона есть звук"
          hint="Снимите галочку, если у камеры нет микрофона — тогда звук не будет запускаться"
          checked={settings.has_microphone}
          onChange={(v) => patch({ has_microphone: v })}
          icon={settings.has_microphone ? <Mic size={16} /> : <MicOff size={16} />}
        />
        <Toggle
          label="Включить звук"
          hint="Управляет публикацией звуковой дорожки для просмотра"
          checked={settings.enabled}
          onChange={(v) => patch({ enabled: v })}
          icon={settings.enabled ? <Volume2 size={16} /> : <VolumeX size={16} />}
        />
        <Toggle
          label="Перекодировать звук в AAC"
          hint="Обязательно для G.711: браузеры не воспроизводят его в HLS. Для Opus/AAC можно выключить"
          checked={settings.transcode}
          onChange={(v) => patch({ transcode: v })}
        />
      </div>

      {/* Громкость */}
      <div style={{ marginBottom: 20 }}>
        <label style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 6 }}>
          <span>Громкость по умолчанию</span>
          <span style={{ color: 'var(--text-secondary)' }}>{Math.round(settings.volume * 100)}%</span>
        </label>
        <input
          type="range" min={0} max={1} step={0.05}
          value={settings.volume}
          onChange={(e) => patch({ volume: Number(e.target.value) })}
          style={{ width: '100%' }}
        />
      </div>

      {/* Детекция звука */}
      <h4 style={{ fontSize: 14, marginBottom: 10, paddingTop: 14, borderTop: '1px solid var(--border)' }}>
        Детекция звуковых событий
      </h4>
      <Toggle
        label="Распознавать звуковые события"
        hint="Крик, выстрел, разбитое стекло, лай собаки и другие — появятся в разделе событий"
        checked={settings.detect_audio}
        onChange={(v) => patch({ detect_audio: v })}
      />

      <div style={{ marginTop: 12, marginBottom: 16 }}>
        <label style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 6 }}>
          <span>Порог срабатывания</span>
          <span style={{ color: 'var(--text-secondary)' }}>{Math.round(settings.audio_threshold * 100)}%</span>
        </label>
        <input
          type="range" min={0} max={1} step={0.05}
          value={settings.audio_threshold}
          onChange={(e) => patch({ audio_threshold: Number(e.target.value) })}
          disabled={!settings.detect_audio}
          style={{ width: '100%' }}
        />
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 4 }}>
          Ниже порога — тише, выше — меньше ложных срабатываний
        </p>
      </div>

      {/* Обратная связь (динамик камеры) */}
      <h4 style={{ fontSize: 14, marginBottom: 10, paddingTop: 14, borderTop: '1px solid var(--border)' }}>
        Обратная связь (динамик камеры)
      </h4>
      <Toggle
        label="Разрешить передачу звука на камеру"
        hint="Двусторонняя связь: оператор говорит через микрофон, звук идёт на динамик камеры"
        checked={settings.speaker_enabled}
        onChange={(v) => patch({ speaker_enabled: v })}
      />
      {settings.speaker_enabled && (
        <div style={{ marginTop: 12 }}>
          <label style={{ display: 'block', fontSize: 13, marginBottom: 6 }}>Кодек динамика</label>
          <select
            className="input"
            value={settings.speaker_codec}
            onChange={(e) => patch({ speaker_codec: e.target.value as AudioSettings['speaker_codec'] })}
            style={{ maxWidth: 240 }}
          >
            <option value="g711">G.711 (совместим почти со всеми камерами)</option>
            <option value="aac">AAC (лучше качество, поддерживают не все)</option>
          </select>
        </div>
      )}

      {/* Панель разговора: показывается только при включённой настройке.
          Если камера не умеет принимать звук, панель объяснит это вместо
          неработающей кнопки. */}
      {settings.speaker_enabled && (
        <div style={{ marginTop: 14, paddingTop: 14, borderTop: '1px solid var(--border)' }}>
          <TalkPanel
            cameraId={cameraId}
            backchannel={!!status?.backchannel}
            speakerEnabled={settings.speaker_enabled}
          />
        </div>
      )}
      <p style={{ fontSize: 12, color: 'var(--text-secondary)', marginTop: 10 }}>
        Возможность обратного канала определяется автоматически по ответу
        камеры на RTSP-запрос: если прошивка не поддерживает приём звука,
        кнопка разговора не появится.
      </p>
    </div>
  )
}

/** Строка «показатель — значение» в блоке состояния. */
function StatusRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={{
      display: 'flex', justifyContent: 'space-between', alignItems: 'center',
      padding: '6px 0', borderBottom: '1px solid var(--border)', fontSize: 13,
    }}>
      <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
      <span>{value}</span>
    </div>
  )
}

/** Переключатель с подписью и пояснением под ней. */
function Toggle({
  label, hint, checked, onChange, icon,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (v: boolean) => void
  icon?: React.ReactNode
}) {
  return (
    <label style={{ display: 'flex', alignItems: 'flex-start', gap: 10, cursor: 'pointer' }}>
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        style={{ marginTop: 3, width: 16, height: 16, accentColor: 'var(--accent)', flexShrink: 0 }}
      />
      <span>
        <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 14 }}>
          {icon}
          {label}
        </span>
        {hint && (
          <span style={{ display: 'block', fontSize: 12, color: 'var(--text-secondary)', marginTop: 2 }}>
            {hint}
          </span>
        )}
      </span>
    </label>
  )
}
