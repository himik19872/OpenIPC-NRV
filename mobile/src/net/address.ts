/**
 * Разбор и нормализация адреса сервера.
 *
 * Пользователь вводит адрес как удобно: «192.168.1.10», «nvr.local:8080»,
 * «https://cam.example.com». Приложение приводит всё к одному виду, чтобы
 * дальше просто приклеивать к нему пути API.
 */

/** Порт по умолчанию, на котором слушает сервер NVR. */
export const DEFAULT_PORT = 8080;

export interface ParsedAddress {
  /** Полный базовый адрес без завершающего слэша. */
  baseUrl: string;
  /** Краткое описание для интерфейса: хост и порт. */
  display: string;
}

/**
 * Приводит введённый адрес к базовому виду.
 *
 * Правила:
 * - если схема не указана, подставляется http (у большинства серверов
 *   видеонаблюдения нет сертификата, а самоподписанный вызовет ошибку);
 * - если порт не указан, подставляется 8080 — порт сервера NVR;
 * - завершающий слэш убирается: иначе при склейке с путём получится «//api».
 *
 * Возвращает null, если адрес разобрать не удалось: проверку делает
 * вызывающий код и показывает пользователю понятную ошибку.
 */
export function parseAddress(input: string): ParsedAddress | null {
  const trimmed = input.trim();
  if (!trimmed) return null;

  // Пробел внутри адреса — почти всегда опечатка при вводе или склейка
  // двух адресов. Лучше отклонить, чем молча получить нерабочую ссылку.
  if (/\s/.test(trimmed)) return null;

  let withScheme = trimmed;
  const hasScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed);
  if (!hasScheme) {
    withScheme = `http://${trimmed}`;
  }

  let url: URL;
  try {
    url = new URL(withScheme);
  } catch {
    return null;
  }

  const host = url.hostname;
  if (!host) return null;

  // Проверяем, что хост похож на адрес или доменное имя. Отсекаем случаи
  // вроде «http://-» или «http://..», которые URL пропускает, а сеть — нет.
  const looksLikeIPv6 = host.includes(':');
  const looksLikeIPv4 = /^\d{1,3}(\.\d{1,3}){3}$/.test(host);
  const looksLikeHostname = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/i.test(host);
  if (!looksLikeIPv6 && !looksLikeIPv4 && !looksLikeHostname) return null;

  // Явно указанный порт уважаем, иначе подставляем порт сервера NVR.
  const port = url.port ? Number(url.port) : DEFAULT_PORT;
  if (!Number.isInteger(port) || port < 1 || port > 65535) return null;

  /**
   * Проверяем, нужно ли показывать порт.
   *
   * Схему в адрес дописывает это приложение (пользователь её не вводил),
   * но порт 80 он указать мог. Поэтому смотрим не на итоговую схему, а на
   * исходный ввод: если порт написан явно — показываем.
   */
  const hostHasPort = /:\d+(\/|$)/.test(trimmed) || /\]\s*:\d+/.test(trimmed);
  const showPort = hostHasPort && port !== 80 && port !== 443;

  const baseUrl = `${url.protocol}//${url.hostname}${port === 80 || port === 443 ? '' : `:${port}`}`;
  const display = showPort ? `${host}:${port}` : host;

  return { baseUrl, display };
}

/**
 * Собирает полный адрес из базового адреса сервера и пути от API.
 *
 * Пути API сервер отдаёт относительными (`/api/v1/...`), и с телефона их
 * нужно привязать к выбранному серверу. Если пришёл абсолютный адрес,
 * возвращаем его как есть — кроме случая, когда он указывает на localhost:
 * такой адрес означает сам телефон и с ним заведомо не работает.
 */
export function joinUrl(baseUrl: string, path?: string): string {
  if (!path) return '';

  // Абсолютная ссылка. Доверять ей нельзя, если она ведёт на localhost:
  // это адрес из конфигурации сервера, а не что-то доступное с телефона.
  if (/^https?:\/\//i.test(path)) {
    if (/^https?:\/\/(localhost|127\.0\.0\.1)(:|\/|$)/i.test(path)) {
      // Подменяем только хост и порт, путь оставляем: он принадлежит API.
      try {
        const parsed = new URL(path);
        // Порт сервера NVR может отличаться от порта из ссылки, поэтому
        // берём его из базового адреса, а не из исходной строки.
        const base = new URL(baseUrl);
        return `${base.protocol}//${base.host}${parsed.pathname}${parsed.search}`;
      } catch {
        return path;
      }
    }
    return path;
  }

  const base = baseUrl.replace(/\/+$/, '');
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${base}${suffix}`;
}
