#pragma once

#include "esp_err.h"
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Begin an OTA update (finds next OTA partition and erases it).
 * After this, feed data with ota_write() and finish with ota_commit(). */
esp_err_t ota_begin(void);

/* Write a chunk of the new firmware image. */
esp_err_t ota_write(const uint8_t *data, size_t len);

/* Finalize the update and reboot to the new image. */
esp_err_t ota_commit(void);

/* Abort an in-progress update. */
void ota_abort(void);

#ifdef __cplusplus
}
#endif
