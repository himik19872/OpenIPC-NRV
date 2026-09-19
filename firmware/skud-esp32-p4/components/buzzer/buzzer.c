#include "buzzer.h"
#include "esp_log.h"
#include "driver/ledc.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "buzzer";

static int s_gpio = -1;
static bool s_init = false;
static int s_ledc_timer = -1;

esp_err_t buzzer_init(int gpio)
{
    s_gpio = gpio;
    if (gpio < 0) {
        ESP_LOGI(TAG, "Buzzer disabled");
        return ESP_OK;
    }

    ledc_timer_config_t tcfg = {
        .speed_mode = LEDC_LOW_SPEED_MODE,
        .duty_resolution = LEDC_TIMER_10_BIT,
        .timer_num = LEDC_TIMER_0,
        .freq_hz = 2000,
        .clk_cfg = LEDC_AUTO_CLK,
    };
    ledc_timer_config(&tcfg);
    s_ledc_timer = LEDC_TIMER_0;

    ledc_channel_config_t ccfg = {
        .gpio_num = gpio,
        .speed_mode = LEDC_LOW_SPEED_MODE,
        .channel = LEDC_CHANNEL_0,
        .intr_type = LEDC_INTR_DISABLE,
        .timer_sel = LEDC_TIMER_0,
        .duty = 0,
        .hpoint = 0,
    };
    ledc_channel_config(&ccfg);

    s_init = true;
    ESP_LOGI(TAG, "Buzzer init on GPIO%d", gpio);
    return ESP_OK;
}

esp_err_t buzzer_deinit(void)
{
    if (s_init && s_gpio >= 0) {
        ledc_stop(LEDC_LOW_SPEED_MODE, LEDC_CHANNEL_0, 0);
        s_init = false;
    }
    s_gpio = -1;
    return ESP_OK;
}

static void set_duty(uint32_t duty)
{
    ledc_set_duty(LEDC_LOW_SPEED_MODE, LEDC_CHANNEL_0, duty);
    ledc_update_duty(LEDC_LOW_SPEED_MODE, LEDC_CHANNEL_0);
}

esp_err_t buzzer_tone(int freq_hz)
{
    if (!s_init || s_gpio < 0) return ESP_OK;
    ledc_set_freq(LEDC_LOW_SPEED_MODE, LEDC_TIMER_0, freq_hz);
    set_duty(512); /* 50% */
    return ESP_OK;
}

esp_err_t buzzer_stop(void)
{
    if (!s_init || s_gpio < 0) return ESP_OK;
    set_duty(0);
    return ESP_OK;
}

esp_err_t buzzer_beep(int n, int freq_hz, int dur_ms, int gap_ms)
{
    if (!s_init || s_gpio < 0) return ESP_OK;
    for (int i = 0; i < n; i++) {
        buzzer_tone(freq_hz);
        vTaskDelay(pdMS_TO_TICKS(dur_ms));
        buzzer_stop();
        if (i < n - 1) vTaskDelay(pdMS_TO_TICKS(gap_ms));
    }
    return ESP_OK;
}
