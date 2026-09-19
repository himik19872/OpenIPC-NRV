import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
})

// Интерсептор для JWT
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (r) => r,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

export default api

// --- Типы ---

export interface Camera {
  id: string
  name: string
  rtsp_url: string
  main_stream?: string
  sub_stream?: string
  ip?: string
  mac?: string
  firmware?: string
  site_id?: string
  wg_ip?: string
  status: 'online' | 'offline' | 'recording'
  ptz?: boolean
  hw_info?: Record<string, any>
  settings?: Record<string, any>
  created_at: string
  updated_at: string
}

export interface CameraCommandResult {
  command: string
  output?: string
  success: boolean
  error?: string
}

export interface PTZStatus {
  pan: number
  tilt: number
  zoom: number
  supports_ptz: boolean
}

export interface PTZPreset {
  token: string
  name: string
}

export interface DiscoveredCamera {
  ip: string
  mac?: string
  firmware?: string
  model?: string
  main_stream: string
  sub_stream: string
  snapshot?: string
  online: boolean
}

export interface ScanResult {
  subnet: string
  total: number
  found: number
  cameras: DiscoveredCamera[]
}

export interface DetectionEvent {
  id: string
  camera_id: string
  camera_name?: string
  timestamp: string
  object_class: string
  confidence: number
  bbox?: { x: number; y: number; w: number; h: number }
  track_id?: number
  snapshot_path?: string
  thumbnail_path?: string
  // Метаданные события: у номеров здесь лежит plate_text — сам распознанный
  // номер, он остаётся в событии даже без записи в справочнике.
  metadata?: Record<string, any>
  // Результат сравнения со справочником известных лиц и номеров
  match_type?: MatchType
  matched_id?: string
  matched_name?: string
}

export interface Recording {
  id: string
  camera_id: string
  // Имя камеры приходит из JOIN с cameras — показываем его вместо UUID
  camera_name?: string
  start_time: string
  end_time: string
  // Бэкенд отдаёт длительность в поле duration (секунды)
  duration: number
  file_path: string
  file_size: number
  resolution?: string
  codec?: string
  event_triggered: boolean
  // Что вызвало запись: object, line, face, plate, always, manual
  trigger_type?: TriggerType
  // Расшифровка: класс объекта, имя человека, номер автомобиля
  trigger_detail?: string
  // Временная ссылка на файл (presigned для MinIO, путь к API для локального диска)
  url?: string
}

export interface ACSController {
  id: string
  name: string
  vendor: string
  ip: string
  port: number
  site_id?: string
  status: 'online' | 'offline'
  config?: Record<string, any>
  created_at: string
}

export interface ACSEvent {
  id: string
  controller_id: string
  door_id: string
  event_type: string
  card_number?: string
  user_id?: string
  timestamp: string
  camera_id?: string
  snapshot_path?: string
}

export interface Stats {
  total_cameras: number
  online_cameras: number
  total_events_24h: number
  disk_used_gb: number
  disk_total_gb: number
  acs_online: number
  acs_total: number
}

export interface StreamInfo {
  rtsp_url: string
  hls_url: string
  webrtc_url: string
  status: string
  main_hls_url: string
  sub_hls_url: string
  main_rtsp_url: string
  sub_rtsp_url: string
  snapshot_url: string
}

export interface StreamProbeResult {
  ok: boolean
  message: string
  codec?: string
  width?: number
  height?: number
  has_audio: boolean
  audio_codec?: string
  fps?: string
}

// --- Аудио ---

export interface AudioSettings {
  camera_id: string
  has_microphone: boolean
  enabled: boolean
  volume: number
  source_codec: 'auto' | 'g711' | 'opus' | 'aac'
  transcode: boolean
  detect_audio: boolean
  audio_events: string[]
  audio_threshold: number
  speaker_enabled: boolean
  speaker_codec: 'g711' | 'aac'
}

export interface AudioStatus {
  camera_id: string
  available: boolean
  codec: string
  transcoding: boolean
  hls_has_audio: boolean
  audio_path: string
  // Умеет ли камера принимать звук на динамик.
  // Ложь — двусторонняя связь невозможна аппаратно.
  backchannel: boolean
  // Идёт ли прямо сейчас передача звука на камеру.
  talking: boolean
}

export interface AudioEvent {
  id: string
  camera_id: string
  camera_name?: string
  timestamp: string
  event_class: string
  confidence: number
  loudness_db?: number
  duration_sec?: number
  transcript?: string
  clip_path?: string
}

export interface PaginatedResponse<T> {
  events?: T[]
  recordings?: T[]
  total: number
  page: number
  page_size: number
}

// --- API методы ---

export const authAPI = {
  login: (username: string, password: string) =>
    api.post<{ token: string; expires_at: number; user: any }>('/auth/login', { username, password }),
}

export const camerasAPI = {
  list: () => api.get<Camera[]>('/cameras'),
  get: (id: string) => api.get<Camera>(`/cameras/${id}`),
  getStream: (id: string) => api.get<StreamInfo>(`/cameras/${id}/stream`),
  getSnapshot: (id: string) => api.get<{ message: string }>(`/cameras/${id}/snapshot`),
  create: (data: {
    name: string; rtsp_url?: string; main_stream?: string; sub_stream?: string;
    ip?: string; mac?: string; firmware?: string; username?: string; password?: string;
  }) => api.post<Camera>('/cameras', data),
  update: (id: string, data: {
    name?: string; rtsp_url?: string; main_stream?: string; sub_stream?: string;
    ip?: string; mac?: string; firmware?: string; status?: string;
    username?: string; password?: string; wg_ip?: string; ptz?: boolean;
  }) => api.patch<Camera>(`/cameras/${id}`, data),
  delete: (id: string) => api.delete(`/cameras/${id}`),
  // Перезапуск стримера камеры (служба Majestic на OpenIPC)
  restartStreamer: (id: string) =>
    api.post<CameraCommandResult>(`/cameras/${id}/restart-streamer`, {}),
  // Перезагрузка камеры целиком
  reboot: (id: string) =>
    api.post<CameraCommandResult>(`/cameras/${id}/reboot`, {}),
  // Проверка доступности звука и состояние транскодирования
  getAudioStatus: (id: string) => api.get<AudioStatus>(`/cameras/${id}/audio/status`),

  // Проверка RTSP-адреса до сохранения камеры: вернёт параметры потока
  // либо понятное объяснение (неверный пароль, нет видео, таймаут).
  probeStream: (data: { rtsp_url: string; username?: string; password?: string }) =>
    api.post<StreamProbeResult>('/cameras/probe-stream', data),
}

export const audioAPI = {
  // Настройки звука конкретной камеры
  getSettings: (cameraId: string) =>
    api.get<AudioSettings>(`/cameras/${cameraId}/audio`),
  updateSettings: (cameraId: string, data: Partial<AudioSettings>) =>
    api.patch<AudioSettings>(`/cameras/${cameraId}/audio`, data),
  status: (cameraId: string) =>
    api.get<AudioStatus>(`/cameras/${cameraId}/audio/status`),

  // События аудиодетекции (пока заполняются детектором на стороне камеры)
  events: (params?: { camera_id?: string; event_class?: string; page?: number; page_size?: number }) =>
    api.get<PaginatedResponse<AudioEvent> & { events: AudioEvent[] }>('/audio/events', { params }),
  classes: () => api.get<{ classes: string[] }>('/audio/classes'),
  stats: () => api.get<{ total_24h: number; by_class: Record<string, number> }>('/audio/stats'),

  // --- Двусторонняя связь (звук оператора → динамик камеры) ---
  // Доступна только для камер с поддержкой обратного аудиоканала.
  startTalk: (cameraId: string, sampleRate = 8000, codec = 'g711') =>
    api.post<{ talking: boolean }>(`/cameras/${cameraId}/audio/talk/start`, {
      sample_rate: sampleRate,
      codec,
    }),
  stopTalk: (cameraId: string) =>
    api.post<{ talking: boolean }>(`/cameras/${cameraId}/audio/talk/stop`, {}),
  // Порция звука: сырые PCM s16le в теле запроса (не JSON),
  // потому что это поток байт, а не структурированные данные.
  sendTalkChunk: (cameraId: string, pcm: ArrayBuffer) =>
    api.post(`/cameras/${cameraId}/audio/talk/chunk`, pcm, {
      headers: { 'Content-Type': 'application/octet-stream' },
    }),
}

export const ptzAPI = {
  status: (id: string) => api.get<PTZStatus>(`/cameras/${id}/ptz/status`),
  move: (id: string, pan: number, tilt: number, zoom = 0, durationMs = 500) =>
    api.post<{ success: boolean }>(`/cameras/${id}/ptz/move`, {
      pan, tilt, zoom, duration_ms: durationMs,
    }),
  stop: (id: string) => api.post<{ success: boolean }>(`/cameras/${id}/ptz/stop`, {}),
  presets: (id: string) => api.get<PTZPreset[]>(`/cameras/${id}/ptz/presets`),
  gotoPreset: (id: string, token: string) =>
    api.post<{ success: boolean }>(`/cameras/${id}/ptz/presets/goto`, { token }),
}

export const scannerAPI = {
  scan: (subnet: string, username?: string, password?: string) =>
    api.post<ScanResult>('/scanner/scan', { subnet, username, password }),
  probe: (ip: string, username?: string, password?: string) =>
    api.post<DiscoveredCamera>('/scanner/probe', { ip, username, password }),
}
export const eventsAPI = {
  list: (params?: { camera_id?: string; object_class?: string; page?: number; page_size?: number }) =>
    api.get<PaginatedResponse<DetectionEvent> & { events: DetectionEvent[] }>('/events', { params }),
  get: (id: string) => api.get<DetectionEvent>(`/events/${id}`),
}

export const recordingsAPI = {
  list: (params?: { camera_id?: string; page?: number; page_size?: number; trigger?: string; search?: string }) =>
    api.get<PaginatedResponse<Recording> & { recordings: Recording[] }>('/recordings', { params }),
  get: (id: string) => api.get<Recording>(`/recordings/${id}`),
  delete: (id: string) => api.delete(`/recordings/${id}`),
}

export const acsAPI = {
  listControllers: () => api.get<ACSController[]>('/acs/controllers'),
  getController: (id: string) => api.get<ACSController>(`/acs/controllers/${id}`),
  createController: (data: any) => api.post<ACSController>('/acs/controllers', data),
  deleteController: (id: string) => api.delete(`/acs/controllers/${id}`),
  listEvents: (params?: { page?: number; page_size?: number }) =>
    api.get<PaginatedResponse<ACSEvent>>('/acs/events', { params }),
  openDoor: (controllerID: string, doorID: string) =>
    api.post(`/acs/doors/${controllerID}/open`, { door_id: doorID }),
}

export const statsAPI = {
  get: () => api.get<Stats>('/stats'),
}

// --- Настройки AI-детекции ---

export interface Point {
  x: number
  y: number
}

export type DetectType = 'object' | 'line' | 'face' | 'plate'
export type RecordMode = 'off' | 'always' | 'event'
export type LineDirection = 'both' | 'forward' | 'backward'

export interface DetectionSettings {
  camera_id: string
  enabled: boolean
  object_classes: string[]
  min_confidence: number
  detect_types: DetectType[]
  zone: Point[]
  line: Point[]
  line_direction: LineDirection
  save_snapshots: boolean
  record_mode: RecordMode
  prebuffer_sec: number
  postbuffer_sec: number
  cooldown_sec: number
  // Область поиска номеров: полигон в 0..1. Пустая — весь кадр.
  plate_zone: Point[]
  // Правила проверки формата номера — отсекают мусор вроде логотипа камеры.
  plate_min_length: number
  plate_max_length: number
  plate_pattern: string
  plate_min_confidence: number
  updated_at: string
}

export interface StorageConfig {
  backend: 'minio' | 'local'
  local_path: string
  retention_days: number
}

export interface ServerSettings {
  storage: StorageConfig
  snapshots: StorageConfig
}

// Классы объектов COCO, которые умеет распознавать YOLOv8.
// Значение — класс модели, подпись — то, что видит пользователь.
export const OBJECT_CLASSES: { value: string; label: string }[] = [
  { value: 'person', label: 'Люди' },
  { value: 'bicycle', label: 'Велосипеды' },
  { value: 'car', label: 'Легковые авто' },
  { value: 'motorcycle', label: 'Мотоциклы' },
  { value: 'bus', label: 'Автобусы' },
  { value: 'truck', label: 'Грузовики' },
  { value: 'dog', label: 'Собаки' },
  { value: 'cat', label: 'Кошки' },
  { value: 'backpack', label: 'Рюкзаки' },
  { value: 'suitcase', label: 'Чемоданы' },
]

export const DETECT_TYPES: { value: DetectType; label: string; hint: string }[] = [
  { value: 'object', label: 'Объекты', hint: 'Люди, машины и другие объекты' },
  { value: 'line', label: 'Пересечение линии', hint: 'Подсчёт пересечений через линию' },
  { value: 'face', label: 'Лица', hint: 'Распознавание лиц (требует модель)' },
  { value: 'plate', label: 'Номера', hint: 'Распознавание автономеров (требует модель)' },
]

/**
 * Шаблоны формата автомобильных номеров.
 * Регулярное выражение применяется к нормализованной строке: кириллица
 * приведена к латинице, разделители убраны. Например «А123ВС77» → «A123BC77».
 *
 * Шаблон нужен, чтобы OCR не принимал за номер надписи из кадра —
 * логотип камеры, название улицы и подобное.
 */
export const PLATE_PATTERNS: Record<string, { label: string; pattern: string; example: string }> = {
  ru: {
    label: 'Россия (А123ВС77)',
    pattern: '^[ABEKMHOPCTYX]\\d{3}[ABEKMHOPCTYX]{2}\\d{2,3}$',
    example: 'А123ВС77',
  },
  by: {
    label: 'Беларусь (1234АВ5)',
    pattern: '^\\d{4}[ABEKMHOPCTYX]{2}\\d$',
    example: '1234АВ5',
  },
  kz: {
    label: 'Казахстан (123АВ77)',
    pattern: '^\\d{3}[ABEKMHOPCTYX]{2}\\d{2,3}$',
    example: '123АВ77',
  },
  any: {
    label: 'Любой формат (только длина)',
    pattern: '',
    example: '12345678',
  },
}

/**
 * Определяет, какой шаблон сейчас выбран, по его значению.
 * Возвращает 'custom', если шаблон задан вручную и не совпадает
 * ни с одной предустановкой.
 */
export function detectPatternKey(pattern: string): string {
  if (!pattern) return 'any'
  for (const [key, v] of Object.entries(PLATE_PATTERNS)) {
    if (v.pattern === pattern) return key
  }
  return 'custom'
}

export const detectionAPI = {
  get: (cameraId: string) =>
    api.get<DetectionSettings>(`/cameras/${cameraId}/detection`),
  update: (cameraId: string, data: Partial<Omit<DetectionSettings, 'camera_id' | 'updated_at'>>) =>
    api.patch<DetectionSettings>(`/cameras/${cameraId}/detection`, data),
}

export const settingsAPI = {
  get: () => api.get<ServerSettings>('/settings'),
  update: (data: Partial<ServerSettings>) => api.patch<ServerSettings>('/settings', data),
}

// --- Распознавание лиц и автомобильных номеров ---

// Причина, по которой создана запись архива.
export type TriggerType = 'manual' | 'always' | 'object' | 'line' | 'face' | 'plate'

// Результат сравнения со справочником.
export type MatchType = 'unknown' | 'known' | 'blocked'

export interface KnownFace {
  id: string
  name: string
  note: string
  photo_path?: string
  // Ссылка на эталонный снимок (относительная, отдаётся бэкендом)
  photo_url?: string
  is_blocked: boolean
  enabled: boolean
  // false означает «снимок есть, но биометрия не рассчитана»
  has_embedding: boolean
  created_at: string
  updated_at: string
}

export interface KnownPlate {
  id: string
  plate: string
  plate_norm: string
  owner: string
  note: string
  photo_path?: string
  photo_url?: string
  is_blocked: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface FaceRecognitionSettings {
  enabled: boolean
  threshold: number
  snapshot_unknown: boolean
  alert_blocked: boolean
}

export interface PlateRecognitionSettings {
  enabled: boolean
  threshold: number
  region: string
  snapshot_unknown: boolean
  alert_blocked: boolean
}

export interface RecognitionSettings {
  faces: FaceRecognitionSettings
  plates: PlateRecognitionSettings
}

// Подписи и цвета для триггеров записи — используются в архиве.
export const TRIGGER_LABELS: Record<TriggerType, string> = {
  manual: 'Вручную',
  always: 'Непрерывно',
  object: 'Объект',
  line: 'Пересечение линии',
  face: 'Лицо',
  plate: 'Номер авто',
}

export const facesAPI = {
  list: (all = false) => api.get<{ faces: KnownFace[]; total: number }>('/faces', { params: { all } }),
  create: (data: { name: string; note?: string; is_blocked?: boolean; embedding?: number[]; photo_base64?: string }) =>
    api.post<KnownFace>('/faces', data),
  update: (id: string, data: Partial<{ name: string; note: string; is_blocked: boolean; enabled: boolean; photo_base64: string }>) =>
    api.patch<KnownFace>(`/faces/${id}`, data),
  delete: (id: string) => api.delete(`/faces/${id}`),
  // Ссылка на снимок: эндпоинт вне JWT, поэтому токен передаём в query
  photoURL: (id: string) => `/api/v1/faces/${id}/photo`,
}

export const platesAPI = {
  list: (all = false) => api.get<{ plates: KnownPlate[]; total: number }>('/plates', { params: { all } }),
  create: (data: { plate: string; owner?: string; note?: string; is_blocked?: boolean; photo_base64?: string }) =>
    api.post<KnownPlate>('/plates', data),
  update: (id: string, data: Partial<{ plate: string; owner: string; note: string; is_blocked: boolean; enabled: boolean }>) =>
    api.patch<KnownPlate>(`/plates/${id}`, data),
  delete: (id: string) => api.delete(`/plates/${id}`),
  photoURL: (id: string) => `/api/v1/plates/${id}/photo`,
}

export const recognitionAPI = {
  getSettings: () => api.get<RecognitionSettings>('/settings/recognition'),
  updateSettings: (data: Partial<RecognitionSettings>) =>
    api.patch<RecognitionSettings>('/settings/recognition', data),
  stats: () => api.get<{ faces: number; plates: number }>('/recognition/stats'),
}