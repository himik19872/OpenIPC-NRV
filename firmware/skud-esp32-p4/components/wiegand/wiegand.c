/*
 * wiegand.c
 * Wiegand 26 decoder using GPIO interrupts + esp_timer for the
 * inter-bit / end-of-message gap timeout.
 *
 * Wiegand 26 format:
 *   Bit 1      : even parity over bits 2..13
 *   Bits 2..9  : facility code (8 bits, MSB first)
 *   Bits 10..25: card number (16 bits, MSB first)
 *   Bit 26     : odd parity over bits 14..25
 */
#include <string.h>
#include "wiegand.h"
#include "esp_log.h"
#include "driver/gpio.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/portmacro.h"

#define WG_GAP_TIMEOUT_US      500     /* 350us typical; 500us safe */
#define WG_MAX_READERS         2

typedef struct {
    int d0_gpio;
    int d1_gpio;
    wiegand_cb_t cb;
    void *user_ctx;
    int  idx;                          /* reader index (0=IN, 1=OUT) */

    volatile uint32_t bits;         /* accumulated data bits (MSB first) */
    volatile uint8_t  bit_count;
    esp_timer_handle_t gap_timer;

    portMUX_TYPE lock;
} wiegand_ctx_t;

static wiegand_ctx_t s_ctx[WG_MAX_READERS];
static bool s_ctx_initialized = false;
static bool s_isr_installed = false;
static const char *TAG = "wiegand";

static void install_isr(void)
{
    if (s_isr_installed) return;
    esp_err_t err = gpio_install_isr_service(0);
    if (err == ESP_OK || err == ESP_ERR_INVALID_STATE) {
        s_isr_installed = true;
    }
}

static void ensure_init(void)
{
    if (!s_ctx_initialized) {
        for (int i = 0; i < WG_MAX_READERS; i++) {
            s_ctx[i].d0_gpio = -1;
            s_ctx[i].d1_gpio = -1;
        }
        s_ctx_initialized = true;
    }
}

static inline void ctx_lock(wiegand_ctx_t *c)   { portENTER_CRITICAL(&c->lock); }
static inline void ctx_unlock(wiegand_ctx_t *c) { portEXIT_CRITICAL(&c->lock); }

/* Compute parity of set bits in a mask over a range */
static uint8_t parity(uint32_t value, int count)
{
    uint8_t p = 0;
    for (int i = 0; i < count; i++) {
        p ^= (value >> i) & 1;
    }
    return p;
}

static void reset_frame(wiegand_ctx_t *c)
{
    c->bits = 0;
    c->bit_count = 0;
}

/* Called when an inter-bit gap expires -> frame must be complete */
static void gap_timeout_cb(void *arg)
{
    wiegand_ctx_t *c = (wiegand_ctx_t *)arg;
    ctx_lock(c);

    uint8_t count = c->bit_count;
    uint32_t bits = c->bits;

    if (count == WIEGAND_BITS_26) {
        /* bits contain the 26 bits with bit1 at bit position 25 */
        uint32_t b = bits;

        uint8_t p_even = parity(b >> 13, 12);
        uint8_t p_odd  = parity(b, 12);

        uint8_t  facility = (uint8_t)((b >> 17) & 0xFF);
        uint16_t card     = (uint16_t)((b >> 1) & 0xFFFF);

        uint32_t data = (facility << 16) | card;
        uint8_t expect_even = parity(data >> 12, 12);
        uint8_t expect_odd  = ~parity(data & 0xFFF, 12) & 1;

        if (p_even == expect_even && p_odd == expect_odd) {
            ESP_LOGI(TAG, "[%d] Card: facility=%u card=%u", c->idx, facility, card);
            if (c->cb) {
                wiegand_card_t cd = { .facility = facility, .card = card };
                ctx_unlock(c);
                c->cb(&cd, c->user_ctx);
                goto done;
            }
        } else {
            ESP_LOGW(TAG, "[%d] parity error (bits=0x%06x)", c->idx, bits);
        }
    } else if (count > 0) {
        ESP_LOGW(TAG, "[%d] incomplete frame: %u bits", c->idx, count);
    }

    ctx_unlock(c);
done:
    reset_frame(c);
}

static void IRAM_ATTR data_isr(void *arg, int bit)
{
    wiegand_ctx_t *c = (wiegand_ctx_t *)arg;
    ctx_lock(c);
    c->bits = (c->bits << 1) | (uint32_t)bit;
    c->bit_count++;
    esp_timer_stop(c->gap_timer);
    esp_timer_start_once(c->gap_timer, WG_GAP_TIMEOUT_US);
    ctx_unlock(c);
}

static void IRAM_ATTR d0_isr(void *arg)
{
    data_isr(arg, 0);
}

static void IRAM_ATTR d1_isr(void *arg)
{
    data_isr(arg, 1);
}

esp_err_t wiegand_init(int d0_gpio, int d1_gpio, wiegand_cb_t cb, void *user_ctx)
{
    ensure_init();

    int slot = -1;
    for (int i = 0; i < WG_MAX_READERS; i++) {
        if (s_ctx[i].d0_gpio < 0 && s_ctx[i].d1_gpio < 0) { slot = i; break; }
    }
    if (slot < 0) {
        ESP_LOGE(TAG, "no free reader slot");
        return ESP_ERR_NO_MEM;
    }

    wiegand_ctx_t *c = &s_ctx[slot];
    memset(c, 0, sizeof(*c));
    c->d0_gpio = d0_gpio;
    c->d1_gpio = d1_gpio;
    c->cb = cb;
    c->user_ctx = user_ctx;
    c->idx = slot;
    portMUX_INITIALIZE(&c->lock);

    char tname[16];
    snprintf(tname, sizeof(tname), "wg_gap%d", slot);
    esp_timer_create_args_t targs = {
        .callback = gap_timeout_cb,
        .arg = c,
        .name = tname,
    };
    esp_err_t ret = esp_timer_create(&targs, &c->gap_timer);
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Timer create failed");
        c->d0_gpio = c->d1_gpio = -1;
        return ret;
    }

    gpio_config_t io = {
        .pin_bit_mask = ((1ULL << d0_gpio) | (1ULL << d1_gpio)),
        .mode = GPIO_MODE_INPUT,
        .pull_up_en = GPIO_PULLUP_ENABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
        .intr_type = GPIO_INTR_NEGEDGE,
    };
    gpio_config(&io);

    install_isr();
    gpio_isr_handler_add(d0_gpio, d0_isr, c);
    gpio_isr_handler_add(d1_gpio, d1_isr, c);

    ESP_LOGI(TAG, "Wiegand 26 [%d] init: D0=%d D1=%d", slot, d0_gpio, d1_gpio);
    return ESP_OK;
}

esp_err_t wiegand_deinit(void)
{
    for (int i = 0; i < WG_MAX_READERS; i++) {
        wiegand_ctx_t *c = &s_ctx[i];
        if (c->gap_timer) {
            esp_timer_stop(c->gap_timer);
            esp_timer_delete(c->gap_timer);
            c->gap_timer = NULL;
        }
        if (s_isr_installed) {
            if (c->d0_gpio >= 0) { gpio_isr_handler_remove(c->d0_gpio); }
            if (c->d1_gpio >= 0) { gpio_isr_handler_remove(c->d1_gpio); }
        }
        c->d0_gpio = -1;
        c->d1_gpio = -1;
    }
    return ESP_OK;
}
