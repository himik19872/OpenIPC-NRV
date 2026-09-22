#pragma once

#include "esp_err.h"
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

#define EVENT_LOG_MAX          1000    /* max events stored in ring buffer */

/* Event source / type */
typedef enum {
    EVT_CARD_GRANTED,       /* valid card -> door opened */
    EVT_CARD_DENIED,        /* unknown/blocked card */
    EVT_RTE_PRESSED,        /* request-to-exit button */
    EVT_DOOR_OPEN,          /* reed switch: door opened */
    EVT_DOOR_CLOSED,        /* reed switch: door closed */
    EVT_DOOR_FORCED,        /* door opened without grant (alarm) */
    EVT_SYSTEM_START,       /* controller boot */
    EVT_AUTH_FAIL,          /* failed login attempt */
    EVT_CONFIG_CHANGED      /* config updated via web */
} event_type_t;

/* A single event record (persisted) */
typedef struct {
    uint32_t    ts;         /* unix timestamp (seconds) */
    uint8_t     type;
    uint8_t     facility;   /* Wiegand facility code (cards only) */
    uint16_t    card;       /* card number (cards only) */
    char        note[32];   /* short free-form note */
    uint8_t     flags;      /* bit0 = pass completed (door opened) */
} event_record_t;

/* Init the event log */
esp_err_t event_log_init(void);

/* Append an event (writes through to NVS ring) */
esp_err_t event_log_add(event_type_t type, uint8_t facility, uint16_t card,
                        const char *note);

/* Append with pass-completion flag (door opened after grant) */
esp_err_t event_log_add_pass(event_type_t type, uint8_t facility, uint16_t card,
                             const char *note, bool pass_completed);

/* Get number of stored events */
int event_log_count(void);

/* Get event at logical index 0..count-1 (0 = oldest) */
esp_err_t event_log_get(int idx, event_record_t *out);

/* Mark the most recent CARD_GRANTED event as pass-completed (door opened). */
esp_err_t event_log_mark_last_pass(void);

/* Clear all events */
esp_err_t event_log_clear(void);

#ifdef __cplusplus
}
#endif
