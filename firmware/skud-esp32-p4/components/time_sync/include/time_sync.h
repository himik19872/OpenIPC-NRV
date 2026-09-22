#pragma once

#include "esp_err.h"
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Start SNTP with given server and periodic re-sync interval (seconds).
 * interval 0 = no periodic re-sync (manual only). */
esp_err_t time_sync_start(const char *server, uint32_t interval_sec);

/* Set local timezone (offset in minutes from UTC). Applies immediately. */
void time_sync_set_timezone(int16_t offset_min);

/* Trigger an immediate manual sync (non-blocking). */
esp_err_t time_sync_now(void);

/* Return true if system time is currently synchronized (RTC valid). */
bool time_sync_synced(void);

#ifdef __cplusplus
}
#endif
