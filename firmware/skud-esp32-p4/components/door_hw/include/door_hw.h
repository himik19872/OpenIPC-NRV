#pragma once

#include "esp_err.h"
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Callback kinds from door hardware */
typedef enum {
    DOOR_EVT_RTE_PRESSED,        /* request-to-exit button pressed */
    DOOR_EVT_SENSOR_OPEN,        /* door opened (reed switch) */
    DOOR_EVT_SENSOR_CLOSED       /* door closed */
} door_event_t;

typedef void (*door_cb_t)(door_event_t evt, void *user_ctx);

/* Configure and init door hardware (lock + optional RTE button + sensor) */
esp_err_t door_hw_init(int lock_gpio, int rte_gpio, int sensor_gpio,
                       uint32_t pulse_ms, door_cb_t cb, void *user_ctx);

/* Release any resources; call before deinit/reboot */
esp_err_t door_hw_deinit(void);

/* Trigger an unlock pulse (returns immediately; lock auto-relocks after pulse) */
esp_err_t door_hw_open(void);

/* Direct control of the lock relay (true = energized/unlocked) */
esp_err_t door_hw_set_lock(bool on);

/* Read current door sensor state (true = open) */
bool door_hw_door_open(void);

#ifdef __cplusplus
}
#endif
