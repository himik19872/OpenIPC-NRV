#include <string.h>
#include "storage.h"
#include "esp_log.h"
#include "esp_partition.h"
#include "esp_vfs.h"
#include "esp_littlefs.h"

static const char *TAG = "storage";

#define STORAGE_PARTITION  "storage"
#define STORAGE_MOUNT      "/storage"

static bool s_ready = false;

esp_err_t storage_init(void)
{
    if (s_ready) {
        return ESP_OK;
    }

    const esp_partition_t *part = esp_partition_find_first(
        ESP_PARTITION_TYPE_DATA, ESP_PARTITION_SUBTYPE_DATA_SPIFFS,
        STORAGE_PARTITION);
    if (part == NULL) {
        ESP_LOGE(TAG, "storage partition '%s' not found", STORAGE_PARTITION);
        return ESP_ERR_NOT_FOUND;
    }

    esp_vfs_littlefs_conf_t conf = {
        .base_path = STORAGE_MOUNT,
        .partition_label = STORAGE_PARTITION,
        .format_if_mount_failed = true,
        .dont_mount = false,
    };

    esp_err_t err = esp_vfs_littlefs_register(&conf);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "littlefs mount failed: %s", esp_err_to_name(err));
        return err;
    }

    size_t total = 0, used = 0;
    if (esp_littlefs_info(STORAGE_PARTITION, &total, &used) == ESP_OK) {
        ESP_LOGI(TAG, "littlefs mounted: total=%u used=%u free=%u",
                 (unsigned)total, (unsigned)used,
                 (unsigned)(total - used));
    } else {
        ESP_LOGI(TAG, "littlefs mounted at %s", STORAGE_MOUNT);
    }

    s_ready = true;
    return ESP_OK;
}

bool storage_ready(void)
{
    return s_ready;
}
