/**
 * Проверка разбора адреса сервера.
 *
 * Функция разбирает адрес вручную, без класса URL: в Hermes он не
 * реализован и приложение падало с ошибкой «URL.hostname is not
 * implemented». Тесты фиксируют поведение и защищают от возврата к
 * использованию URL.
 */

import { parseAddress, joinUrl, DEFAULT_PORT } from '../src/net/address';

describe('parseAddress', () => {
  it('подставляет http и порт по умолчанию для адреса без порта', () => {
    const result = parseAddress('192.168.1.10');
    expect(result).not.toBeNull();
    expect(result!.baseUrl).toBe(`http://192.168.1.10:${DEFAULT_PORT}`);
    expect(result!.display).toBe('192.168.1.10');
  });

  it('сохраняет явно указанный порт', () => {
    expect(parseAddress('192.168.1.10:9000')!.baseUrl).toBe(
      'http://192.168.1.10:9000',
    );
  });

  it('распознаёт доменное имя', () => {
    expect(parseAddress('nvr.local')!.baseUrl).toBe(`http://nvr.local:${DEFAULT_PORT}`);
  });

  it('уважает явно указанную схему', () => {
    expect(parseAddress('https://cam.example.com')!.baseUrl).toBe(
      'https://cam.example.com',
    );
  });

  it('не пишет порт 443 для https и 80 для http', () => {
    expect(parseAddress('https://cam.example.com:443')!.baseUrl).toBe(
      'https://cam.example.com',
    );
    expect(parseAddress('http://cam.example.com:80')!.baseUrl).toBe(
      'http://cam.example.com',
    );
  });

  it('отбрасывает путь и параметры', () => {
    expect(parseAddress('192.168.1.10:8080/api/v1?x=1')!.baseUrl).toBe(
      'http://192.168.1.10:8080',
    );
  });

  it('поддерживает адрес IPv6 в скобках', () => {
    expect(parseAddress('[::1]:8080')!.baseUrl).toBe('http://[::1]:8080');
  });

  it('показывает порт в подписи, если он указан явно', () => {
    expect(parseAddress('192.168.1.10:8080')!.display).toBe('192.168.1.10:8080');
  });

  it('отклоняет пустой ввод и пробелы внутри адреса', () => {
    expect(parseAddress('')).toBeNull();
    expect(parseAddress('   ')).toBeNull();
    expect(parseAddress('192.168 1.10')).toBeNull();
  });

  it('отклоняет недопустимые адреса', () => {
    // 999 не помещается в октет адреса.
    expect(parseAddress('999.1.1.1')).toBeNull();
    // Порт вне диапазона.
    expect(parseAddress('192.168.1.10:99999')).toBeNull();
    // Хост из одних дефисов.
    expect(parseAddress('-')).toBeNull();
  });
});

describe('joinUrl', () => {
  const base = 'http://192.168.1.10:8080';

  it('приклеивает относительный путь к адресу сервера', () => {
    expect(joinUrl(base, '/api/v1/cameras')).toBe(
      'http://192.168.1.10:8080/api/v1/cameras',
    );
  });

  it('добавляет слэш, если путь его не содержит', () => {
    expect(joinUrl(base, 'api/v1/cameras')).toBe(
      'http://192.168.1.10:8080/api/v1/cameras',
    );
  });

  it('не создаёт двойной слэш при завершающем слэше в адресе', () => {
    expect(joinUrl('http://192.168.1.10:8080/', '/api/v1')).toBe(
      'http://192.168.1.10:8080/api/v1',
    );
  });

  it('возвращает пустую строку, если пути нет', () => {
    expect(joinUrl(base, undefined)).toBe('');
    expect(joinUrl(base, '')).toBe('');
  });

  it('оставляет внешнюю абсолютную ссылку без изменений', () => {
    expect(joinUrl(base, 'http://example.com/a.mp4')).toBe(
      'http://example.com/a.mp4',
    );
  });

  it('подменяет localhost на адрес выбранного сервера', () => {
    // Сервер отдаёт такие ссылки в своей конфигурации, с телефона они
    // указывают на сам телефон и заведомо не работают.
    expect(joinUrl(base, 'http://localhost:8889/abc')).toBe(
      'http://192.168.1.10:8080/abc',
    );
    expect(joinUrl(base, 'http://127.0.0.1/abc')).toBe(
      'http://192.168.1.10:8080/abc',
    );
  });

  it('подменяет localhost даже когда путь содержит параметры', () => {
    expect(joinUrl(base, 'http://localhost:9000/nvr-recordings/a.mp4?X-Amz=1')).toBe(
      'http://192.168.1.10:8080/nvr-recordings/a.mp4?X-Amz=1',
    );
  });

  it('возвращает адрес сервера, если в ссылке на localhost нет пути', () => {
    expect(joinUrl(base, 'http://localhost:8080')).toBe(base);
  });
});
