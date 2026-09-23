/**
 * Типы данных, которые приложение получает от сервера NVR.
 *
 * Описания повторяют ответы REST API (см. docs/API.md в репозитории
 * сервера). Поля необязательные там, где сервер может их не прислать:
 * например, старые камеры не отдают дополнительные потоки.
 */

/** Сохранённый сервер, к которому подключается приложение. */
export interface ServerProfile {
  /** Идентификатор записи на устройстве. Генерируется при добавлении. */
  id: string;
  /** Имя, которое видит пользователь: «Офис», «Дача». */
  name: string;
  /**
   * Базовый адрес без завершающего слэша: `http://192.168.1.10:8080`.
   * Хранится в нормализованном виде, чтобы сборка ссылок была однозначной.
   */
  baseUrl: string;
  /** Логин, сохранённый для этого сервера. */
  username?: string;
}

/** Ответ сервера на успешный вход. */
export interface LoginResult {
  token: string;
  /** Unix-время истечения токена в секундах. */
  expires_at: number;
  user: {
    id: string;
    username: string;
    role: string;
    permissions?: Record<string, unknown>;
  };
}

/** Камера в списке. */
export interface Camera {
  id: string;
  name: string;
  rtsp_url?: string;
  main_stream?: string;
  sub_stream?: string;
  ip?: string;
  mac?: string;
  firmware?: string;
  status: 'online' | 'offline' | 'recording';
  ptz?: boolean;
  /** Номер канала внешнего RTSP-доступа. Может отсутствовать. */
  channel_number?: number;
  hw_info?: Record<string, unknown>;
  settings?: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
}

/**
 * Адреса потоков камеры.
 *
 * ВАЖНО: поля hls_url, main_hls_url и sub_hls_url сервер отдаёт
 * ОТНОСИТЕЛЬНЫМИ (`/api/v1/...`), поэтому в приложении к ним нужно
 * приклеивать базовый адрес выбранного сервера.
 *
 * Поля webrtc_url и rtsp_url сервер отдаёт с внутренним именем хоста
 * (localhost) — с телефона они не работают, и приложение их не использует.
 */
export interface StreamInfo {
  rtsp_url?: string;
  hls_url?: string;
  webrtc_url?: string;
  status?: string;
  main_hls_url?: string;
  sub_hls_url?: string;
  main_rtsp_url?: string;
  sub_rtsp_url?: string;
  /** Адрес кадра в локальной сети камеры: с телефона недоступен. */
  snapshot_url?: string;
}

/** Одна запись архива. */
export interface Recording {
  id: string;
  camera_id: string;
  camera_name?: string;
  start_time: string;
  end_time: string;
  /** Длительность в секундах. */
  duration: number;
  file_path?: string;
  file_size: number;
  /** Кодек файла: h264 воспроизводится везде, hevc — не на всех телефонах. */
  codec?: string;
  resolution?: string;
  trigger_type?: string;
  trigger_detail?: string;
  event_triggered?: boolean;
  /**
   * Ссылка на файл записи. Сервер отдаёт её ОТНОСИТЕЛЬНОЙ намеренно:
   * подписанная ссылка MinIO ломается при доступе с другого адреса.
   */
  url?: string;
}

/** Ответ сервера на запрос списка записей. */
export interface RecordingsPage {
  recordings: Recording[];
  total: number;
  page: number;
  page_size: number;
}
