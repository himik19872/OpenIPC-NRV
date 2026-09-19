#include "schedule.h"
#include "esp_log.h"

static const char *TAG = "schedule";

static bool any_weekly_enabled(const sys_config_t *cfg)
{
    for (int i = 0; i < 7; i++) {
        if (cfg->weekly_enabled[i]) return true;
    }
    return false;
}

bool schedule_allowed(const sys_config_t *cfg, const time_t *now)
{
    if (!cfg) return true;

    time_t t = now ? *now : time(NULL);
    struct tm tm;
    localtime_r(&t, &tm);

    /* Floating (date-interval) schedule takes precedence if enabled */
    if (cfg->floating_enabled) {
        if (cfg->floating_start && cfg->floating_end) {
            if ((uint32_t)t < cfg->floating_start || (uint32_t)t > cfg->floating_end) {
                ESP_LOGD(TAG, "floating schedule blocks (now=%ld outside [%lu..%lu])",
                         (long)t, (unsigned long)cfg->floating_start,
                         (unsigned long)cfg->floating_end);
                return false;
            }
        }
    }

    /* Weekly schedule */
    if (any_weekly_enabled(cfg)) {
        int wday = tm.tm_wday;   /* 0=Sunday .. 6=Saturday */
        int now_min = tm.tm_hour * 60 + tm.tm_min;

        if (cfg->weekly_enabled[wday]) {
            int on = cfg->weekly_on[wday];
            int off = cfg->weekly_off[wday];
            /* handle wrap-around interval (on > off means crosses midnight) */
            bool in_window;
            if (on <= off) {
                in_window = (now_min >= on && now_min < off);
            } else {
                in_window = (now_min >= on || now_min < off);
            }
            if (!in_window) {
                ESP_LOGD(TAG, "weekly schedule blocks (wday=%d min=%d outside [%d..%d])",
                         wday, now_min, on, off);
                return false;
            }
        } else {
            /* this weekday is not in the schedule -> denied */
            ESP_LOGD(TAG, "weekly schedule blocks (wday=%d disabled)", wday);
            return false;
        }
    }

    return true;
}
