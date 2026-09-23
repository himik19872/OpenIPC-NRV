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
 * Адрес разбирается вручную, без класса URL. В браузере он есть, но в
 * Hermes — движке JavaScript в React Native — реализован частично и падает
 * с ошибкой «URL.hostname is not implemented». Регулярные выражения
 * работают везде одинаково.
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

  // Схема: http:// или https://. Всё остальное (в том числе ftp://) не
  // поддерживается: сервер отвечает по HTTP.
  const schemeMatch = /^(https?):\/\//i.exec(trimmed);
  const scheme = schemeMatch ? schemeMatch[1].toLowerCase() : 'http';

  // Остаток адреса после схемы: «хост:порт/путь?запрос».
  let rest = schemeMatch ? trimmed.slice(schemeMatch[0].length) : trimmed;

  // Путь и параметры отбрасываем: пользователь вводит адрес сервера, а
  // пути API приложение дописывает само.
  rest = rest.split(/[/?#]/, 1)[0];
  if (!rest) return null;

  /**
   * Отделяем порт от хоста.
   *
   * Случаи: `192.168.1.10`, `192.168.1.10:8080`, `nvr.local:9000` и
   * IPv6 в квадратных скобках — `[::1]:8080`.
   */
  let hostPart: string;
  let portPart: string | null = null;

  const ipv6Match = /^\[([^\]]+)\](?::(\d+))?$/.exec(rest);
  if (ipv6Match) {
    hostPart = ipv6Match[1];
    portPart = ipv6Match[2] ?? null;
  } else {
    const lastColon = rest.lastIndexOf(':');
    // Двоеточие есть и после него только цифры — это порт. Иначе двоеточие
    // принадлежит адресу IPv6, записанному без скобок.
    if (lastColon > 0 && /^\d+$/.test(rest.slice(lastColon + 1))) {
      hostPart = rest.slice(0, lastColon);
      portPart = rest.slice(lastColon + 1);
    } else {
      hostPart = rest;
    }
  }

  if (!hostPart) return null;

  // Проверяем, что хост похож на адрес или доменное имя. Отсекаем случаи
  // вроде «http://-» или «http://..», которые формально разбираются, а в
  // сети не существуют.
  const looksLikeIPv4 = /^\d{1,3}(\.\d{1,3}){3}$/.test(hostPart);
  const looksLikeIPv6 = hostPart.includes(':');
  const looksLikeHostname =
    /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/i.test(
      hostPart,
    );
  if (!looksLikeIPv4 && !looksLikeIPv6 && !looksLikeHostname) return null;

  // Для IPv4 проверяем диапазон: 999.1.1.1 формально проходит регулярное
  // выражение, но адресом не является.
  if (looksLikeIPv4) {
    const parts = hostPart.split('.').map(Number);
    if (parts.some((part) => part > 255)) return null;
  }

  // Явно указанный порт уважаем. Если порта нет, берём стандартный для
  // схемы: для https это 443 (пользователь написал схему осознанно), для
  // http — порт сервера NVR.
  const defaultPortForScheme = scheme === 'https' ? 443 : DEFAULT_PORT;
  const port = portPart === null ? defaultPortForScheme : Number(portPart);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return null;

  // Порт не пишем в адрес, если он стандартный для схемы: так ссылка
  // выглядит привычнее, а браузер и сервер понимают её одинаково.
  const isDefaultPort =
    (scheme === 'http' && port === 80) || (scheme === 'https' && port === 443);
  const hostForUrl = looksLikeIPv6 ? `[${hostPart}]` : hostPart;
  const baseUrl = `${scheme}://${hostForUrl}${isDefaultPort ? '' : `:${port}`}`;

  // В подписи для интерфейса порт показываем, если он написан явно и не
  // стандартный: «192.168.1.10:8080» понятнее, чем просто адрес.
  const showPort = portPart !== null && !isDefaultPort;
  const display = showPort ? `${hostForUrl}:${port}` : hostForUrl;

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
    if (/^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:|\/|$)/i.test(path)) {
      /*
       * Подменяем хост и порт на выбранный сервер, путь оставляем: он
       * принадлежит API. Разбираем строкой, а не классом URL — в Hermes
       * он не реализован.
       */
      const withoutScheme = path.replace(/^https?:\/\//i, '');
      const slashIndex = withoutScheme.search(/[/?#]/);
      if (slashIndex < 0) {
        // Путь отсутствует: остаётся только адрес сервера.
        return baseUrl.replace(/\/+$/, '');
      }
      const rest = withoutScheme.slice(slashIndex);
      return `${baseUrl.replace(/\/+$/, '')}${rest}`;
    }
    return path;
  }

  const base = baseUrl.replace(/\/+$/, '');
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${base}${suffix}`;
}
