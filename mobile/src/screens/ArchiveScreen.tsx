import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import Video from 'react-native-video';
import { useApp } from '../state/AppContext';
import { describeNetworkError } from '../net/api';
import { joinUrl } from '../net/address';
import { colors, radius, spacing, triggerColors, triggerLabels } from '../theme';
import type { Camera, Recording } from '../types';

/** Сколько записей запрашивать за раз. */
const PAGE_SIZE = 20;

/** Приводит длительность в секундах к виду «5:03». */
function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  return `${minutes}:${rest.toString().padStart(2, '0')}`;
}

/** Приводит размер файла к читаемому виду. */
function formatSize(bytes: number): string {
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(2)} ГБ`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(1)} МБ`;
  if (bytes >= 1e3) return `${(bytes / 1e3).toFixed(0)} КБ`;
  return `${bytes} Б`;
}

/** Дата и время записи в местном времени телефона. */
function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

/**
 * Архив записей.
 *
 * Список можно сузить до одной камеры: при открытии из карточки камеры
 * показываются только её записи. Полный архив открывается кнопкой из
 * списка камер.
 */
export default function ArchiveScreen({
  camera,
  onBack,
}: {
  camera?: Camera;
  onBack: () => void;
}) {
  const { client, current, token } = useApp();
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [playing, setPlaying] = useState<Recording | null>(null);
  const [playError, setPlayError] = useState<string | null>(null);
  // Фильтр по камере: пусто — весь архив.
  const [cameraFilter, setCameraFilter] = useState<string | null>(
    camera?.id ?? null,
  );
  const [cameras, setCameras] = useState<Camera[]>([]);

  const load = useCallback(
    async (targetPage: number, silent = false) => {
      if (!client) return;
      if (!silent) setLoading(true);
      try {
        const result = await client.listRecordings({
          cameraId: cameraFilter ?? undefined,
          page: targetPage,
          pageSize: PAGE_SIZE,
        });
        // На первой странице заменяем список, на следующих — добавляем:
        // так работает подгрузка при прокрутке.
        setRecordings((prev) =>
          targetPage === 1 ? result.recordings : [...prev, ...result.recordings],
        );
        setTotal(result.total);
        setPage(targetPage);
        setError(null);
      } catch (err) {
        setError(describeNetworkError(err));
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [client, cameraFilter],
  );

  // Список камер нужен для фильтра. Если экран открыт из карточки, список
  // всё равно запрашиваем: пользователь может захотеть посмотреть архив
  // другой камеры, не выходя из экрана.
  useEffect(() => {
    if (!client) return;
    let cancelled = false;
    (async () => {
      try {
        const list = await client.listCameras();
        if (!cancelled) setCameras(list);
      } catch {
        // Ошибку списка камер здесь не показываем: архив важнее, а фильтр
        // без списка просто останется недоступным.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [client]);

  useEffect(() => {
    void load(1);
  }, [load]);

  /**
   * Адрес файла записи.
   *
   * Сервер отдаёт относительную ссылку намеренно (подписанная ссылка MinIO
   * ломается при доступе с другого адреса), поэтому приклеиваем адрес
   * выбранного сервера. Токен идёт в query: плеер не шлёт заголовки.
   */
  const playbackUrl = useMemo(() => {
    if (!playing?.url || !token) return null;
    const full = joinUrl(current?.baseUrl ?? '', playing.url);
    const separator = full.includes('?') ? '&' : '?';
    return `${full}${separator}jwt=${encodeURIComponent(token)}`;
  }, [playing, token, current]);

  const openRecording = (recording: Recording) => {
    if (!recording.url) {
      setError('У записи нет ссылки на файл');
      return;
    }
    setPlayError(null);
    setPlaying(recording);
  };

  const onEndReached = () => {
    // Подгружаем следующую страницу, только если она есть.
    if (!loading && recordings.length < total) {
      void load(page + 1, true);
    }
  };

  if (playing) {
    const isHevc = (playing.codec || '').toLowerCase().includes('h265')
      || (playing.codec || '').toLowerCase().includes('hevc');

    return (
      <View style={styles.container}>
        <View style={styles.playerBox}>
          {playbackUrl ? (
            <Video
              source={{ uri: playbackUrl }}
              style={styles.player}
              resizeMode="contain"
              controls
              paused={false}
              onError={(event) => {
                const message =
                  event?.error?.errorString
                  || event?.error?.localizedDescription
                  || '';
                // HEVC — частая причина отказа: не все телефоны умеют его
                // декодировать, и браузер в веб-интерфейсе падает так же.
                setPlayError(
                  isHevc
                    ? 'Запись в HEVC (H.265): этот телефон её не воспроизводит'
                    : message || 'Не удалось воспроизвести запись',
                );
              }}
            />
          ) : (
            <View style={styles.playerPlaceholder}>
              <Text style={styles.placeholderText}>
                Ссылка на файл недоступна
              </Text>
            </View>
          )}
        </View>

        {playError && (
          <View style={styles.warning}>
            <Text style={styles.warningText}>{playError}</Text>
          </View>
        )}

        <ScrollView contentContainerStyle={styles.playerInfo}>
          <Text style={styles.recordTitle}>
            {playing.camera_name || camera?.name || 'Запись'}
          </Text>
          <Text style={styles.recordMeta}>
            {formatDateTime(playing.start_time)} ·{' '}
            {formatDuration(playing.duration)} · {formatSize(playing.file_size)}
          </Text>
          {playing.resolution && (
            <Text style={styles.recordMeta}>
              {playing.resolution}
              {playing.codec ? ` · ${playing.codec.toUpperCase()}` : ''}
            </Text>
          )}
        </ScrollView>

        <Pressable
          style={styles.backButton}
          onPress={() => {
            setPlaying(null);
            setPlayError(null);
          }}
        >
          <Text style={styles.backButtonText}>Назад к списку</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.title}>Архив</Text>
        <Text style={styles.subtitle}>
          {cameraFilter
            ? cameras.find((c) => c.id === cameraFilter)?.name || 'Одна камера'
            : 'Все камеры'}{' '}
          · {total} записей
        </Text>
      </View>

      {cameras.length > 0 && (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.filters}
        >
          <Pressable
            style={[styles.chip, !cameraFilter && styles.chipActive]}
            onPress={() => setCameraFilter(null)}
          >
            <Text
              style={[styles.chipText, !cameraFilter && styles.chipTextActive]}
            >
              Все камеры
            </Text>
          </Pressable>
          {cameras.map((item) => (
            <Pressable
              key={item.id}
              style={[
                styles.chip,
                cameraFilter === item.id && styles.chipActive,
              ]}
              onPress={() => setCameraFilter(item.id)}
            >
              <Text
                style={[
                  styles.chipText,
                  cameraFilter === item.id && styles.chipTextActive,
                ]}
                numberOfLines={1}
              >
                {item.name}
              </Text>
            </Pressable>
          ))}
        </ScrollView>
      )}

      {error && (
        <View style={styles.errorBox}>
          <Text style={styles.errorText}>{error}</Text>
        </View>
      )}

      <FlatList
        data={recordings}
        keyExtractor={(item) => item.id}
        contentContainerStyle={styles.list}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => {
              setRefreshing(true);
              void load(1, true);
            }}
            tintColor={colors.textSecondary}
          />
        }
        onEndReached={onEndReached}
        onEndReachedThreshold={0.4}
        ListEmptyComponent={
          loading ? (
            <View style={styles.centered}>
              <ActivityIndicator color={colors.primary} />
              <Text style={styles.centeredText}>Читаю архив…</Text>
            </View>
          ) : (
            <View style={styles.centered}>
              <Text style={styles.centeredText}>Записей не найдено</Text>
            </View>
          )
        }
        ListFooterComponent={
          loading && recordings.length > 0 ? (
            <ActivityIndicator
              color={colors.textSecondary}
              style={styles.footerLoader}
            />
          ) : null
        }
        renderItem={({ item }) => {
          const trigger = item.trigger_type || 'object';
          const triggerColor = triggerColors[trigger] || colors.textSecondary;
          return (
            <Pressable style={styles.record} onPress={() => openRecording(item)}>
              <View style={styles.recordLeft}>
                <Text style={styles.recordTime}>
                  {formatDateTime(item.start_time)}
                </Text>
                <Text style={styles.recordCamera} numberOfLines={1}>
                  {item.camera_name || 'Камера'}
                </Text>
                <View style={styles.recordBadges}>
                  <View
                    style={[
                      styles.triggerBadge,
                      {
                        borderColor: triggerColor,
                        backgroundColor: `${triggerColor}22`,
                      },
                    ]}
                  >
                    <Text style={[styles.triggerText, { color: triggerColor }]}>
                      {triggerLabels[trigger] || trigger}
                    </Text>
                  </View>
                  {item.trigger_detail ? (
                    <Text style={styles.triggerDetail} numberOfLines={1}>
                      {item.trigger_detail}
                    </Text>
                  ) : null}
                </View>
              </View>

              <View style={styles.recordRight}>
                <Text style={styles.recordDuration}>
                  {formatDuration(item.duration)}
                </Text>
                <Text style={styles.recordSize}>
                  {formatSize(item.file_size)}
                </Text>
              </View>
            </Pressable>
          );
        }}
      />

      <Pressable style={styles.backButton} onPress={onBack}>
        <Text style={styles.backButtonText}>К камерам</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: colors.background },
  header: { padding: spacing.lg, paddingBottom: spacing.sm },
  title: { fontSize: 26, fontWeight: '700', color: colors.text },
  subtitle: { fontSize: 13, color: colors.textSecondary, marginTop: spacing.xs },

  filters: { paddingHorizontal: spacing.lg, gap: spacing.sm, paddingBottom: spacing.sm },
  chip: {
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 999,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    backgroundColor: colors.surface,
    maxWidth: 180,
  },
  chipActive: { borderColor: colors.primary, backgroundColor: `${colors.primary}22` },
  chipText: { fontSize: 13, color: colors.textSecondary },
  chipTextActive: { color: colors.text, fontWeight: '600' },

  list: { padding: spacing.lg, paddingTop: spacing.sm, gap: spacing.sm },
  record: {
    flexDirection: 'row',
    backgroundColor: colors.surface,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    padding: spacing.md,
    gap: spacing.md,
  },
  recordLeft: { flex: 1, gap: spacing.xs },
  recordTime: { fontSize: 14, fontWeight: '600', color: colors.text },
  recordCamera: { fontSize: 12, color: colors.textSecondary },
  recordBadges: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: 2 },
  triggerBadge: {
    borderWidth: 1,
    borderRadius: radius.sm,
    paddingHorizontal: spacing.sm,
    paddingVertical: 1,
  },
  triggerText: { fontSize: 10, fontWeight: '600' },
  triggerDetail: { fontSize: 11, color: colors.textMuted, flexShrink: 1 },
  recordRight: { alignItems: 'flex-end', justifyContent: 'center' },
  recordDuration: { fontSize: 14, fontWeight: '600', color: colors.text },
  recordSize: { fontSize: 11, color: colors.textMuted, marginTop: 2 },

  playerBox: { width: '100%', aspectRatio: 16 / 9, backgroundColor: '#000' },
  player: { width: '100%', height: '100%' },
  playerPlaceholder: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  placeholderText: { color: colors.textSecondary, fontSize: 14 },

  warning: {
    margin: spacing.lg,
    marginBottom: 0,
    backgroundColor: 'rgba(245,158,11,0.12)',
    borderWidth: 1,
    borderColor: colors.warning,
    borderRadius: radius.md,
    padding: spacing.md,
  },
  warningText: { color: colors.text, fontSize: 13, lineHeight: 19 },
  playerInfo: { padding: spacing.lg, gap: spacing.xs },
  recordTitle: { fontSize: 18, fontWeight: '700', color: colors.text },
  recordMeta: { fontSize: 13, color: colors.textSecondary },

  backButton: {
    margin: spacing.lg,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: radius.md,
    paddingVertical: spacing.md,
    alignItems: 'center',
  },
  backButtonText: { color: colors.text, fontSize: 15, fontWeight: '500' },

  errorBox: {
    marginHorizontal: spacing.lg,
    marginBottom: spacing.sm,
    backgroundColor: 'rgba(239,68,68,0.12)',
    borderWidth: 1,
    borderColor: colors.danger,
    borderRadius: radius.md,
    padding: spacing.md,
  },
  errorText: { color: colors.text, fontSize: 13 },
  centered: {
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: spacing.xxl,
    gap: spacing.md,
  },
  centeredText: { color: colors.textSecondary, fontSize: 14 },
  footerLoader: { paddingVertical: spacing.lg },
});
