#include <string.h>
#include "ota.h"
#include "esp_log.h"
#include "esp_ota_ops.h"
#include "esp_partition.h"
#include "esp_app_format.h"
#include "esp_system.h"

static const char *TAG = "ota";

static esp_ota_handle_t s_handle = 0;
static const esp_partition_t *s_update_part = NULL;
static bool s_in_progress = false;

static void reset_state(void)
{
    s_handle = 0;
    s_update_part = NULL;
    s_in_progress = false;
}

esp_err_t ota_begin(void)
{
    const esp_partition_t *update = esp_ota_get_next_update_partition(NULL);
    if (!update) {
        ESP_LOGE(TAG, "No OTA update partition found");
        return ESP_ERR_NOT_FOUND;
    }
    ESP_LOGI(TAG, "OTA target: %s (size %lu)", update->label,
             (unsigned long)update->size);

    esp_err_t err = esp_ota_begin(update, OTA_WITH_SEQUENTIAL_WRITES, &s_handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_ota_begin failed: %s", esp_err_to_name(err));
        return err;
    }
    s_update_part = update;
    s_in_progress = true;
    return ESP_OK;
}

esp_err_t ota_write(const uint8_t *data, size_t len)
{
    if (!s_in_progress) return ESP_ERR_INVALID_STATE;
    esp_err_t err = esp_ota_write(s_handle, data, len);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_ota_write failed: %s", esp_err_to_name(err));
        ota_abort();
    }
    return err;
}

esp_err_t ota_commit(void)
{
    if (!s_in_progress) return ESP_ERR_INVALID_STATE;

    esp_err_t err = esp_ota_end(s_handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_ota_end failed: %s", esp_err_to_name(err));
        ota_abort();
        return err;
    }

    err = esp_ota_set_boot_partition(s_update_part);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_ota_set_boot_partition failed: %s", esp_err_to_name(err));
        return err;
    }

    ESP_LOGI(TAG, "OTA committed, rebooting...");
    reset_state();
    esp_restart();
    return ESP_OK; /* not reached */
}

void ota_abort(void)
{
    if (s_in_progress && s_handle) {
        esp_ota_abort(s_handle);
    }
    reset_state();
}
