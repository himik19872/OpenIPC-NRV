#pragma once

#include "esp_err.h"
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Event push to the NVR/central server (MODE_NETWORK).
 *
 * When configured, pushes access events as JSON (HTTP POST) to:
 *   http://<server_host>:<server_port><server_path>
 *
 * The push is best-effort and non-blocking: events are queued and sent by a
 * background task. Failures are logged, not fatal.
 */

/* Start the push task + queue. Safe to call multiple times (idempotent). */
esp_err_t event_push_init(void);

/* Configure the push target. Call with enabled=false to disable. */
esp_err_t event_push_configure(const char *host, uint16_t port, const char *path,
                               const char *device_id, bool enabled);

/* Queue an event for delivery. Non-blocking. type = event_type_t numeric. */
esp_err_t event_push_send(int type, uint8_t facility,
                          uint16_t card, const char *name, uint32_t ts,
                          int flags);

#ifdef __cplusplus
}
#endif
