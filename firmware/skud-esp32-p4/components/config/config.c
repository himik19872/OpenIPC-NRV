#include <string.h>
#include "config.h"
#include "nvs.h"
#include "nvs_flash.h"
#include "esp_log.h"

static const char *TAG = "config";

#define NVS_NAMESPACE       "skud"
#define KEY_LOCK_GPIO       "lock_gpio"
#define KEY_RTE_GPIO        "rte_gpio"
#define KEY_SENSOR_GPIO     "sensor_gpio"
#define KEY_WG_D0           "wg_d0"
#define KEY_WG_D1           "wg_d1"
#define KEY_WG2_D0          "wg2_d0"
#define KEY_WG2_D1          "wg2_d1"
#define KEY_BUZZER_GPIO     "buzzer_gpio"
#define KEY_LOCK_PULSE      "lock_pulse"

/* system config keys */
#define KEY_LOGIN           "login"
#define KEY_PASS            "pass"
#define KEY_SYSLOG_EN       "syslog_en"
#define KEY_SYSLOG_HOST     "syslog_host"
#define KEY_SYSLOG_PORT     "syslog_port"
#define KEY_NET_MODE        "net_mode"
#define KEY_IP              "ip"
#define KEY_MASK            "mask"
#define KEY_GW              "gw"
#define KEY_OP_MODE         "op_mode"
#define KEY_SRV_HOST        "srv_host"
#define KEY_SRV_PORT        "srv_port"
#define KEY_SRV_PATH        "srv_path"
#define KEY_DOOR_TYPE       "door_type"
#define KEY_APB             "antipassback"

/* schedule keys */
#define KEY_WEEK_EN         "week_en"       /* 7 bytes blob */
#define KEY_WEEK_ON         "week_on"       /* 7 x uint16 blob = 14 bytes */
#define KEY_WEEK_OFF        "week_off"      /* 14 bytes */
#define KEY_FLT_EN          "flt_en"
#define KEY_FLT_START       "flt_start"
#define KEY_FLT_END         "flt_end"

/* time + identity keys */
#define KEY_NTP_SERVER      "ntp_server"
#define KEY_NTP_INTERVAL    "ntp_interval"
#define KEY_TZ_MIN          "tz_min"
#define KEY_DEV_NAME        "dev_name"
#define KEY_DEV_LOC         "dev_loc"

void config_defaults(pin_config_t *cfg)
{
    memset(cfg, 0, sizeof(*cfg));
    cfg->lock_gpio    = 8;
    cfg->rte_gpio     = PIN_DISABLED;
    cfg->sensor_gpio  = PIN_DISABLED;
    cfg->wiegand_d0   = 4;
    cfg->wiegand_d1   = 5;
    cfg->wiegand2_d0  = PIN_DISABLED;
    cfg->wiegand2_d1  = PIN_DISABLED;
    cfg->buzzer_gpio  = PIN_DISABLED;
    cfg->lock_pulse_ms = 3000;
}

esp_err_t config_load(pin_config_t *cfg)
{
    nvs_handle_t h;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READONLY, &h);
    if (err != ESP_OK) {
        /* fresh flash: defaults */
        config_defaults(cfg);
        return ESP_OK;
    }

    config_defaults(cfg);

    int32_t v = 0;
    if (nvs_get_i32(h, KEY_LOCK_GPIO, &v) == ESP_OK)   cfg->lock_gpio = (int16_t)v;
    if (nvs_get_i32(h, KEY_RTE_GPIO, &v) == ESP_OK)    cfg->rte_gpio = (int16_t)v;
    if (nvs_get_i32(h, KEY_SENSOR_GPIO, &v) == ESP_OK) cfg->sensor_gpio = (int16_t)v;
    if (nvs_get_i32(h, KEY_WG_D0, &v) == ESP_OK)       cfg->wiegand_d0 = (int16_t)v;
    if (nvs_get_i32(h, KEY_WG_D1, &v) == ESP_OK)       cfg->wiegand_d1 = (int16_t)v;
    if (nvs_get_i32(h, KEY_WG2_D0, &v) == ESP_OK)      cfg->wiegand2_d0 = (int16_t)v;
    if (nvs_get_i32(h, KEY_WG2_D1, &v) == ESP_OK)      cfg->wiegand2_d1 = (int16_t)v;
    if (nvs_get_i32(h, KEY_BUZZER_GPIO, &v) == ESP_OK) cfg->buzzer_gpio = (int16_t)v;
    if (nvs_get_u32(h, KEY_LOCK_PULSE, (uint32_t *)&cfg->lock_pulse_ms) != ESP_OK) {}

    nvs_close(h);
    ESP_LOGI(TAG, "Loaded: lock=%d rte=%d sensor=%d wg=%d/%d wg2=%d/%d buzzer=%d pulse=%lu",
             cfg->lock_gpio, cfg->rte_gpio, cfg->sensor_gpio,
             cfg->wiegand_d0, cfg->wiegand_d1,
             cfg->wiegand2_d0, cfg->wiegand2_d1,
             cfg->buzzer_gpio, (unsigned long)cfg->lock_pulse_ms);
    return ESP_OK;
}

esp_err_t config_save(const pin_config_t *cfg)
{
    nvs_handle_t h;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &h);
    if (err != ESP_OK) return err;

    nvs_set_i32(h, KEY_LOCK_GPIO, (int32_t)cfg->lock_gpio);
    nvs_set_i32(h, KEY_RTE_GPIO,  (int32_t)cfg->rte_gpio);
    nvs_set_i32(h, KEY_SENSOR_GPIO, (int32_t)cfg->sensor_gpio);
    nvs_set_i32(h, KEY_WG_D0,     (int32_t)cfg->wiegand_d0);
    nvs_set_i32(h, KEY_WG_D1,     (int32_t)cfg->wiegand_d1);
    nvs_set_i32(h, KEY_WG2_D0,    (int32_t)cfg->wiegand2_d0);
    nvs_set_i32(h, KEY_WG2_D1,    (int32_t)cfg->wiegand2_d1);
    nvs_set_i32(h, KEY_BUZZER_GPIO, (int32_t)cfg->buzzer_gpio);
    nvs_set_u32(h, KEY_LOCK_PULSE, cfg->lock_pulse_ms);

    err = nvs_commit(h);
    nvs_close(h);

    if (err == ESP_OK) {
        ESP_LOGI(TAG, "Saved pin config");
    }
    return err;
}

/* ================= System config (auth + syslog) ================= */

void sys_config_defaults(sys_config_t *cfg)
{
    memset(cfg, 0, sizeof(*cfg));
    strcpy(cfg->login, "admin");
    strcpy(cfg->password, "admin");
    cfg->syslog_enabled = false;
    cfg->syslog_host[0] = '\0';
    cfg->syslog_port = 514;

    cfg->net_mode = NET_DHCP;
    cfg->op_mode = MODE_AUTONOMOUS;
    cfg->server_port = 80;
    cfg->server_path[0] = '\0';

    cfg->door_type = DOOR_TYPE_ONE_READER;
    cfg->antipassback = false;

    strcpy(cfg->ntp_server, "pool.ntp.org");
    cfg->ntp_interval_sec = 3600;
    cfg->timezone_min = 180;   /* MSK = UTC+3 */
    strcpy(cfg->device_name, "SKUD-01");
    strcpy(cfg->device_location, "Главный вход");
}

esp_err_t sys_config_load(sys_config_t *cfg)
{
    nvs_handle_t h;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READONLY, &h);
    if (err != ESP_OK) {
        sys_config_defaults(cfg);
        return ESP_OK;
    }

    sys_config_defaults(cfg);

    size_t len;
    len = sizeof(cfg->login);   nvs_get_str(h, KEY_LOGIN, cfg->login, &len);
    len = sizeof(cfg->password); nvs_get_str(h, KEY_PASS, cfg->password, &len);
    len = sizeof(cfg->syslog_host); nvs_get_str(h, KEY_SYSLOG_HOST, cfg->syslog_host, &len);
    len = sizeof(cfg->server_host); nvs_get_str(h, KEY_SRV_HOST, cfg->server_host, &len);
    len = sizeof(cfg->server_path); nvs_get_str(h, KEY_SRV_PATH, cfg->server_path, &len);

    uint8_t en = 0;
    if (nvs_get_u8(h, KEY_SYSLOG_EN, &en) == ESP_OK) cfg->syslog_enabled = (en != 0);
    uint16_t port = 0;
    if (nvs_get_u16(h, KEY_SYSLOG_PORT, &port) == ESP_OK) cfg->syslog_port = port;

    uint8_t nm = 0; if (nvs_get_u8(h, KEY_NET_MODE, &nm) == ESP_OK) cfg->net_mode = nm;
    uint8_t om = 0; if (nvs_get_u8(h, KEY_OP_MODE, &om) == ESP_OK) cfg->op_mode = om;
    uint8_t dt = 0; if (nvs_get_u8(h, KEY_DOOR_TYPE, &dt) == ESP_OK) cfg->door_type = dt;
    uint8_t apb = 0; if (nvs_get_u8(h, KEY_APB, &apb) == ESP_OK) cfg->antipassback = (apb != 0);

    size_t n = 4;
    nvs_get_blob(h, KEY_IP, cfg->ip, &n); n = 4; nvs_get_blob(h, KEY_MASK, cfg->mask, &n);
    n = 4; nvs_get_blob(h, KEY_GW, cfg->gw, &n);
    uint16_t sp = 0; if (nvs_get_u16(h, KEY_SRV_PORT, &sp) == ESP_OK) cfg->server_port = sp;

    /* schedules */
    nvs_get_blob(h, KEY_WEEK_EN, cfg->weekly_enabled, &(size_t){7});
    {
        uint8_t tmp_on[14], tmp_off[14];
        size_t sz = sizeof(tmp_on);
        if (nvs_get_blob(h, KEY_WEEK_ON, tmp_on, &sz) == ESP_OK) {
            for (int i = 0; i < 7; i++) cfg->weekly_on[i] = (tmp_on[2*i] | (tmp_on[2*i+1] << 8));
        }
        sz = sizeof(tmp_off);
        if (nvs_get_blob(h, KEY_WEEK_OFF, tmp_off, &sz) == ESP_OK) {
            for (int i = 0; i < 7; i++) cfg->weekly_off[i] = (tmp_off[2*i] | (tmp_off[2*i+1] << 8));
        }
    }
    uint8_t fe = 0; if (nvs_get_u8(h, KEY_FLT_EN, &fe) == ESP_OK) cfg->floating_enabled = (fe != 0);
    uint32_t u32 = 0;
    if (nvs_get_u32(h, KEY_FLT_START, &u32) == ESP_OK) cfg->floating_start = u32;
    if (nvs_get_u32(h, KEY_FLT_END, &u32) == ESP_OK) cfg->floating_end = u32;

    /* time + identity */
    len = sizeof(cfg->ntp_server); nvs_get_str(h, KEY_NTP_SERVER, cfg->ntp_server, &len);
    len = sizeof(cfg->device_name); nvs_get_str(h, KEY_DEV_NAME, cfg->device_name, &len);
    len = sizeof(cfg->device_location); nvs_get_str(h, KEY_DEV_LOC, cfg->device_location, &len);
    if (nvs_get_u32(h, KEY_NTP_INTERVAL, &u32) == ESP_OK) cfg->ntp_interval_sec = u32;
    int32_t tzv = 0;
    if (nvs_get_i32(h, KEY_TZ_MIN, &tzv) == ESP_OK) cfg->timezone_min = (int16_t)tzv;

    nvs_close(h);
    ESP_LOGI(TAG, "Sys config: login=%s syslog=%s net=%s op=%s door=%s apb=%s",
             cfg->login,
             cfg->syslog_enabled ? "ON" : "OFF",
             cfg->net_mode == NET_DHCP ? "DHCP" : "static",
             cfg->op_mode == MODE_AUTONOMOUS ? "autonomous" : "network",
             cfg->door_type == DOOR_TYPE_ONE_READER ? "1reader" : "2readers",
             cfg->antipassback ? "ON" : "OFF");
    return ESP_OK;
}

esp_err_t sys_config_save(const sys_config_t *cfg)
{
    nvs_handle_t h;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &h);
    if (err != ESP_OK) return err;

    nvs_set_str(h, KEY_LOGIN, cfg->login);
    nvs_set_str(h, KEY_PASS, cfg->password);
    nvs_set_str(h, KEY_SYSLOG_HOST, cfg->syslog_host);
    nvs_set_u8(h, KEY_SYSLOG_EN, cfg->syslog_enabled ? 1 : 0);
    nvs_set_u16(h, KEY_SYSLOG_PORT, cfg->syslog_port);

    nvs_set_u8(h, KEY_NET_MODE, cfg->net_mode);
    nvs_set_blob(h, KEY_IP, cfg->ip, 4);
    nvs_set_blob(h, KEY_MASK, cfg->mask, 4);
    nvs_set_blob(h, KEY_GW, cfg->gw, 4);

    nvs_set_u8(h, KEY_OP_MODE, cfg->op_mode);
    nvs_set_str(h, KEY_SRV_HOST, cfg->server_host);
    nvs_set_u16(h, KEY_SRV_PORT, cfg->server_port);
    nvs_set_str(h, KEY_SRV_PATH, cfg->server_path);

    nvs_set_u8(h, KEY_DOOR_TYPE, cfg->door_type);
    nvs_set_u8(h, KEY_APB, cfg->antipassback ? 1 : 0);

    /* schedules */
    nvs_set_blob(h, KEY_WEEK_EN, cfg->weekly_enabled, 7);
    {
        uint8_t tmp_on[14], tmp_off[14];
        for (int i = 0; i < 7; i++) {
            tmp_on[2*i]   = cfg->weekly_on[i] & 0xFF;
            tmp_on[2*i+1] = (cfg->weekly_on[i] >> 8) & 0xFF;
            tmp_off[2*i]   = cfg->weekly_off[i] & 0xFF;
            tmp_off[2*i+1] = (cfg->weekly_off[i] >> 8) & 0xFF;
        }
        nvs_set_blob(h, KEY_WEEK_ON, tmp_on, 14);
        nvs_set_blob(h, KEY_WEEK_OFF, tmp_off, 14);
    }
    nvs_set_u8(h, KEY_FLT_EN, cfg->floating_enabled ? 1 : 0);
    nvs_set_u32(h, KEY_FLT_START, cfg->floating_start);
    nvs_set_u32(h, KEY_FLT_END, cfg->floating_end);

    nvs_set_str(h, KEY_NTP_SERVER, cfg->ntp_server);
    nvs_set_u32(h, KEY_NTP_INTERVAL, cfg->ntp_interval_sec);
    nvs_set_i32(h, KEY_TZ_MIN, (int32_t)cfg->timezone_min);
    nvs_set_str(h, KEY_DEV_NAME, cfg->device_name);
    nvs_set_str(h, KEY_DEV_LOC, cfg->device_location);

    err = nvs_commit(h);
    nvs_close(h);

    if (err == ESP_OK) {
        ESP_LOGI(TAG, "Saved system config");
    }
    return err;
}
