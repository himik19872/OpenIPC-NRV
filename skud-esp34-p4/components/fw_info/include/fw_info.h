#pragma once

// Версия прошивки контроллера.
//
// Нужна для OTA: по ней видно, что установлено на устройстве и обновилось
// ли оно после прошивки. Без версии обновление превращается в проверку
// вслепую — нельзя отличить успешную прошивку от неудачной.
//
// Правило изменения: FW_VERSION_MAJOR.MINOR.PATCH поднимается вручную при
// выпуске. Автоматическая подстановка даты сборки не подходит: две сборки
// одного дня было бы невозможно различить.

#define FW_VERSION_MAJOR 1
#define FW_VERSION_MINOR 0
#define FW_VERSION_PATCH 10

// Строковое представление версии: "1.0.0".
#define FW_STRINGIFY_(x) #x
#define FW_STRINGIFY(x) FW_STRINGIFY_(x)
#define FW_VERSION_STRING \
    FW_STRINGIFY(FW_VERSION_MAJOR) "." \
    FW_STRINGIFY(FW_VERSION_MINOR) "." \
    FW_STRINGIFY(FW_VERSION_PATCH)

// Дата и время сборки — дополнительный ориентир, если версия не менялась.
#define FW_BUILD_DATE __DATE__ " " __TIME__
