#include <string.h>
#include <stdio.h>
#include "event_push.h"
#include "esp_log.h"
#include "esp_http_client.h"
#include "cJSON.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/queue.h"

static const char *TAG = "event_push";

#define PUSH_QUEUE_LEN    64

typedef struct {
    char    event_type[24];
    uint8_t facility;
    uint16_t card;
    char    name[48];
    uint32_t ts;
    int     flags;
} push_item_t;

static QueueHandle_t s_queue = NULL;
static TaskHandle_t s_task = NULL;
static bool s_enabled = false;

static char s_host[64];
static uint16_t s_port = 8080;
static char s_path[64];
static char s_device_id[32];

/* Map our event_type_t numeric values to NVR string event types. */
static const char *event_type_str(int type)
{
    switch (type) {
    case 0: return "access_granted";
    case 1: return "access_denied";
    case 2: return "rte_pressed";
    case 3: return "door_open";
    case 4: return "door_closed";
    case 5: return "door_forced";
    case 6: return "system_start";
    case 7: return "auth_fail";
    case 8: return "config_changed";
    default: return "unknown";
    }
}

esp_err_t event_push_configure(const char *host, uint16_t port, const char *path,
                               const char *device_id, bool enabled)
{
    s_enabled = enabled;
    if (host) strncpy(s_host, host, sizeof(s_host) - 1);
    s_port = port ? port : 80;
    if (path && path[0]) strncpy(s_path, path, sizeof(s_path) - 1);
    else strncpy(s_path, "/api/v1/acs/ingest", sizeof(s_path) - 1);
    if (device_id) strncpy(s_device_id, device_id, sizeof(s_device_id) - 1);

    ESP_LOGI(TAG, "push %s -> http://%s:%u%s (device=%s)",
             s_enabled ? "ENABLED" : "DISABLED",
             s_host, s_port, s_path, s_device_id);
    return ESP_OK;
}

static void push_task(void *arg)
{
    push_item_t item;
    while (1) {
        if (xQueueReceive(s_queue, &item, portMAX_DELAY) != pdTRUE) continue;

        if (!s_enabled) continue;

        char url[256];
        snprintf(url, sizeof(url), "http://%s:%u%s", s_host, s_port, s_path);

        cJSON *root = cJSON_CreateObject();
        cJSON_AddStringToObject(root, "device_id", s_device_id);
        cJSON_AddStringToObject(root, "event_type", item.event_type);
        cJSON_AddNumberToObject(root, "facility", item.facility);
        cJSON_AddNumberToObject(root, "card_number", item.card);
        cJSON_AddStringToObject(root, "name", item.name);
        cJSON_AddNumberToObject(root, "timestamp", item.ts);
        cJSON_AddNumberToObject(root, "flags", item.flags);
        char *body = cJSON_PrintUnformatted(root);
        cJSON_Delete(root);

        esp_http_client_config_t cfg = {
            .url = url,
            .method = HTTP_METHOD_POST,
            .timeout_ms = 5000,
        };
        esp_http_client_handle_t client = esp_http_client_init(&cfg);
        esp_http_client_set_header(client, "Content-Type", "application/json");
        esp_http_client_set_post_field(client, body, strlen(body));

        esp_err_t err = esp_http_client_perform(client);
        int status = esp_http_client_get_status_code(client);
        esp_http_client_cleanup(client);
        free(body);

        if (err != ESP_OK || status >= 400) {
            ESP_LOGW(TAG, "push failed: %s (status=%d)", esp_err_to_name(err), status);
        }
    }
}

esp_err_t event_push_send(int type, uint8_t facility,
                          uint16_t card, const char *name, uint32_t ts,
                          int flags)
{
    if (!s_queue) return ESP_ERR_INVALID_STATE;
    if (!s_enabled) return ESP_OK; /* silently ignore when disabled */

    push_item_t item;
    memset(&item, 0, sizeof(item));
    strncpy(item.event_type, event_type_str(type), sizeof(item.event_type) - 1);
    item.facility = facility;
    item.card = card;
    if (name) strncpy(item.name, name, sizeof(item.name) - 1);
    item.ts = ts;
    item.flags = flags;

    if (xQueueSend(s_queue, &item, 0) != pdTRUE) {
        /* queue full: drop oldest, retry once */
        push_item_t drop;
        if (xQueueReceive(s_queue, &drop, 0) == pdTRUE) {
            xQueueSend(s_queue, &item, 0);
        }
    }
    return ESP_OK;
}

/* Called once at boot (not wrapped in init guard — idempotent). */
static bool s_started = false;

esp_err_t event_push_init(void)
{
    if (s_started) return ESP_OK;
    s_queue = xQueueCreate(PUSH_QUEUE_LEN, sizeof(push_item_t));
    if (!s_queue) return ESP_ERR_NO_MEM;
    xTaskCreate(push_task, "event_push", 4096, NULL, 5, &s_task);
    s_started = true;
    ESP_LOGI(TAG, "event push task started");
    return ESP_OK;
}
