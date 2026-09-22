#pragma once

#include "esp_err.h"
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

#define CARD_DB_MAX         20000
#define CARD_NAME_MAX       48
#define CARD_GROUP_MAX      24

/* Access type */
typedef enum {
    CARD_ACCESS_PERMANENT = 0,   /* always */
    CARD_ACCESS_SCHEDULED = 1    /* only within schedules */
} card_access_t;

/* A single card record */
typedef struct {
    uint8_t  facility;
    uint16_t card;
    char     name[CARD_NAME_MAX];      /* ФИО */
    uint8_t  access;                    /* CARD_ACCESS_* */
    char     group[CARD_GROUP_MAX];     /* optional group label */
    bool     active;                    /* enabled/disabled */
} card_record_t;

/* Init the card database */
esp_err_t card_db_init(void);

/* Get number of stored cards */
int card_db_count(void);

/* Look up a card by facility+card. Returns index or -1 if not found. */
int card_db_find(uint8_t facility, uint16_t card);

/* Get card at index 0..count-1 (sorted insertion order). */
esp_err_t card_db_get(int idx, card_record_t *out);

/* Add a new card. Returns ESP_OK or ESP_ERR_NO_MEM (full) / exists. */
esp_err_t card_db_add(const card_record_t *rec);

/* Update an existing card (by facility+card). */
esp_err_t card_db_update(const card_record_t *rec);

/* Remove a card by facility+card. */
esp_err_t card_db_remove(uint8_t facility, uint16_t card);

/* Remove all cards. */
esp_err_t card_db_clear(void);

/* ---- Learn mode: capture next card read and add it to DB ---- */

/* Arm learn mode with a pending name. Next card_db_learn_capture() adds it. */
void card_db_learn_arm(const char *name);

/* Disarm learn mode. */
void card_db_learn_cancel(void);

/* Returns true if learn mode is armed. */
bool card_db_learn_active(void);

/* Attempt to learn a card (called from card callback). If learn mode is
 * armed, adds the card to DB and disarms. Returns true if consumed. */
bool card_db_learn_capture(uint8_t facility, uint16_t card);

#ifdef __cplusplus
}
#endif
