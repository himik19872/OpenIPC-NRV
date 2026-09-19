#include <string.h>
#include <stdio.h>
#include "event_log.h"
#include "storage.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "sys/time.h"

static const char *TAG = "event_log";

/* File layout (/storage/events.bin):
 *   [0..3]   magic "EVLG"
 *   [4..7]   uint32_t head (next write index)
 *   [8..11]  uint32_t count
 *   [12..]   event_record_t[EVENT_LOG_MAX]
 */

#define LOG_FILE        "/storage/events.bin"
#define LOG_MAGIC       0x474C5645  /* "EVLG" LE */

static struct {
    bool        ok;
    uint32_t    head;    /* next slot to write */
    uint32_t    count;   /* number of valid entries */
} s_log;

static esp_err_t load_meta(void)
{
    FILE *f = fopen(LOG_FILE, "rb");
    if (!f) return ESP_ERR_NOT_FOUND;

    uint32_t magic = 0;
    if (fread(&magic, 4, 1, f) != 1 || magic != LOG_MAGIC) {
        fclose(f);
        return ESP_ERR_NOT_FOUND;
    }
    if (fread(&s_log.head, 4, 1, f) != 1) s_log.head = 0;
    if (fread(&s_log.count, 4, 1, f) != 1) s_log.count = 0;
    fclose(f);

    if (s_log.head > EVENT_LOG_MAX) s_log.head = 0;
    if (s_log.count > EVENT_LOG_MAX) s_log.count = EVENT_LOG_MAX;
    return ESP_OK;
}

static void save_meta(void)
{
    FILE *f = fopen(LOG_FILE, "r+b");
    if (!f) return;
    fseek(f, 4, SEEK_SET);
    fwrite(&s_log.head, 4, 1, f);
    fwrite(&s_log.count, 4, 1, f);
    fclose(f);
}

esp_err_t event_log_init(void)
{
    esp_err_t err = storage_init();
    if (err != ESP_OK) {
        return err;
    }

    s_log.head = 0;
    s_log.count = 0;

    FILE *f = fopen(LOG_FILE, "rb");
    if (!f) {
        f = fopen(LOG_FILE, "wb");
        if (!f) {
            ESP_LOGE(TAG, "cannot create log file");
            return ESP_FAIL;
        }
        uint32_t magic = LOG_MAGIC;
        uint32_t zero = 0;
        fwrite(&magic, 4, 1, f);
        fwrite(&zero, 4, 1, f);
        fwrite(&zero, 4, 1, f);
        fclose(f);
    } else {
        fclose(f);
        load_meta();
    }

    s_log.ok = true;
    ESP_LOGI(TAG, "Event log init: count=%lu", (unsigned long)s_log.count);
    return ESP_OK;
}

static uint32_t now_ts(void)
{
    struct timeval tv;
    gettimeofday(&tv, NULL);
    if (tv.tv_sec < 1000000000) {
        /* RTC not synced; use monotonic time as fallback */
        return (uint32_t)(esp_timer_get_time() / 1000000ULL);
    }
    return (uint32_t)tv.tv_sec;
}

/* read record at physical slot */
static esp_err_t read_slot(uint32_t slot, event_record_t *out)
{
    FILE *f = fopen(LOG_FILE, "rb");
    if (!f) return ESP_ERR_NOT_FOUND;

    long off = 12 + (long)slot * sizeof(event_record_t);
    esp_err_t err = ESP_OK;
    if (fseek(f, off, SEEK_SET) != 0) {
        err = ESP_FAIL;
    } else if (fread(out, sizeof(event_record_t), 1, f) != 1) {
        err = ESP_FAIL;
    }
    fclose(f);
    return err;
}

static esp_err_t write_slot(uint32_t slot, const event_record_t *rec)
{
    FILE *f = fopen(LOG_FILE, "r+b");
    if (!f) return ESP_ERR_NOT_FOUND;

    long off = 12 + (long)slot * sizeof(event_record_t);
    esp_err_t err = ESP_OK;
    if (fseek(f, off, SEEK_SET) != 0) {
        err = ESP_FAIL;
    } else if (fwrite(rec, sizeof(event_record_t), 1, f) != 1) {
        err = ESP_FAIL;
    }
    fclose(f);
    return err;
}

esp_err_t event_log_add(event_type_t type, uint8_t facility, uint16_t card,
                        const char *note)
{
    return event_log_add_pass(type, facility, card, note, false);
}

esp_err_t event_log_add_pass(event_type_t type, uint8_t facility, uint16_t card,
                             const char *note, bool pass_completed)
{
    event_record_t rec;
    memset(&rec, 0, sizeof(rec));
    rec.ts = now_ts();
    rec.type = (uint8_t)type;
    rec.facility = facility;
    rec.card = card;
    rec.flags = pass_completed ? 1 : 0;
    if (note) {
        strncpy(rec.note, note, sizeof(rec.note) - 1);
        rec.note[sizeof(rec.note) - 1] = '\0';
    }

    write_slot(s_log.head, &rec);

    s_log.head = (s_log.head + 1) % EVENT_LOG_MAX;
    if (s_log.count < EVENT_LOG_MAX) s_log.count++;

    save_meta();

    return ESP_OK;
}

int event_log_count(void)
{
    return (int)s_log.count;
}

esp_err_t event_log_get(int idx, event_record_t *out)
{
    if (idx < 0 || idx >= (int)s_log.count) return ESP_ERR_INVALID_ARG;

    uint32_t count = s_log.count;
    uint32_t head = s_log.head;
    uint32_t oldest = (head + EVENT_LOG_MAX - count) % EVENT_LOG_MAX;
    uint32_t slot = (oldest + (uint32_t)idx) % EVENT_LOG_MAX;

    return read_slot(slot, out);
}

esp_err_t event_log_clear(void)
{
    /* zero out all records */
    event_record_t zero;
    memset(&zero, 0, sizeof(zero));
    for (uint32_t i = 0; i < EVENT_LOG_MAX; i++) {
        write_slot(i, &zero);
    }
    s_log.head = 0;
    s_log.count = 0;
    save_meta();
    return ESP_OK;
}

esp_err_t event_log_mark_last_pass(void)
{
    if (s_log.count == 0) return ESP_ERR_NOT_FOUND;

    /* scan backwards for most recent CARD_GRANTED (type 0) */
    for (int i = (int)s_log.count - 1; i >= 0; i--) {
        event_record_t rec;
        if (event_log_get(i, &rec) != ESP_OK) continue;
        if (rec.type == 0) { /* EVT_CARD_GRANTED */
            rec.flags |= 1;
            uint32_t count = s_log.count;
            uint32_t head = s_log.head;
            uint32_t oldest = (head + EVENT_LOG_MAX - count) % EVENT_LOG_MAX;
            uint32_t slot = (oldest + (uint32_t)i) % EVENT_LOG_MAX;
            write_slot(slot, &rec);
            return ESP_OK;
        }
    }
    return ESP_ERR_NOT_FOUND;
}
