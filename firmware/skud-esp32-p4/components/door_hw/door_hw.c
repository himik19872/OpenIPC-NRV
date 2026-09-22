#include <string.h>
#include "door_hw.h"
#include "esp_log.h"
#include "driver/gpio.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/queue.h"

static const char *TAG = "door_hw";

#define LOCK_ACTIVE_LEVEL   1   /* relay active-high by default */

static struct {
    int lock_gpio;
    int rte_gpio;
    int sensor_gpio;
    uint32_t pulse_ms;
    esp_timer_handle_t relock_timer;
    door_cb_t cb;
    void *user_ctx;
    volatile bool lock_on;
} s_ctx;

/* debounce task queue for button/sensor events */
static QueueHandle_t s_evt_queue = NULL;
static bool s_isr_installed = false;
static bool s_init_done = false;

static void install_isr(void)
{
    if (s_isr_installed) return;
    esp_err_t err = gpio_install_isr_service(0);
    if (err == ESP_OK || err == ESP_ERR_INVALID_STATE) {
        s_isr_installed = true;
    }
}

static void relock_timer_cb(void *arg)
{
    /* Auto-relock after pulse */
    door_hw_set_lock(false);
}

static void IRAM_ATTR rte_isr(void *arg)
{
    door_event_t e = DOOR_EVT_RTE_PRESSED;
    xQueueSendFromISR(s_evt_queue, &e, NULL);
}

static void IRAM_ATTR sensor_isr(void *arg)
{
    /* determine open/closed from level */
    if (s_ctx.sensor_gpio >= 0) {
        int lvl = gpio_get_level(s_ctx.sensor_gpio);
        door_event_t e = lvl ? DOOR_EVT_SENSOR_OPEN : DOOR_EVT_SENSOR_CLOSED;
        xQueueSendFromISR(s_evt_queue, &e, NULL);
    }
}

static void door_task(void *arg)
{
    door_event_t e;
    while (1) {
        if (xQueueReceive(s_evt_queue, &e, portMAX_DELAY) == pdTRUE) {
            if (s_ctx.cb) {
                s_ctx.cb(e, s_ctx.user_ctx);
            }
        }
    }
}

esp_err_t door_hw_init(int lock_gpio, int rte_gpio, int sensor_gpio,
                       uint32_t pulse_ms, door_cb_t cb, void *user_ctx)
{
    /* idempotent: if already initialized, release previous resources */
    if (s_init_done) {
        door_hw_deinit();
    }

    s_ctx.lock_gpio = lock_gpio;
    s_ctx.rte_gpio = rte_gpio;
    s_ctx.sensor_gpio = sensor_gpio;
    s_ctx.pulse_ms = pulse_ms;
    s_ctx.cb = cb;
    s_ctx.user_ctx = user_ctx;
    s_ctx.lock_on = false;

    /* Event queue + task (create once) */
    if (s_evt_queue == NULL) {
        s_evt_queue = xQueueCreate(16, sizeof(door_event_t));
        xTaskCreate(door_task, "door_task", 4096, NULL, 5, NULL);
    }

    /* Lock relay output */
    if (lock_gpio >= 0) {
        gpio_config_t io = {
            .pin_bit_mask = (1ULL << lock_gpio),
            .mode = GPIO_MODE_OUTPUT,
            .pull_up_en = GPIO_PULLUP_DISABLE,
            .pull_down_en = GPIO_PULLDOWN_DISABLE,
            .intr_type = GPIO_INTR_DISABLE,
        };
        gpio_config(&io);
        gpio_set_level(lock_gpio, !LOCK_ACTIVE_LEVEL);
    }

    /* RTE button input (active-low, pull-up) */
    if (rte_gpio >= 0) {
        gpio_config_t io = {
            .pin_bit_mask = (1ULL << rte_gpio),
            .mode = GPIO_MODE_INPUT,
            .pull_up_en = GPIO_PULLUP_ENABLE,
            .pull_down_en = GPIO_PULLDOWN_DISABLE,
            .intr_type = GPIO_INTR_NEGEDGE,
        };
        gpio_config(&io);
        install_isr();
        gpio_isr_handler_add(rte_gpio, rte_isr, NULL);
    }

    /* Door sensor input (any edge) */
    if (sensor_gpio >= 0) {
        gpio_config_t io = {
            .pin_bit_mask = (1ULL << sensor_gpio),
            .mode = GPIO_MODE_INPUT,
            .pull_up_en = GPIO_PULLUP_ENABLE,
            .pull_down_en = GPIO_PULLDOWN_DISABLE,
            .intr_type = GPIO_INTR_ANYEDGE,
        };
        gpio_config(&io);
        install_isr();
        gpio_isr_handler_add(sensor_gpio, sensor_isr, NULL);
    }

    /* Relock timer (create once) */
    if (s_ctx.relock_timer == NULL) {
        esp_timer_create_args_t t = {
            .callback = relock_timer_cb,
            .arg = NULL,
            .name = "relock",
        };
        esp_timer_create(&t, &s_ctx.relock_timer);
    }

    ESP_LOGI(TAG, "Door HW init: lock=%d rte=%d sensor=%d",
             lock_gpio, rte_gpio, sensor_gpio);
    s_init_done = true;
    return ESP_OK;
}

esp_err_t door_hw_deinit(void)
{
    if (s_ctx.relock_timer) {
        esp_timer_stop(s_ctx.relock_timer);
        esp_timer_delete(s_ctx.relock_timer);
        s_ctx.relock_timer = NULL;
    }
    if (s_isr_installed) {
        if (s_ctx.rte_gpio >= 0) gpio_isr_handler_remove(s_ctx.rte_gpio);
        if (s_ctx.sensor_gpio >= 0) gpio_isr_handler_remove(s_ctx.sensor_gpio);
    }
    s_ctx.rte_gpio = -1;
    s_ctx.sensor_gpio = -1;
    s_ctx.lock_gpio = -1;
    s_init_done = false;
    return ESP_OK;
}

esp_err_t door_hw_open(void)
{
    if (s_ctx.lock_gpio < 0) return ESP_ERR_INVALID_STATE;

    door_hw_set_lock(true);
    /* schedule relock */
    if (s_ctx.pulse_ms > 0) {
        esp_timer_stop(s_ctx.relock_timer);
        esp_timer_start_once(s_ctx.relock_timer, s_ctx.pulse_ms * 1000);
    }
    return ESP_OK;
}

esp_err_t door_hw_set_lock(bool on)
{
    if (s_ctx.lock_gpio < 0) return ESP_ERR_INVALID_STATE;
    s_ctx.lock_on = on;
    gpio_set_level(s_ctx.lock_gpio, on ? LOCK_ACTIVE_LEVEL : !LOCK_ACTIVE_LEVEL);
    ESP_LOGI(TAG, "Lock %s", on ? "UNLOCKED" : "LOCKED");
    return ESP_OK;
}

bool door_hw_door_open(void)
{
    if (s_ctx.sensor_gpio < 0) return false;
    return gpio_get_level(s_ctx.sensor_gpio) != 0;
}
