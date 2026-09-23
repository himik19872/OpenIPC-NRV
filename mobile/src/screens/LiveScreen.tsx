import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import Video, { type OnVideoErrorData } from 'react-native-video';
import { useApp } from '../state/AppContext';
import { describeNetworkError } from '../net/api';
import { joinUrl } from '../net/address';
import { colors, radius, spacing } from '../theme';
import type { Camera, StreamInfo } from '../types';

/** Тип потока камеры: основной или дополнительный. */
type StreamKind = 'sub' | 'main';

/**
 * Онлайн-просмотр камеры.
 *
 * Видео идёт по HLS: сервер отдаёт плейлист через свой API, а токен
 * подставляется в query — плеер не умеет слать заголовок Authorization.
 *
 * По умолчанию открывается дополнительный поток (704×576): он заметно
 * экономичнее по трафику и батарее, что важно за пределами локальной сети.
 * Переключиться на основной можно кнопкой.
 */
export default function LiveScreen({
  camera,
  onBack,
}: {
  camera: Camera;
  onBack: () => void;
}) {
  const { client, current, token } = useApp();
  const [stream, setStream] = useState<StreamInfo | null>(null);
  const [kind, setKind] = useState<StreamKind>('sub');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [videoError, setVideoError] = useState<string | null>(null);
  const [paused, setPaused] = useState(false);

  // Плеер пересоздаётся при смене потока, поэтому ключ считаем из адреса:
  // без этого переключение может оставить старое видео на экране.
  const [reloadKey, setReloadKey] = useState(0);
  const retriesRef = useRef(0);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!client) return;
      setLoading(true);
      try {
        const info = await client.getStream(camera.id);
        if (!cancelled) {
          setStream(info);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) setError(describeNetworkError(err));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [client, camera.id]);

  /**
   * Адрес HLS-плейлиста.
   *
   * Сервер отдаёт путь относительным, поэтому приклеиваем адрес сервера.
   * Поле webrtc_url не используем: сервер отдаёт в нём внутренний
   * адрес вида localhost:8889, недоступный с телефона.
   */
  const hlsUrl = useMemo(() => {
    if (!stream || !token) return null;
    const path =
      kind === 'sub'
        ? stream.sub_hls_url || stream.hls_url
        : stream.main_hls_url || stream.hls_url;
    if (!path) return null;

    const full = joinUrl(current?.baseUrl ?? '', path);
    const separator = full.includes('?') ? '&' : '?';
    return `${full}${separator}token=${encodeURIComponent(token)}`;
  }, [stream, kind, token, current]);

  const handleVideoError = useCallback(
    (event: OnVideoErrorData) => {
      const message =
        event?.error?.errorString || event?.error?.localizedDescription || '';
      // Сетевые сбои лечим повтором: поток мог ещё не подняться на сервере
      // или сессия плейлиста устарела.
      if (retriesRef.current < 3) {
        retriesRef.current += 1;
        setTimeout(() => setReloadKey((value) => value + 1), 1500);
        setVideoError('Поток не отвечает, пробую снова…');
        return;
      }
      setVideoError(
        message
          ? `Не удалось воспроизвести: ${message}`
          : 'Не удалось воспроизвести поток',
      );
    },
    [],
  );

  const switchStream = () => {
    const next: StreamKind = kind === 'sub' ? 'main' : 'sub';
    // Сбрасываем счётчик повторов: у другого потока может быть своя причина
    // временной недоступности, и попытки нужно отсчитывать заново.
    retriesRef.current = 0;
    setVideoError(null);
    setKind(next);
    setReloadKey((value) => value + 1);
  };

  return (
    <View style={styles.container}>
      <View style={styles.videoBox}>
        {hlsUrl ? (
          <Video
            key={`${kind}-${reloadKey}`}
            source={{ uri: hlsUrl }}
            style={styles.video}
            resizeMode="contain"
            paused={paused}
            controls
            onError={handleVideoError}
            onLoad={() => {
              retriesRef.current = 0;
              setVideoError(null);
            }}
            // Плеер не должен останавливаться при уходе приложения в фон:
            // оператор обычно сверяется с другими делами, глядя на экран.
            playInBackground={false}
            // Задержка минимальная: для наблюдения важно видеть происходящее
            // сейчас, а не 10 секунд назад.
            bufferConfig={{
              minBufferMs: 1000,
              maxBufferMs: 5000,
              bufferForPlaybackMs: 500,
              bufferForPlaybackAfterRebufferMs: 1000,
            }}
          />
        ) : (
          <View style={styles.videoPlaceholder}>
            {loading ? (
              <>
                <ActivityIndicator color={colors.primary} size="large" />
                <Text style={styles.placeholderText}>Готовлю поток…</Text>
              </>
            ) : (
              <Text style={styles.placeholderText}>
                {error || 'Адрес потока недоступен'}
              </Text>
            )}
          </View>
        )}
      </View>

      {videoError && (
        <View style={styles.warning}>
          <Text style={styles.warningText}>{videoError}</Text>
        </View>
      )}

      <View style={styles.info}>
        <Text style={styles.cameraName}>{camera.name}</Text>
        <Text style={styles.cameraMeta}>
          {camera.ip || 'адрес не указан'}
          {stream?.status ? ` · ${stream.status}` : ''}
        </Text>
      </View>

      <View style={styles.buttons}>
        <Pressable
          style={styles.button}
          onPress={switchStream}
          disabled={!stream}
        >
          <Text style={styles.buttonText}>
            {kind === 'sub' ? 'Основной поток' : 'Дополнительный поток'}
          </Text>
        </Pressable>

        <Pressable
          style={styles.button}
          onPress={() => setPaused((value) => !value)}
        >
          <Text style={styles.buttonText}>
            {paused ? 'Продолжить' : 'Пауза'}
          </Text>
        </Pressable>

        <Pressable
          style={[styles.button, styles.buttonBack]}
          onPress={onBack}
        >
          <Text style={styles.buttonText}>К списку</Text>
        </Pressable>
      </View>

      <Text style={styles.hint}>
        {kind === 'sub'
          ? 'Дополнительный поток экономит трафик. Переключитесь на основной для большей детализации.'
          : 'Основной поток даёт лучшее качество и расходует больше трафика.'}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.background },
  videoBox: {
    width: '100%',
    aspectRatio: 16 / 9,
    backgroundColor: '#000',
  },
  video: { width: '100%', height: '100%' },
  videoPlaceholder: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: spacing.md,
    padding: spacing.lg,
  },
  placeholderText: {
    color: colors.textSecondary,
    fontSize: 14,
    textAlign: 'center',
  },

  warning: {
    margin: spacing.lg,
    marginBottom: 0,
    backgroundColor: 'rgba(245,158,11,0.12)',
    borderWidth: 1,
    borderColor: colors.warning,
    borderRadius: radius.md,
    padding: spacing.md,
  },
  warningText: { color: colors.text, fontSize: 13 },

  info: { padding: spacing.lg },
  cameraName: { fontSize: 20, fontWeight: '700', color: colors.text },
  cameraMeta: {
    fontSize: 13,
    color: colors.textSecondary,
    marginTop: spacing.xs,
  },

  buttons: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: spacing.sm,
    paddingHorizontal: spacing.lg,
  },
  button: {
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.md,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.md,
  },
  buttonBack: { borderColor: colors.primary },
  buttonText: { color: colors.text, fontSize: 14, fontWeight: '500' },

  hint: {
    fontSize: 12,
    color: colors.textMuted,
    padding: spacing.lg,
    lineHeight: 18,
  },
});
