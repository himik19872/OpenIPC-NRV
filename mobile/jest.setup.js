/**
 * Настройка тестов.
 *
 * Нативные модули React Native в Node не работают: они обращаются к
 * платформе Android или iOS. Ниже — заглушки для тех, что используются
 * в коде; без них тесты падают ещё до выполнения.
 */

// Хранилище: подменяем на память. Тесты логики не должны зависеть от
// файловой системы телефона.
jest.mock('@react-native-async-storage/async-storage', () => {
  const store = new Map();
  return {
    __esModule: true,
    default: {
      getItem: jest.fn(async (key) => (store.has(key) ? store.get(key) : null)),
      setItem: jest.fn(async (key, value) => {
        store.set(key, value);
      }),
      removeItem: jest.fn(async (key) => {
        store.delete(key);
      }),
      clear: jest.fn(async () => {
        store.clear();
      }),
    },
  };
});

// Видеоплеер: полноценный плеер в тестах не нужен, достаточно заглушки,
// чтобы модуль импортировался без обращения к нативной части.
jest.mock('react-native-video', () => 'Video');
