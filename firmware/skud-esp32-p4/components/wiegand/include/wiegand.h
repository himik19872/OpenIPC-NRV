#pragma once

#include "esp_err.h"
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define WIEGAND_BITS_26     26

/* Result of a successful 26-bit Wiegand read */
typedef struct {
    uint8_t  facility;   /* 8-bit facility code */
    uint16_t card;       /* 16-bit card number */
} wiegand_card_t;

/* Callback invoked on each successfully decoded card */
typedef void (*wiegand_cb_t)(const wiegand_card_t *card, void *user_ctx);

/**
 * Initialize the Wiegand 26 interface on D0/D1 pins.
 * @param d0_gpio  GPIO number for DATA0 (wired-AND active low)
 * @param d1_gpio  GPIO number for DATA1
 * @param cb       callback for decoded cards
 * @param user_ctx optional user context passed to callback
 */
esp_err_t wiegand_init(int d0_gpio, int d1_gpio, wiegand_cb_t cb, void *user_ctx);

/**
 * Deinitialize the Wiegand interface.
 */
esp_err_t wiegand_deinit(void);

#ifdef __cplusplus
}
#endif
