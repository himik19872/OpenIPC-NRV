import AsyncStorage from '@react-native-async-storage/async-storage';
import type { ServerProfile } from '../types';

/**
 * Локальное хранилище приложения.
 *
 * Здесь лежат список серверов, выбранный сервер и токены доступа.
 * Токены хранятся в AsyncStorage, а не в защищённом хранилище: это
 * осознанный выбор ради отсутствия лишних зависимостей. На устройстве
 * без root файлы приложения недоступны другим программам, но при
 * переходе на публичный сервер стоит заменить хранилище на
 * react-native-keychain.
 */

const KEY_SERVERS = '@nvr/servers';
const KEY_SELECTED = '@nvr/selected_server';
const KEY_TOKEN_PREFIX = '@nvr/token/';

/** Читает список сохранённых серверов. */
export async function loadServers(): Promise<ServerProfile[]> {
  try {
    const raw = await AsyncStorage.getItem(KEY_SERVERS);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    // Проверяем форму данных: файл мог остаться от другой версии приложения.
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (s): s is ServerProfile =>
        !!s && typeof s.id === 'string' && typeof s.baseUrl === 'string',
    );
  } catch {
    return [];
  }
}

/** Сохраняет список серверов целиком. */
export async function saveServers(servers: ServerProfile[]): Promise<void> {
  await AsyncStorage.setItem(KEY_SERVERS, JSON.stringify(servers));
}

/** Идентификатор выбранного сервера или null, если выбора ещё не было. */
export async function loadSelectedServerId(): Promise<string | null> {
  try {
    return await AsyncStorage.getItem(KEY_SELECTED);
  } catch {
    return null;
  }
}

export async function saveSelectedServerId(id: string | null): Promise<void> {
  if (id === null) {
    await AsyncStorage.removeItem(KEY_SELECTED);
    return;
  }
  await AsyncStorage.setItem(KEY_SELECTED, id);
}

/**
 * Токен доступа хранится отдельно для каждого сервера.
 *
 * Это важно при работе с несколькими серверами: переключение между ними
 * не должно требовать повторного входа, а выход с одного сервера не
 * затрагивает остальные.
 */
export async function loadToken(serverId: string): Promise<string | null> {
  try {
    return await AsyncStorage.getItem(KEY_TOKEN_PREFIX + serverId);
  } catch {
    return null;
  }
}

export async function saveToken(serverId: string, token: string): Promise<void> {
  await AsyncStorage.setItem(KEY_TOKEN_PREFIX + serverId, token);
}

export async function clearToken(serverId: string): Promise<void> {
  await AsyncStorage.removeItem(KEY_TOKEN_PREFIX + serverId);
}
