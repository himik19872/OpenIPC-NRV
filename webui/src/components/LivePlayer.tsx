import { useEffect, useRef, useCallback, useState } from 'react'
import Hls from 'hls.js'

interface LivePlayerProps {
  hlsUrl?: string
  webrtcUrl?: string
  poster?: string
  muted?: boolean
  autoPlay?: boolean
  className?: string
}

export default function LivePlayer({
  hlsUrl,
  poster,
  muted = true,
  autoPlay = true,
  className = '',
}: LivePlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const hlsRef = useRef<Hls | null>(null)
  const [status, setStatus] = useState<'connecting' | 'playing' | 'error' | 'idle'>('idle')
  const [retryCount, setRetryCount] = useState(0)

  const initHls = useCallback(() => {
    const video = videoRef.current
    if (!video || !hlsUrl) return

    // Добавляем JWT-токен в query-параметр (hls.js не может слать Authorization-заголовок)
    let authedUrl = hlsUrl
    try {
      const token = localStorage.getItem('token')
      if (token) {
        const u = new URL(hlsUrl, window.location.origin)
        u.searchParams.set('token', token)
        authedUrl = u.pathname + u.search
      }
    } catch {
      // ignore — оставляем как есть
    }

    // Уничтожаем предыдущий экземпляр
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
  }, [hlsUrl, autoPlay, retryCount])

  useEffect(() => {
    initHls()
    return () => {
      if (hlsRef.current) {
        hlsRef.current.destroy()
        hlsRef.current = null
      }
    }
  }, [initHls])

  return (
    <div className={`video-player-wrapper ${className}`}>
      <video
        ref={videoRef}
        className="video-player"
        poster={poster}
        muted={muted}
        controls
        playsInline
        style={{ width: '100%', height: '100%', borderRadius: 'var(--radius)', background: '#000' }}
      />

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