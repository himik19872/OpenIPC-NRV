#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include "time_sync.h"
#include "esp_log.h"
#include "esp_sntp.h"
#include "esp_netif.h"

static const char *TAG = "time_sync";

static bool s_started = false;
static bool s_synced = false;

static void sntp_sync_cb(struct timeval *tv)
{
    s_synced = true;
    ESP_LOGI(TAG, "Time synchronized: %ld s", (long)tv->tv_sec);
}

esp_err_t time_sync_start(const char *server, uint32_t interval_sec)
{
    if (s_started) {
        esp_sntp_stop();
    }
    s_synced = false;

    esp_sntp_setoperatingmode(SNTP_OPMODE_POLL);
    if (server && server[0]) {
        esp_sntp_setservername(0, (char *)server);
    } else {
        esp_sntp_setservername(0, "pool.ntp.org");
    }

    esp_sntp_set_sync_mode(SNTP_SYNC_MODE_IMMED);
    esp_sntp_set_sync_interval(interval_sec > 0 ? interval_sec * 1000 : 3600000);
    esp_sntp_set_time_sync_notification_cb(sntp_sync_cb);

    esp_sntp_init();
    s_started = true;

    ESP_LOGI(TAG, "SNTP started: server=%s interval=%lus", server, (unsigned long)interval_sec);
    return ESP_OK;
}

esp_err_t time_sync_now(void)
{
    if (!s_started) return ESP_ERR_INVALID_STATE;
    /* restart SNTP to trigger an immediate re-sync */
    esp_sntp_restart();
    ESP_LOGI(TAG, "Manual sync requested");
    return ESP_OK;
}

void time_sync_set_timezone(int16_t offset_min)
{
    char tz[32];
    int hours = offset_min / 60;
    int mins = offset_min % 60;
    if (mins < 0) { mins = -mins; }
    /* POSIX TZ format: "<std><offset>" where offset is negative of local
     * for east-of-UTC zones: e.g. MSK (UTC+3) = "UTC-3" */
    int posix_off = -offset_min;
    int ph = posix_off / 60;
    int pm = posix_off % 60;
    if (pm < 0) { pm = -pm; }
    snprintf(tz, sizeof(tz), "UTC%+d:%02d", ph, pm);
    if (pm == 0) snprintf(tz, sizeof(tz), "UTC%+d", ph);

    setenv("TZ", tz, 1);
    tzset();
    ESP_LOGI(TAG, "Timezone set: %s (offset %+d min UTC)", tz, offset_min);
}

bool time_sync_synced(void)
{
    /* trust the SNTP notification callback flag; also accept a plausibly
     * non-zero RTC (e.g. time was set previously) */
    if (s_synced) return true;
    time_t now = time(NULL);
    return now > 1000000000; /* after 2001-09-09 == RTC likely synced */
}
