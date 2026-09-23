module.exports = {
  preset: 'react-native',
  // Общие заглушки нативных модулей: без них тесты падают на этапе импорта.
  setupFiles: ['<rootDir>/jest.setup.js'],
  // Файлы с исходниками и тестами — только TypeScript.
  moduleFileExtensions: ['ts', 'tsx', 'js', 'jsx', 'json', 'node'],
};
