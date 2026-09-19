#pragma once

#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Mounts a littlefs filesystem on the "storage" partition at /storage.
 * Idempotent: safe to call multiple times. Must be called once at boot
 * before any card_db / event_log access.
 */
esp_err_t storage_init(void);

/* Returns true if the filesystem is mounted. */
bool storage_ready(void);

#ifdef __cplusplus
}
#endif
