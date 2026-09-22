#include <string.h>
#include "ota.h"
#include "esp_log.h"
#include "esp_ota_ops.h"
#include "esp_partition.h"
#include "esp_app_format.h"
#include "esp_system.h"
#include "esp_task_wdt.h"
#include "nvs.h"
#include "nvs_flash.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "ota";

/* Журнал последнего OTA хранится в NVS.
 *
 * Канал syslog не работает, а UART на устройстве недоступен, поэтому
 * единственный способ узнать причину сбоя — записать её в энергонезависимую
 * память и прочитать после перезагрузки. При аварийном падении лог в
 * консоли теряется, а NVS сохраняется. */
#define OTA_LOG_NVS_NS   "ota_log"

static void ota_journal(const char *stage, const char *detail)
{
    nvs_handle_t h;
    if (nvs_open(OTA_LOG_NVS_NS, NVS_READWRITE, &h) != ESP_OK) {
        return;
    }
    nvs_set_str(h, "stage", stage);
    if (detail) {
        nvs_set_str(h, "detail", detail);
    }
    nvs_commit(h);
    nvs_close(h);
}

/* Читает и очищает журнал последнего OTA. Вызывается при старте. */
void ota_read_journal(void)
{
    nvs_handle_t h;
    if (nvs_open(OTA_LOG_NVS_NS, NVS_READONLY, &h) != ESP_OK) {
        return;
    }
    char stage[64] = {0};
    char detail[128] = {0};
    size_t len = sizeof(stage);
    nvs_get_str(h, "stage", stage, &len);
    len = sizeof(detail);
    nvs_get_str(h, "detail", detail, &len);
    nvs_close(h);

    if (stage[0]) {
        ESP_LOGW(TAG, "прошлый OTA остановился на шаге: %s (%s)", stage, detail);
    }

    /* Стираем, чтобы запись не путала следующий запуск. */
    nvs_handle_t hw;
    if (nvs_open(OTA_LOG_NVS_NS, NVS_READWRITE, &hw) == ESP_OK) {
        nvs_erase_all(hw);
        nvs_commit(hw);
        nvs_close(hw);
    }
}

static esp_ota_handle_t s_handle = 0;
static const esp_partition_t *s_update_part = NULL;
static bool s_in_progress = false;
/* Сколько байт уже принято — для отметок прогресса в журнале. */
static size_t s_written = 0;

/* Стирание OTA-раздела и запись образа — длительные операции, которые
 * надолго блокируют текущую задачу: раздел занимает 3 МБ, и его очистка
 * идёт заметно дольше таймаута штатного watchdog (5 секунд). Без сброса
 * сторожевой таймер считает устройство зависшим и перезагружает его —
 * именно поэтому OTA прерывалась сразу после начала, независимо от
 * размера образа. */
static void ota_feed_wdt(void)
{
    esp_task_wdt_reset();
}

static void reset_state(void)
{
    s_handle = 0;
    s_update_part = NULL;
    s_in_progress = false;
    s_written = 0;
}

esp_err_t ota_begin(size_t image_size)
{
    /* Ищем раздел для записи и логируем это до любых операций с flash:
     * если следующий шаг приведёт к сбою, в логе останется хотя бы
     * информация о том, куда шла запись. */
    const esp_partition_t *update = esp_ota_get_next_update_partition(NULL);
    if (!update) {
        ESP_LOGE(TAG, "No OTA update partition found");
        return ESP_ERR_NOT_FOUND;
    }

    if (image_size == 0) {
        ESP_LOGE(TAG, "OTA: не указан размер образа");
        return ESP_ERR_INVALID_ARG;
    }
    if (image_size > update->size) {
        ESP_LOGE(TAG, "OTA: образ %u байт больше раздела %lu",
                 (unsigned)image_size, (unsigned long)update->size);
        return ESP_ERR_INVALID_SIZE;
    }

    const esp_partition_t *running = esp_ota_get_running_partition();
    ESP_LOGW(TAG, "OTA begin: target=%s at 0x%lx size %lu, running=%s, image %u, heap %u",
             update->label, (unsigned long)update->address,
             (unsigned long)update->size,
             running ? running->label : "?",
             (unsigned)image_size, (unsigned)esp_get_free_heap_size());

    char jd[128];
    snprintf(jd, sizeof(jd), "target=%s size=%u heap=%u",
             update->label, (unsigned)image_size,
             (unsigned)esp_get_free_heap_size());
    ota_journal("begin", jd);

    /* Стирание раздела занимает больше таймаута штатного watchdog,
     * поэтому сбрасываем таймер и берём задачу под наблюдение. */
    esp_task_wdt_add(NULL);
    ota_feed_wdt();

    /* Размер образа передаём явно.
     *
     * При нулевом размере слой OTA резервирует буферы под весь раздел —
     * а он 3 МБ, что больше доступной памяти. Устройство падало молча,
     * без записей в лог, и перезагружалось сразу после начала загрузки. */
    esp_err_t err = esp_ota_begin(update, image_size, &s_handle);
    ota_feed_wdt();
    if (err != ESP_OK) {
        esp_task_wdt_delete(NULL);
        ESP_LOGE(TAG, "esp_ota_begin failed: %s", esp_err_to_name(err));
        ota_journal("begin_failed", esp_err_to_name(err));
        return err;
    }

    ESP_LOGW(TAG, "OTA begin: ok, handle=%p, heap %u",
             (void *)s_handle, (unsigned)esp_get_free_heap_size());
    ota_journal("begin_ok", NULL);
    s_update_part = update;
    s_in_progress = true;
    return ESP_OK;
}

esp_err_t ota_write(const uint8_t *data, size_t len)
{
    if (!s_in_progress) return ESP_ERR_INVALID_STATE;

    /* Запись во flash блокирует задачу: сбрасываем watchdog перед каждой
     * порцией, иначе таймер сработает и прервёт обновление. */
    ota_feed_wdt();
    esp_err_t err = esp_ota_write(s_handle, data, len);
    ota_feed_wdt();

    /* Отмечаем прогресс в журнале: если устройство упадёт на записи,
     * после перезагрузки будет видно, сколько успело записаться. */
    if (s_written == 0 || (s_written / (256 * 1024)) != ((s_written + len) / (256 * 1024))) {
        char jd[64];
        snprintf(jd, sizeof(jd), "%u", (unsigned)(s_written + len));
        ota_journal("writing", jd);
    }
    s_written += len;

    if (err != ESP_OK) {
        ESP_LOGE(TAG, "esp_ota_write failed: %s", esp_err_to_name(err));
        ota_journal("write_failed", esp_err_to_name(err));
        ota_abort();
    }
    return err;
}

esp_err_t ota_commit(void)
{
    if (!s_in_progress) return ESP_ERR_INVALID_STATE;

    ota_feed_wdt();
    esp_err_t err = esp_ota_end(s_handle);
    ota_feed_wdt();
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

    /* Обновление завершено — снимаем задачу со сторожевого таймера перед
     * перезагрузкой. */
    esp_task_wdt_delete(NULL);

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
    esp_task_wdt_delete(NULL);
    reset_state();
}

void confirm_firmware(void)
{
    /* Загрузчик держит новую прошивку в состоянии «на проверке» и вернётся
     * к предыдущей, если та не подтвердится. Подтверждаем только сейчас —
     * когда сеть и HTTP-сервер уже работают: тогда устройство реально
     * управляемо, и откат не нужен. */
    const esp_partition_t *running = esp_ota_get_running_partition();
    if (!running) {
        ESP_LOGW(TAG, "running partition unknown, cannot confirm firmware");
        return;
    }

    esp_ota_img_states_t state;
    if (esp_ota_get_state_partition(running, &state) != ESP_OK) {
        /* Состояние недоступно, если откат выключен в конфигурации —
         * подтверждать нечего. */
        ESP_LOGD(TAG, "OTA state unavailable, rollback disabled");
        return;
    }

    if (state == ESP_OTA_IMG_PENDING_VERIFY) {
        esp_err_t err = esp_ota_mark_app_valid_cancel_rollback();
        if (err == ESP_OK) {
            ESP_LOGI(TAG, "firmware confirmed: rollback cancelled");
        } else {
            ESP_LOGE(TAG, "failed to confirm firmware: %s", esp_err_to_name(err));
        }
    } else {
        ESP_LOGI(TAG, "firmware already confirmed, state=%d", (int)state);
    }
}

/* Возвращает запись журнала последнего OTA без очистки.
 * Используется для диагностики: результат виден через HTTP, что важно,
 * когда консоль и syslog недоступны. */
void ota_peek_journal(char *stage, size_t stage_len,
                      char *detail, size_t detail_len)
{
    if (stage && stage_len) stage[0] = 0;
    if (detail && detail_len) detail[0] = 0;

    nvs_handle_t h;
    if (nvs_open(OTA_LOG_NVS_NS, NVS_READONLY, &h) != ESP_OK) {
        return;
    }
    if (stage && stage_len) {
        nvs_get_str(h, "stage", stage, &stage_len);
    }
    if (detail && detail_len) {
        nvs_get_str(h, "detail", detail, &detail_len);
    }
    nvs_close(h);
}
