#include <string.h>
#include <stdio.h>
#include "card_db.h"
#include "storage.h"
#include "esp_log.h"

static const char *TAG = "card_db";

#define DB_FILE         "/storage/cards.bin"

/* File layout:
 *   [0..3]   magic "CRDS"
 *   [4..7]   uint32_t count
 *   [8..]    card_record_t[CARD_DB_MAX]  (fixed-size records)
 */

#define DB_MAGIC        0x53445243  /* "CRDS" little-endian */

static bool s_ok = false;

/* loads count from file */
static int load_count(void)
{
    FILE *f = fopen(DB_FILE, "rb");
    if (!f) return 0;

    uint32_t magic = 0, count = 0;
    if (fread(&magic, 4, 1, f) == 1 && magic == DB_MAGIC) {
        if (fread(&count, 4, 1, f) != 1) count = 0;
    }
    fclose(f);

    if (count > CARD_DB_MAX) count = CARD_DB_MAX;
    return (int)count;
}

static void save_count(int count)
{
    FILE *f = fopen(DB_FILE, "r+b");
    if (!f) return;
    fseek(f, 4, SEEK_SET);
    uint32_t c = (uint32_t)count;
    fwrite(&c, 4, 1, f);
    fclose(f);
}

esp_err_t card_db_init(void)
{
    esp_err_t err = storage_init();
    if (err != ESP_OK) {
        return err;
    }

    /* ensure file exists with header */
    FILE *f = fopen(DB_FILE, "rb");
    if (!f) {
        f = fopen(DB_FILE, "wb");
        if (!f) {
            ESP_LOGE(TAG, "cannot create db file");
            return ESP_FAIL;
        }
        uint32_t magic = DB_MAGIC;
        uint32_t zero = 0;
        fwrite(&magic, 4, 1, f);
        fwrite(&zero, 4, 1, f);
        fclose(f);
    } else {
        fclose(f);
    }

    s_ok = true;
    ESP_LOGI(TAG, "card db ready, count=%d", card_db_count());
    return ESP_OK;
}

int card_db_count(void)
{
    if (!s_ok) {
        if (storage_init() != ESP_OK) return 0;
        s_ok = true;
    }
    return load_count();
}

/* read record at slot idx (0-based) */
static esp_err_t read_slot(int idx, card_record_t *out)
{
    FILE *f = fopen(DB_FILE, "rb");
    if (!f) return ESP_ERR_NOT_FOUND;

    long off = 8 + (long)idx * sizeof(card_record_t);
    esp_err_t err = ESP_OK;
    if (fseek(f, off, SEEK_SET) != 0) {
        err = ESP_FAIL;
    } else if (fread(out, sizeof(card_record_t), 1, f) != 1) {
        err = ESP_FAIL;
    }
    fclose(f);
    return err;
}

static esp_err_t write_slot(int idx, const card_record_t *rec)
{
    FILE *f = fopen(DB_FILE, "r+b");
    if (!f) return ESP_ERR_NOT_FOUND;

    long off = 8 + (long)idx * sizeof(card_record_t);
    esp_err_t err = ESP_OK;
    if (fseek(f, off, SEEK_SET) != 0) {
        err = ESP_FAIL;
    } else if (fwrite(rec, sizeof(card_record_t), 1, f) != 1) {
        err = ESP_FAIL;
    }
    fclose(f);
    return err;
}

int card_db_find(uint8_t facility, uint16_t card)
{
    int count = card_db_count();
    for (int i = 0; i < count; i++) {
        card_record_t r;
        if (read_slot(i, &r) == ESP_OK) {
            if (r.facility == facility && r.card == card) return i;
        }
    }
    return -1;
}

esp_err_t card_db_get(int idx, card_record_t *out)
{
    if (idx < 0 || idx >= card_db_count()) return ESP_ERR_INVALID_ARG;
    return read_slot(idx, out);
}

esp_err_t card_db_add(const card_record_t *rec)
{
    int count = card_db_count();
    if (count >= CARD_DB_MAX) return ESP_ERR_NO_MEM;
    if (card_db_find(rec->facility, rec->card) >= 0) return ESP_ERR_INVALID_STATE;

    esp_err_t err = write_slot(count, rec);
    if (err == ESP_OK) {
        save_count(count + 1);
    }
    return err;
}

esp_err_t card_db_update(const card_record_t *rec)
{
    int idx = card_db_find(rec->facility, rec->card);
    if (idx < 0) return ESP_ERR_NOT_FOUND;
    return write_slot(idx, rec);
}

esp_err_t card_db_remove(uint8_t facility, uint16_t card)
{
    int idx = card_db_find(facility, card);
    if (idx < 0) return ESP_ERR_NOT_FOUND;

    int count = card_db_count();
    /* shift subsequent slots left */
    for (int i = idx; i < count - 1; i++) {
        card_record_t r;
        if (read_slot(i + 1, &r) == ESP_OK) {
            write_slot(i, &r);
        }
    }
    /* erase last */
    card_record_t zero;
    memset(&zero, 0, sizeof(zero));
    write_slot(count - 1, &zero);
    save_count(count - 1);
    return ESP_OK;
}

esp_err_t card_db_clear(void)
{
    int count = card_db_count();
    card_record_t zero;
    memset(&zero, 0, sizeof(zero));
    for (int i = 0; i < count; i++) {
        write_slot(i, &zero);
    }
    save_count(0);
    return ESP_OK;
}

/* ---- Learn mode ---- */
static bool s_learn_active = false;
static char s_learn_name[CARD_NAME_MAX];

void card_db_learn_arm(const char *name)
{
    memset(s_learn_name, 0, sizeof(s_learn_name));
    if (name) strncpy(s_learn_name, name, CARD_NAME_MAX - 1);
    s_learn_active = true;
    ESP_LOGI(TAG, "Learn mode armed (name='%s')", s_learn_name);
}

void card_db_learn_cancel(void)
{
    s_learn_active = false;
    ESP_LOGI(TAG, "Learn mode cancelled");
}

bool card_db_learn_active(void)
{
    return s_learn_active;
}

bool card_db_learn_capture(uint8_t facility, uint16_t card)
{
    if (!s_learn_active) return false;

    card_record_t rec;
    memset(&rec, 0, sizeof(rec));
    rec.facility = facility;
    rec.card = card;
    strncpy(rec.name, s_learn_name, CARD_NAME_MAX - 1);
    rec.access = CARD_ACCESS_PERMANENT;
    rec.active = true;

    int existing = card_db_find(facility, card);
    if (existing >= 0) {
        card_db_update(&rec);
        ESP_LOGI(TAG, "Learn: updated existing card f=%u c=%u", facility, card);
    } else {
        card_db_add(&rec);
        ESP_LOGI(TAG, "Learn: added card f=%u c=%u", facility, card);
    }

    s_learn_active = false;
    return true;
}
