import { useEffect, useRef, useCallback, useState } from 'react'
import Hls from 'hls.js'

interface LivePlayerProps {
  hlsUrl?: string
  webrtcUrl?: string
  poster?: string
  muted?: boolean
  autoPlay?: boolean
  className?: string
  /**
   * URL отдельного HLS-потока со звуком.
   *
   * Звук идёт отдельно от видео, потому что камеры отдают его в G.711,
   * который браузеры не воспроизводят в HLS. Бэкенд перекодирует звук
   * в AAC и публикует отдельным потоком (<cameraID>_audio).
   */
  audioUrl?: string
  /** Начальная громкость 0..1 */
  volume?: number
  /** Обработчик недоступности звука — камера без микрофона */
  onAudioUnavailable?: () => void
}

export default function LivePlayer({
  hlsUrl,
  poster,
  muted = true,
  autoPlay = true,
  className = '',
  audioUrl,
  volume = 0.7,
  onAudioUnavailable,
}: LivePlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  // Элемент для звука: отдельный <video>, скрытый визуально (см. JSX).
  const audioElRef = useRef<HTMLVideoElement>(null)
  const hlsRef = useRef<Hls | null>(null)
  const audioHlsRef = useRef<Hls | null>(null)
  const [status, setStatus] = useState<'connecting' | 'playing' | 'error' | 'idle'>('idle')
  const [retryCount, setRetryCount] = useState(0)
  // Звук выключен по умолчанию: оператор включает его сам. Это ожидаемое
  // поведение для видеонаблюдения — иначе звук с десятков камер мешает.
  const [soundOn, setSoundOn] = useState(!muted)
  // Доступен ли аудиопоток: если камеры без микрофона, кнопку не показываем.
  const [audioAvailable, setAudioAvailable] = useState(false)
  const [audioReady, setAudioReady] = useState(false)

  // Колбэк в ref: родитель обычно передаёт новую функцию на каждый рендер,
  // и если положить её прямо в зависимости эффекта, HLS-инстанс будет
  // пересоздаваться при каждом обновлении страницы, обрывая звук.
  const onAudioUnavailableRef = useRef(onAudioUnavailable)
  onAudioUnavailableRef.current = onAudioUnavailable
  // Сколько раз перезагружали аудиоплейлист. Ограничение нужно, чтобы при
  // действительно отсутствующем звуке не было бесконечного цикла запросов.
  const audioReloadsRef = useRef(0)

  /** Добавляет JWT-токен в query: hls.js не умеет слать заголовок Authorization. */
  const withToken = useCallback((url: string) => {
    try {
      const token = localStorage.getItem('token')
      if (!token) return url
      const u = new URL(url, window.location.origin)
      u.searchParams.set('token', token)
      return u.pathname + u.search
    } catch {
      return url
    }
  }, [])

  const initHls = useCallback(() => {
    const video = videoRef.current
    if (!video || !hlsUrl) return

    const authedUrl = withToken(hlsUrl)

    if (hlsRef.current) {
      hlsRef.current.destroy()
      hlsRef.current = null
    }

    setStatus('connecting')

    if (Hls.isSupported()) {
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
        backBufferLength: 90,
        maxBufferLength: 30,
        maxMaxBufferLength: 60,
      })

      hls.loadSource(authedUrl)
      hls.attachMedia(video)

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        if (autoPlay) {
          video.play().catch(() => {
            // Автовоспроизведение может быть заблокировано браузером
            video.muted = true
            video.play().catch(() => {})
          })
        }
        setStatus('playing')
        setRetryCount(0)
      })

      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) {
          switch (data.type) {
            case Hls.ErrorTypes.NETWORK_ERROR:
              setStatus('error')
              if (retryCount < 5) {
                setRetryCount((c) => c + 1)
                setTimeout(() => {
                  hls.loadSource(authedUrl)
                }, 2000 * (retryCount + 1))
              }
              break
            case Hls.ErrorTypes.MEDIA_ERROR:
              setStatus('error')
              hls.recoverMediaError()
              break
            default:
              hls.destroy()
              setStatus('error')
              break
          }
        }
      })

      hlsRef.current = hls
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      // Safari — нативная поддержка HLS
      video.src = authedUrl
      video.addEventListener('loadedmetadata', () => {
        if (autoPlay) video.play().catch(() => {})
        setStatus('playing')
      })
      video.addEventListener('error', () => setStatus('error'))
    } else {
      setStatus('error')
    }
  }, [hlsUrl, autoPlay, retryCount, withToken])

  useEffect(() => {
    initHls()
    return () => {
      if (hlsRef.current) {
        hlsRef.current.destroy()
        hlsRef.current = null
      }
    }
  }, [initHls])

  /**
   * Подключает отдельный аудиопоток.
   *
   * Звук воспроизводится отдельным элементом <audio>, а не дорожкой внутри
   * видео: MediaMTX не умеет добавить AAC в уже существующий HLS-поток
   * с G.711, поэтому звук живёт своим потоком.
   */
  useEffect(() => {
    const audio = audioElRef.current
    if (!audio || !audioUrl) {
      setAudioAvailable(false)
      return
    }

    const authedUrl = withToken(audioUrl)
    setAudioAvailable(false)
    setAudioReady(false)
    audioReloadsRef.current = 0

    let giveUpTimer: ReturnType<typeof setTimeout> | null = null

    if (Hls.isSupported()) {
      const hls = new Hls({
        enableWorker: true,
        lowLatencyMode: true,
        backBufferLength: 30,
        maxBufferLength: 15,
      })

      hls.loadSource(authedUrl)
      hls.attachMedia(audio)

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        setAudioAvailable(true)
        setAudioReady(true)
        audio.volume = volume
        // Запускаем сразу, но без звука: при включении кнопки звук появится
        // без задержки на запуск потока.
        audio.muted = !soundOn
        audio.play().catch(() => { /* включат по кнопке */ })
      })

      hls.on(Hls.Events.LEVEL_LOADED, () => {
        // Плейлист загрузился — аудиопоток точно существует.
        // Снимаем возможный таймер «звука нет» и показываем кнопку.
        if (giveUpTimer) {
          clearTimeout(giveUpTimer)
          giveUpTimer = null
        }
        setAudioAvailable(true)
      })

      hls.on(Hls.Events.ERROR, (_e, data) => {
        // Аудиоканал считается живым, если плейлист хотя бы раз разобрался.
        // После этого ошибки — обычные сетевые сбои (оторвался сегмент,
        // буфер), а не признак отсутствия микрофона.
        setAudioAvailable((was) => {
          if (was) return true
          if (data.fatal) {
            // Ошибки восстановимого типа НЕ считаем признаком отсутствия звука:
            // при старте потока hls.js почти всегда ловит одну-две таких,
            // хотя звук в итоге играет.
            const recoverable =
              data.type === Hls.ErrorTypes.MEDIA_ERROR ||
              data.details === Hls.ErrorDetails.BUFFER_STALLED_ERROR ||
              data.details === Hls.ErrorDetails.BUFFER_APPENDING_ERROR ||
              data.details === Hls.ErrorDetails.BUFFER_ADD_CODEC_ERROR
            if (!recoverable && !giveUpTimer) {
              // Похоже, аудиопотока действительно нет (камера без микрофона
              // или ffmpeg ещё не опубликовал дорожку). Даём время, потом
              // убираем кнопку звука.
              giveUpTimer = setTimeout(() => {
                setAudioAvailable(false)
                onAudioUnavailableRef.current?.()
              }, 20000)
            }
          }
          return false
        })

        if (!data.fatal) return

        // Ошибки медиа-типа лечим без перезагрузки потока.
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          hls.recoverMediaError()
          return
        }

        // Сессия MediaMTX — это точка входа в плейлист, и она указывает на
        // сегменты, актуальные на момент создания. Если сессия «провисла»
        // (её сегменты уже удалены), звук не стартует: прокси отвечает 404.
        //
        // Перезагружаем МАСТЕР-плейлист с кэш-бастером: только он возвращает
        // НОВУЮ сессию. Без параметра запроса hls.js/browser отдадут
        // закешированный index.m3u8 со старой сессией, и цикл повторится.
        if (audioReloadsRef.current < 3) {
          audioReloadsRef.current += 1
          const sep = authedUrl.includes('?') ? '&' : '?'
          hls.loadSource(`${authedUrl}${sep}_r=${audioReloadsRef.current}`)
        }
      })

      audioHlsRef.current = hls
    } else if (audio.canPlayType('application/vnd.apple.mpegurl')) {
      audio.src = authedUrl
      audio.addEventListener('loadedmetadata', () => {
        setAudioAvailable(true)
        setAudioReady(true)
        audio.volume = volume
        audio.muted = !soundOn
        audio.play().catch(() => {})
      })
    }

    return () => {
      if (giveUpTimer) clearTimeout(giveUpTimer)
      if (audioHlsRef.current) {
        audioHlsRef.current.destroy()
        audioHlsRef.current = null
      }
    }
  }, [audioUrl, volume, withToken])

  // Видео всегда без звука: звук идёт отдельным потоком, иначе он задваивался бы.
  useEffect(() => {
    const video = videoRef.current
    if (video) {
      video.muted = true
      // Громкость обнуляем: некоторые браузеры игнорируют muted, если
      // громкость выставлена, и звук из видео дублировал бы аудиопоток.
      video.volume = 0
    }
    const audio = audioElRef.current
    if (!audio) return
    audio.muted = !soundOn
    if (soundOn) {
      audio.play().catch(() => {
        // Браузер может заблокировать звук без действия пользователя —
        // возвращаем кнопку в исходное состояние.
        setSoundOn(false)
      })
    }
  }, [soundOn, audioReady])

  return (
    <div className={`video-player-wrapper ${className}`} style={{ position: 'relative', overflow: 'hidden' }}>
      <video
        ref={videoRef}
        className="video-player"
        poster={poster}
        muted
        controls
        playsInline
        style={{ width: '100%', height: '100%', borderRadius: 'var(--radius)', background: '#000' }}
      />
      {/* Скрытый элемент звука.
          Именно <video>, а не <audio>: hls.js/Chrome ставит <audio> без
          controls в display:none и тогда НЕ декодирует звук (readyState
          остаётся 0). У <video> такого поведения нет, поэтому звук ведём
          через него — спрятав контейнер и отключив картинку.
          Прячем размером и прозрачностью, а НЕ display:none: элемент обязан
          оставаться в потоке отображения, иначе декодирование не запустится. */}
      <video
        ref={audioElRef}
        autoPlay
        muted
        playsInline
        style={{
          position: 'absolute', bottom: 0, left: 0,
          width: 1, height: 1, opacity: 0, pointerEvents: 'none',
          display: 'block',
        }}
      />

      {/* Кнопка звука: показывается, только когда аудиопоток доступен */}
      {audioAvailable && (
        <button
          onClick={() => setSoundOn((v) => !v)}
          title={soundOn ? 'Выключить звук' : 'Включить звук'}
          style={{
            position: 'absolute', top: 10, right: 10, zIndex: 5,
            background: soundOn ? 'var(--accent)' : 'rgba(0,0,0,0.6)',
            color: '#fff', border: 'none', borderRadius: 6,
            width: 34, height: 34, cursor: 'pointer',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 16, lineHeight: 1,
          }}
        >
          {soundOn ? '🔊' : '🔇'}
        </button>
      )}

      {/* Индикатор состояния */}
      {status === 'connecting' && (
        <div className="video-status-overlay">
          <div className="spinner" style={{ margin: 0, width: 28, height: 28, borderWidth: 2 }} />
          <span>Подключение к потоку...</span>
        </div>
      )}
      {status === 'error' && retryCount >= 5 && (
        <div className="video-status-overlay">
          <span style={{ color: 'var(--danger)' }}>Не удалось подключиться к камере</span>
          <button className="btn btn-outline btn-sm" onClick={initHls} style={{ marginTop: 8 }}>
            Повторить
          </button>
        </div>
      )}
      {status === 'playing' && (
        <div className="video-live-badge">
          <span className="badge-dot badge-dot-online" style={{ width: 8, height: 8 }} />
          LIVE
        </div>
      )}
    </div>
  )
}