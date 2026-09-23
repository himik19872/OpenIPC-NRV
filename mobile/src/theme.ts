/**
 * Оформление приложения.
 *
 * Цвета взяты из тёмной темы веб-интерфейса, чтобы приложение и сайт
 * выглядели как одно целое. Тёмная схема здесь ещё и практичнее: экран
 * смотрится в тёмном помещении, где обычно и ставят пульт охраны.
 */

export const colors = {
  background: '#0f1117',
  surface: '#171a23',
  surfaceAlt: '#1e222d',
  border: '#2a2f3d',

  text: '#e6e8ee',
  textSecondary: '#9aa3b2',
  textMuted: '#6b7280',

  primary: '#3b82f6',
  primaryPressed: '#2563eb',

  success: '#22c55e',
  warning: '#f59e0b',
  danger: '#ef4444',

  // Подсветка для оверлеев на видео: сплошной цвет здесь мешает смотреть.
  overlay: 'rgba(0, 0, 0, 0.55)',
};

export const spacing = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  xxl: 32,
};

export const radius = {
  sm: 6,
  md: 10,
  lg: 14,
};

/**
 * Цвета и подписи причин записи.
 *
 * Совпадают с веб-интерфейсом: оператор, который смотрит и сайт, и
 * телефон, должен узнавать записи по одному и тому же цвету.
 */
export const triggerColors: Record<string, string> = {
  face: '#8b5cf6',
  plate: '#ea580c',
  line: '#06b6d4',
  object: '#22c55e',
  always: '#6b7280',
  manual: '#6b7280',
};

export const triggerLabels: Record<string, string> = {
  face: 'Лицо',
  plate: 'Номер',
  line: 'Пересечение',
  object: 'Объект',
  always: 'Постоянно',
  manual: 'Вручную',
};
