import axios, { AxiosInstance, AxiosError } from 'axios';
import type {
  Camera,
  LoginResult,
  Recording,
  RecordingsPage,
  StreamInfo,
} from '../types';

/**
 * Клиент REST API сервера NVR.
 *
 * Экземпляр создаётся на каждый сервер, потому что базовый адрес у них
 * разный. Токен подставляется в заголовок Authorization.
 */
export class ApiClient {
  private http: AxiosInstance;
  private token: string | null;

  constructor(
    public readonly baseUrl: string,
    token: string | null = null,
  ) {
    this.token = token;
    this.http = axios.create({
      baseURL: `${baseUrl.replace(/\/+$/, '')}/api/v1`,
      // Таймаут небольшой: если сервер недоступен, пользователь должен
      // узнать об этом сразу, а не ждать полминуты. Для превью кадров
      // сервер отвечает быстро, а долгие операции — это HLS, он идёт
      // не через axios.
      timeout: 15000,
      // Не выбрасываем ошибки 4xx/5xx наружу как исключения axios:
      // разбираем их сами, чтобы показать понятный текст.
      validateStatus: (status) => status >= 200 && status < 500,
    });

    this.http.interceptors.request.use((config) => {
      if (this.token) {
        config.headers.Authorization = `Bearer ${this.token}`;
      }
      return config;
    });
  }

  /** Обновляет токен доступа (после входа или выхода). */
  setToken(token: string | null): void {
    this.token = token;
  }

  /** Вход в систему. */
  async login(username: string, password: string): Promise<LoginResult> {
    const response = await this.http.post('/auth/login', { username, password });
    if (response.status !== 200) {
      throw toApiError(response.status, response.data, new Error('login'));
    }
    return response.data as LoginResult;
  }

  /**
   * Проверка доступности сервера.
   *
   * Используем публичный /health: он отвечает без авторизации, поэтому
   * проверку можно делать до входа и прямо на экране добавления сервера.
   * Ответ содержит версию, по ней можно судить, что это именно наш сервер.
   */
  async health(): Promise<{ status: string }> {
    const response = await axios.get(
      `${this.baseUrl.replace(/\/+$/, '')}/health`,
      { timeout: 8000, validateStatus: () => true },
    );
    if (response.status !== 200) {
      throw toApiError(response.status, response.data, new Error('health'));
    }
    return response.data as { status: string };
  }

  /** Список камер. */
  async listCameras(): Promise<Camera[]> {
    const response = await this.http.get('/cameras');
    if (response.status !== 200) {
      throw toApiError(response.status, response.data, new Error('cameras'));
    }
    return (response.data ?? []) as Camera[];
  }

  /** Адреса потоков одной камеры. */
  async getStream(cameraId: string): Promise<StreamInfo> {
    const response = await this.http.get(`/cameras/${cameraId}/stream`);
    if (response.status !== 200) {
      throw toApiError(response.status, response.data, new Error('stream'));
    }
    return response.data as StreamInfo;
  }

  /** Страница архива записей. */
  async listRecordings(params: {
    cameraId?: string;
    page?: number;
    pageSize?: number;
    trigger?: string;
  }): Promise<RecordingsPage> {
    const response = await this.http.get('/recordings', {
      params: {
        camera_id: params.cameraId,
        page: params.page ?? 1,
        page_size: params.pageSize ?? 20,
        trigger: params.trigger || undefined,
      },
    });
    if (response.status !== 200) {
      throw toApiError(response.status, response.data, new Error('recordings'));
    }
    return response.data as RecordingsPage;
  }

  /** Одна запись со свежей ссылкой на файл. */
  async getRecording(id: string): Promise<Recording> {
    const response = await this.http.get(`/recordings/${id}`);
    if (response.status !== 200) {
      throw toApiError(response.status, response.data, new Error('recording'));
    }
    return response.data as Recording;
  }
}

/**
 * Превращает ответ сервера в понятную ошибку.
 *
 * Сервер отвечает `{"error": "..."}` на английском, поэтому частые случаи
 * переводим сами. Остальное показываем как есть: подменять незнакомый
 * текст своим хуже, чем показать оригинал — по нему можно искать причину.
 */
export function toApiError(
  status: number,
  data: unknown,
  original: Error,
): Error {
  const serverMessage =
    data && typeof data === 'object' && 'error' in data
      ? String((data as { error: unknown }).error)
      : '';

  if (status === 401) {
    return new Error('Неверный логин или пароль');
  }
  if (status === 403) {
    return new Error('Доступ запрещён: у пользователя нет прав');
  }
  if (status === 404) {
    return new Error('Не найдено на сервере');
  }
  if (status >= 500) {
    return new Error(
      serverMessage
        ? `Ошибка сервера: ${serverMessage}`
        : `Сервер ответил ошибкой ${status}`,
    );
  }
  if (serverMessage) {
    return new Error(serverMessage);
  }
  return original;
}

/**
 * Превращает ошибку сети в понятное сообщение.
 *
 * Отдельная функция нужна потому, что сбои соединения — самая частая
 * проблема при подключении к камерам, и «Network Error» пользователю
 * ничего не объясняет.
 */
export function describeNetworkError(error: unknown): string {
  if (axios.isAxiosError(error)) {
    const axiosError = error as AxiosError;
    if (axiosError.code === 'ECONNABORTED') {
      return 'Сервер не ответил вовремя. Проверьте адрес и доступность порта.';
    }
    if (axiosError.code === 'ERR_NETWORK') {
      return 'Не удалось соединиться. Проверьте адрес, порт и то, что телефон в той же сети.';
    }
    if (axiosError.response) {
      return toApiError(
        axiosError.response.status,
        axiosError.response.data,
        axiosError,
      ).message;
    }
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return 'Неизвестная ошибка';
}
