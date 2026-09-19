#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <sys/param.h>
#include "web_server.h"
#include "config.h"
#include "door_hw.h"
#include "event_log.h"
#include "syslog_udp.h"
#include "card_db.h"
#include "time_sync.h"
#include "ota.h"
#include "esp_timer.h"
#include "esp_system.h"
#include <time.h>
#include "esp_log.h"
#include "esp_http_server.h"
#include "cJSON.h"
#include "esp_system.h"

static const char *TAG = "web";

static httpd_handle_t s_server = NULL;
static web_config_changed_cb_t s_cb = NULL;
static web_sysconfig_changed_cb_t s_sys_cb = NULL;
static void *s_cb_ctx = NULL;
static pin_config_t s_current;      /* working copy of pins */
static sys_config_t s_sys_current;  /* working copy of system config */

/* ---- helpers ---- */

static esp_err_t send_json(httpd_req_t *req, cJSON *root, int status)
{
    char *buf = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    httpd_resp_set_type(req, "application/json");
    httpd_resp_set_status(req, status == 200 ? HTTPD_200 : HTTPD_400);
    esp_err_t r = httpd_resp_sendstr(req, buf);
    free(buf);
    return r;
}

/* ---- minimal base64 decode ---- */
static int b64_val(char c)
{
    if (c >= 'A' && c <= 'Z') return c - 'A';
    if (c >= 'a' && c <= 'z') return c - 'a' + 26;
    if (c >= '0' && c <= '9') return c - '0' + 52;
    if (c == '+') return 62;
    if (c == '/') return 63;
    return -1;
}

static int b64_decode(const char *src, int len, char *out)
{
    int o = 0, i = 0;
    uint32_t acc = 0;
    int bits = 0;
    for (i = 0; i < len; i++) {
        char c = src[i];
        if (c == '=') break;
        int v = b64_val(c);
        if (v < 0) continue;
        acc = (acc << 6) | (uint32_t)v;
        bits += 6;
        if (bits >= 8) {
            bits -= 8;
            out[o++] = (char)((acc >> bits) & 0xFF);
        }
    }
    return o;
}

/* ---- Basic Auth check ---- */
static bool check_auth(httpd_req_t *req)
{
    size_t auth_len = httpd_req_get_hdr_value_len(req, "Authorization");
    if (auth_len == 0) return false;

    char *auth = calloc(1, auth_len + 1);
    if (!auth) return false;
    if (httpd_req_get_hdr_value_str(req, "Authorization", auth, auth_len + 1)
        != ESP_OK) {
        free(auth);
        return false;
    }

    const char *prefix = "Basic ";
    bool ok = false;
    if (strncmp(auth, prefix, strlen(prefix)) == 0) {
        const char *b64 = auth + strlen(prefix);
        char decoded[128];
        int dlen = b64_decode(b64, strlen(b64), decoded);

        char expected[128];
        snprintf(expected, sizeof(expected), "%s:%s",
                 s_sys_current.login, s_sys_current.password);
        if (dlen == (int)strlen(expected)
            && memcmp(decoded, expected, dlen) == 0) {
            ok = true;
        }
    }
    free(auth);
    return ok;
}

static esp_err_t auth_fail(httpd_req_t *req)
{
    event_log_add(EVT_AUTH_FAIL, 0, 0, "auth");
    httpd_resp_set_status(req, "401 Unauthorized");
    httpd_resp_set_hdr(req, "WWW-Authenticate", "Basic realm=\"SKUD\"");
    return httpd_resp_sendstr(req, "401 Unauthorized");
}

/* Serve embedded index.html */
static esp_err_t root_get_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    extern const unsigned char index_html_start[] asm("_binary_index_html_start");
    extern const unsigned char index_html_end[]   asm("_binary_index_html_end");
    const size_t size = (index_html_end - index_html_start);
    httpd_resp_set_type(req, "text/html");
    httpd_resp_send(req, (const char *)index_html_start, size);
    return ESP_OK;
}

/* GET /api/config -> current pin config */
static esp_err_t config_get_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    cJSON *root = cJSON_CreateObject();
    cJSON_AddNumberToObject(root, "lock_gpio", s_current.lock_gpio);
    cJSON_AddNumberToObject(root, "rte_gpio", s_current.rte_gpio);
    cJSON_AddNumberToObject(root, "sensor_gpio", s_current.sensor_gpio);
    cJSON_AddNumberToObject(root, "wiegand_d0", s_current.wiegand_d0);
    cJSON_AddNumberToObject(root, "wiegand_d1", s_current.wiegand_d1);
    cJSON_AddNumberToObject(root, "wiegand2_d0", s_current.wiegand2_d0);
    cJSON_AddNumberToObject(root, "wiegand2_d1", s_current.wiegand2_d1);
    cJSON_AddNumberToObject(root, "buzzer_gpio", s_current.buzzer_gpio);
    cJSON_AddNumberToObject(root, "lock_pulse_ms", s_current.lock_pulse_ms);
    return send_json(req, root, 200);
}

/* POST /api/config  body: JSON {lock_gpio, rte_gpio, ...} */
static esp_err_t config_post_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    char buf[512];
    int len = httpd_req_recv(req, buf, sizeof(buf) - 1);
    if (len <= 0) return ESP_FAIL;
    buf[len] = 0;

    cJSON *in = cJSON_Parse(buf);
    if (!in) {
        cJSON *err = cJSON_CreateObject();
        cJSON_AddStringToObject(err, "error", "invalid json");
        return send_json(req, err, 400);
    }

    pin_config_t next = s_current;
    cJSON *it;
    if ((it = cJSON_GetObjectItem(in, "lock_gpio")))    next.lock_gpio   = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "rte_gpio")))     next.rte_gpio    = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "sensor_gpio")))  next.sensor_gpio = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "wiegand_d0")))   next.wiegand_d0  = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "wiegand_d1")))   next.wiegand_d1  = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "wiegand2_d0")))  next.wiegand2_d0 = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "wiegand2_d1")))  next.wiegand2_d1 = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "buzzer_gpio")))  next.buzzer_gpio = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "lock_pulse_ms"))) next.lock_pulse_ms = it->valueint;
    cJSON_Delete(in);

    config_save(&next);
    s_current = next;

    event_log_add(EVT_CONFIG_CHANGED, 0, 0, "pins");

    if (s_cb) s_cb(&next, s_cb_ctx);

    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    cJSON_AddStringToObject(ok, "info", "applied live");
    return send_json(req, ok, 200);
}

/* GET /api/sysconfig -> auth + syslog settings */
static esp_err_t sysconfig_get_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "login", s_sys_current.login);
    cJSON_AddStringToObject(root, "password", s_sys_current.password);
    cJSON_AddBoolToObject(root, "syslog_enabled", s_sys_current.syslog_enabled);
    cJSON_AddStringToObject(root, "syslog_host", s_sys_current.syslog_host);
    cJSON_AddNumberToObject(root, "syslog_port", s_sys_current.syslog_port);

    /* network */
    cJSON_AddNumberToObject(root, "net_mode", s_sys_current.net_mode);
    char ip[16], mask[16], gw[16];
    snprintf(ip, sizeof(ip), "%u.%u.%u.%u", s_sys_current.ip[0], s_sys_current.ip[1],
             s_sys_current.ip[2], s_sys_current.ip[3]);
    snprintf(mask, sizeof(mask), "%u.%u.%u.%u", s_sys_current.mask[0], s_sys_current.mask[1],
             s_sys_current.mask[2], s_sys_current.mask[3]);
    snprintf(gw, sizeof(gw), "%u.%u.%u.%u", s_sys_current.gw[0], s_sys_current.gw[1],
             s_sys_current.gw[2], s_sys_current.gw[3]);
    cJSON_AddStringToObject(root, "ip", ip);
    cJSON_AddStringToObject(root, "mask", mask);
    cJSON_AddStringToObject(root, "gw", gw);

    /* server */
    cJSON_AddNumberToObject(root, "op_mode", s_sys_current.op_mode);
    cJSON_AddStringToObject(root, "server_host", s_sys_current.server_host);
    cJSON_AddNumberToObject(root, "server_port", s_sys_current.server_port);
    cJSON_AddStringToObject(root, "server_path", s_sys_current.server_path);

    /* door */
    cJSON_AddNumberToObject(root, "door_type", s_sys_current.door_type);
    cJSON_AddBoolToObject(root, "antipassback", s_sys_current.antipassback);

    /* schedules */
    cJSON *week = cJSON_AddArrayToObject(root, "weekly");
    for (int i = 0; i < 7; i++) {
        cJSON *d = cJSON_CreateObject();
        cJSON_AddNumberToObject(d, "day", i);
        cJSON_AddNumberToObject(d, "enabled", s_sys_current.weekly_enabled[i]);
        cJSON_AddNumberToObject(d, "on", s_sys_current.weekly_on[i]);
        cJSON_AddNumberToObject(d, "off", s_sys_current.weekly_off[i]);
        cJSON_AddItemToArray(week, d);
    }
    cJSON_AddBoolToObject(root, "floating_enabled", s_sys_current.floating_enabled);
    cJSON_AddNumberToObject(root, "floating_start", s_sys_current.floating_start);
    cJSON_AddNumberToObject(root, "floating_end", s_sys_current.floating_end);

    /* time + identity */
    cJSON_AddStringToObject(root, "ntp_server", s_sys_current.ntp_server);
    cJSON_AddNumberToObject(root, "ntp_interval_sec", s_sys_current.ntp_interval_sec);
    cJSON_AddNumberToObject(root, "timezone_min", s_sys_current.timezone_min);
    cJSON_AddStringToObject(root, "device_name", s_sys_current.device_name);
    cJSON_AddStringToObject(root, "device_location", s_sys_current.device_location);

    return send_json(req, root, 200);
}

/* POST /api/sysconfig -> save auth + syslog */
static esp_err_t sysconfig_post_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    char buf[512];
    int len = httpd_req_recv(req, buf, sizeof(buf) - 1);
    if (len <= 0) return ESP_FAIL;
    buf[len] = 0;

    cJSON *in = cJSON_Parse(buf);
    if (!in) {
        cJSON *err = cJSON_CreateObject();
        cJSON_AddStringToObject(err, "error", "invalid json");
        return send_json(req, err, 400);
    }

    sys_config_t next = s_sys_current;
    cJSON *it;
    if ((it = cJSON_GetObjectItem(in, "login")) && it->valuestring) {
        strncpy(next.login, it->valuestring, CFG_LOGIN_MAX - 1);
        next.login[CFG_LOGIN_MAX - 1] = '\0';
    }
    if ((it = cJSON_GetObjectItem(in, "password")) && it->valuestring
        && it->valuestring[0]) {
        strncpy(next.password, it->valuestring, CFG_PASS_MAX - 1);
        next.password[CFG_PASS_MAX - 1] = '\0';
    }
    if ((it = cJSON_GetObjectItem(in, "syslog_enabled"))) next.syslog_enabled = cJSON_IsTrue(it);
    if ((it = cJSON_GetObjectItem(in, "syslog_host")) && it->valuestring) {
        strncpy(next.syslog_host, it->valuestring, CFG_SYSLOG_HOST_MAX - 1);
        next.syslog_host[CFG_SYSLOG_HOST_MAX - 1] = '\0';
    }
    if ((it = cJSON_GetObjectItem(in, "syslog_port"))) next.syslog_port = it->valueint;

    /* network */
    if ((it = cJSON_GetObjectItem(in, "net_mode"))) next.net_mode = (uint8_t)it->valueint;
    if ((it = cJSON_GetObjectItem(in, "ip")) && it->valuestring) {
        unsigned a, b, c, d;
        if (sscanf(it->valuestring, "%u.%u.%u.%u", &a, &b, &c, &d) == 4) {
            next.ip[0]=a; next.ip[1]=b; next.ip[2]=c; next.ip[3]=d;
        }
    }
    if ((it = cJSON_GetObjectItem(in, "mask")) && it->valuestring) {
        unsigned a, b, c, d;
        if (sscanf(it->valuestring, "%u.%u.%u.%u", &a, &b, &c, &d) == 4) {
            next.mask[0]=a; next.mask[1]=b; next.mask[2]=c; next.mask[3]=d;
        }
    }
    if ((it = cJSON_GetObjectItem(in, "gw")) && it->valuestring) {
        unsigned a, b, c, d;
        if (sscanf(it->valuestring, "%u.%u.%u.%u", &a, &b, &c, &d) == 4) {
            next.gw[0]=a; next.gw[1]=b; next.gw[2]=c; next.gw[3]=d;
        }
    }

    /* server */
    if ((it = cJSON_GetObjectItem(in, "op_mode"))) next.op_mode = (uint8_t)it->valueint;
    if ((it = cJSON_GetObjectItem(in, "server_host")) && it->valuestring) {
        strncpy(next.server_host, it->valuestring, CFG_SERVER_HOST_MAX - 1);
        next.server_host[CFG_SERVER_HOST_MAX - 1] = '\0';
    }
    if ((it = cJSON_GetObjectItem(in, "server_port"))) next.server_port = it->valueint;
    if ((it = cJSON_GetObjectItem(in, "server_path")) && it->valuestring) {
        strncpy(next.server_path, it->valuestring, CFG_SERVER_PATH_MAX - 1);
        next.server_path[CFG_SERVER_PATH_MAX - 1] = '\0';
    }

    /* door */
    if ((it = cJSON_GetObjectItem(in, "door_type"))) next.door_type = (uint8_t)it->valueint;
    if ((it = cJSON_GetObjectItem(in, "antipassback"))) next.antipassback = cJSON_IsTrue(it);

    /* schedules */
    if ((it = cJSON_GetObjectItem(in, "weekly")) && cJSON_IsArray(it)) {
        int n = cJSON_GetArraySize(it);
        for (int i = 0; i < n && i < 7; i++) {
            cJSON *d = cJSON_GetArrayItem(it, i);
            cJSON *f;
            if ((f = cJSON_GetObjectItem(d, "day"))) {
                int day = f->valueint;
                if (day < 0 || day > 6) continue;
                if ((f = cJSON_GetObjectItem(d, "enabled"))) next.weekly_enabled[day] = (uint8_t)f->valueint;
                if ((f = cJSON_GetObjectItem(d, "on"))) next.weekly_on[day] = (uint16_t)f->valueint;
                if ((f = cJSON_GetObjectItem(d, "off"))) next.weekly_off[day] = (uint16_t)f->valueint;
            }
        }
    }
    if ((it = cJSON_GetObjectItem(in, "floating_enabled"))) next.floating_enabled = cJSON_IsTrue(it);
    if ((it = cJSON_GetObjectItem(in, "floating_start"))) next.floating_start = (uint32_t)it->valuedouble;
    if ((it = cJSON_GetObjectItem(in, "floating_end"))) next.floating_end = (uint32_t)it->valuedouble;

    /* time + identity */
    if ((it = cJSON_GetObjectItem(in, "ntp_server")) && it->valuestring) {
        strncpy(next.ntp_server, it->valuestring, CFG_NTP_HOST_MAX - 1);
        next.ntp_server[CFG_NTP_HOST_MAX - 1] = '\0';
    }
    if ((it = cJSON_GetObjectItem(in, "ntp_interval_sec"))) next.ntp_interval_sec = (uint32_t)it->valuedouble;
    if ((it = cJSON_GetObjectItem(in, "timezone_min"))) next.timezone_min = (int16_t)it->valueint;
    if ((it = cJSON_GetObjectItem(in, "device_name")) && it->valuestring) {
        strncpy(next.device_name, it->valuestring, CFG_DEV_NAME_MAX - 1);
        next.device_name[CFG_DEV_NAME_MAX - 1] = '\0';
    }
    if ((it = cJSON_GetObjectItem(in, "device_location")) && it->valuestring) {
        strncpy(next.device_location, it->valuestring, CFG_DEV_LOC_MAX - 1);
        next.device_location[CFG_DEV_LOC_MAX - 1] = '\0';
    }

    cJSON_Delete(in);

    sys_config_save(&next);
    s_sys_current = next;

    event_log_add(EVT_CONFIG_CHANGED, 0, 0, "sysconf");

    if (s_sys_cb) s_sys_cb(&next, s_cb_ctx);

    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    return send_json(req, ok, 200);
}

/* GET /api/log -> event list */
static esp_err_t log_get_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    int count = event_log_count();
    cJSON *root = cJSON_CreateObject();
    cJSON *arr = cJSON_AddArrayToObject(root, "events");

    int start = count > 200 ? count - 200 : 0;
    for (int i = start; i < count; i++) {
        event_record_t rec;
        if (event_log_get(i, &rec) != ESP_OK) continue;
        cJSON *e = cJSON_CreateObject();
        cJSON_AddNumberToObject(e, "ts", rec.ts);
        cJSON_AddNumberToObject(e, "type", rec.type);
        cJSON_AddNumberToObject(e, "facility", rec.facility);
        cJSON_AddNumberToObject(e, "card", rec.card);
        cJSON_AddStringToObject(e, "note", rec.note);
        cJSON_AddNumberToObject(e, "flags", rec.flags);
        cJSON_AddItemToArray(arr, e);
    }
    cJSON_AddNumberToObject(root, "count", count);
    return send_json(req, root, 200);
}

/* POST /api/log/clear */
static esp_err_t log_clear_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);
    event_log_clear();
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    return send_json(req, ok, 200);
}

/* POST /api/door/open -> open the door now */
static esp_err_t door_open_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);
    door_hw_open();
    event_log_add(EVT_RTE_PRESSED, 0, 0, "web");
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    return send_json(req, ok, 200);
}

/* ---- Card database API ---- */

/* GET /api/cards -> list all cards */
static esp_err_t cards_get_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    int count = card_db_count();
    cJSON *root = cJSON_CreateObject();
    cJSON *arr = cJSON_AddArrayToObject(root, "cards");
    for (int i = 0; i < count; i++) {
        card_record_t r;
        if (card_db_get(i, &r) != ESP_OK) continue;
        cJSON *c = cJSON_CreateObject();
        cJSON_AddNumberToObject(c, "facility", r.facility);
        cJSON_AddNumberToObject(c, "card", r.card);
        cJSON_AddStringToObject(c, "name", r.name);
        cJSON_AddNumberToObject(c, "access", r.access);
        cJSON_AddStringToObject(c, "group", r.group);
        cJSON_AddBoolToObject(c, "active", r.active);
        cJSON_AddItemToArray(arr, c);
    }
    cJSON_AddNumberToObject(root, "count", count);
    return send_json(req, root, 200);
}

/* Parse a card JSON object into record. Returns true on success. */
static bool parse_card(cJSON *obj, card_record_t *out)
{
    memset(out, 0, sizeof(*out));
    cJSON *it;
    if ((it = cJSON_GetObjectItem(obj, "facility"))) out->facility = (uint8_t)it->valueint;
    if ((it = cJSON_GetObjectItem(obj, "card"))) out->card = (uint16_t)it->valueint;
    if ((it = cJSON_GetObjectItem(obj, "name")) && it->valuestring)
        strncpy(out->name, it->valuestring, CARD_NAME_MAX - 1);
    if ((it = cJSON_GetObjectItem(obj, "access"))) out->access = (uint8_t)it->valueint;
    if ((it = cJSON_GetObjectItem(obj, "group")) && it->valuestring)
        strncpy(out->group, it->valuestring, CARD_GROUP_MAX - 1);
    if ((it = cJSON_GetObjectItem(obj, "active"))) out->active = cJSON_IsTrue(it);
    return true;
}

/* POST /api/cards/add  body: {facility,card,name,access,group,active} */
static esp_err_t cards_add_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    char buf[512];
    int len = httpd_req_recv(req, buf, sizeof(buf) - 1);
    if (len <= 0) return ESP_FAIL;
    buf[len] = 0;

    cJSON *in = cJSON_Parse(buf);
    if (!in) {
        cJSON *err = cJSON_CreateObject();
        cJSON_AddStringToObject(err, "error", "invalid json");
        return send_json(req, err, 400);
    }

    card_record_t rec;
    parse_card(in, &rec);
    cJSON_Delete(in);

    esp_err_t r = card_db_add(&rec);
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", r == ESP_OK);
    if (r == ESP_ERR_NO_MEM) cJSON_AddStringToObject(ok, "error", "full");
    else if (r == ESP_ERR_INVALID_STATE) cJSON_AddStringToObject(ok, "error", "exists");
    return send_json(req, ok, 200);
}

/* POST /api/cards/update */
static esp_err_t cards_update_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    char buf[512];
    int len = httpd_req_recv(req, buf, sizeof(buf) - 1);
    if (len <= 0) return ESP_FAIL;
    buf[len] = 0;

    cJSON *in = cJSON_Parse(buf);
    if (!in) {
        cJSON *err = cJSON_CreateObject();
        cJSON_AddStringToObject(err, "error", "invalid json");
        return send_json(req, err, 400);
    }
    card_record_t rec;
    parse_card(in, &rec);
    cJSON_Delete(in);

    esp_err_t r = card_db_update(&rec);
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", r == ESP_OK);
    if (r == ESP_ERR_NOT_FOUND) cJSON_AddStringToObject(ok, "error", "not found");
    return send_json(req, ok, 200);
}

/* POST /api/cards/remove body: {facility,card} */
static esp_err_t cards_remove_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    char buf[128];
    int len = httpd_req_recv(req, buf, sizeof(buf) - 1);
    if (len <= 0) return ESP_FAIL;
    buf[len] = 0;

    cJSON *in = cJSON_Parse(buf);
    if (!in) return ESP_FAIL;
    cJSON *it;
    uint8_t f = 0; uint16_t c = 0;
    if ((it = cJSON_GetObjectItem(in, "facility"))) f = (uint8_t)it->valueint;
    if ((it = cJSON_GetObjectItem(in, "card"))) c = (uint16_t)it->valueint;
    cJSON_Delete(in);

    esp_err_t r = card_db_remove(f, c);
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", r == ESP_OK);
    return send_json(req, ok, 200);
}

/* POST /api/cards/clear */
static esp_err_t cards_clear_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);
    card_db_clear();
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    return send_json(req, ok, 200);
}

/* GET /api/cards/export -> JSON array of all cards */
static esp_err_t cards_export_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    int count = card_db_count();
    cJSON *arr = cJSON_CreateArray();
    for (int i = 0; i < count; i++) {
        card_record_t r;
        if (card_db_get(i, &r) != ESP_OK) continue;
        cJSON *c = cJSON_CreateObject();
        cJSON_AddNumberToObject(c, "facility", r.facility);
        cJSON_AddNumberToObject(c, "card", r.card);
        cJSON_AddStringToObject(c, "name", r.name);
        cJSON_AddNumberToObject(c, "access", r.access);
        cJSON_AddStringToObject(c, "group", r.group);
        cJSON_AddBoolToObject(c, "active", r.active);
        cJSON_AddItemToArray(arr, c);
    }
    /* send raw (not wrapped in {ok}) for easy save-as-file */
    char *buf = cJSON_Print(arr);
    cJSON_Delete(arr);
    httpd_resp_set_type(req, "application/json");
    httpd_resp_set_hdr(req, "Content-Disposition", "attachment; filename=\"cards.json\"");
    httpd_resp_sendstr(req, buf);
    free(buf);
    return ESP_OK;
}

/* POST /api/cards/import body: JSON array of cards. mode in query? optional clear */
static esp_err_t cards_import_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    char *buf = malloc(8192);
    if (!buf) return ESP_FAIL;
    int total = 0;
    int done = 0;
    int off = 0;
    int len;
    while ((len = httpd_req_recv(req, buf + off, 1024)) > 0) {
        off += len;
        if (off > 8000) break;
    }
    buf[off] = 0;

    cJSON *arr = cJSON_Parse(buf);
    int ok_flag = 0;
    if (arr && cJSON_IsArray(arr)) {
        total = cJSON_GetArraySize(arr);
        for (int i = 0; i < total; i++) {
            cJSON *obj = cJSON_GetArrayItem(arr, i);
            card_record_t rec;
            parse_card(obj, &rec);
            if (card_db_find(rec.facility, rec.card) >= 0) {
                card_db_update(&rec);
            } else {
                if (card_db_add(&rec) == ESP_OK) done++;
            }
        }
        ok_flag = 1;
    }

    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", ok_flag == 1);
    cJSON_AddNumberToObject(ok, "imported", done);
    cJSON_AddNumberToObject(ok, "total", total);
    free(buf);
    if (arr) cJSON_Delete(arr);
    return send_json(req, ok, 200);
}

/* POST /api/cards/learn  body: {name} -> arm learn mode; GET -> status */
static esp_err_t cards_learn_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    if (req->method == HTTP_POST) {
        char buf[128];
        int len = httpd_req_recv(req, buf, sizeof(buf) - 1);
        if (len > 0) {
            buf[len] = 0;
            cJSON *in = cJSON_Parse(buf);
            const char *name = "";
            if (in) {
                cJSON *it = cJSON_GetObjectItem(in, "name");
                if (it && it->valuestring) name = it->valuestring;
                cJSON_Delete(in);
            }
            card_db_learn_arm(name);
        }
    }

    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "learn_active", card_db_learn_active());
    return send_json(req, ok, 200);
}

/* POST /api/cards/learn/cancel */
static esp_err_t cards_learn_cancel_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);
    card_db_learn_cancel();
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    return send_json(req, ok, 200);
}

/* GET /api/status -> uptime, memory, time, identity */
static esp_err_t status_get_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    int64_t now_us = esp_timer_get_time();
    uint32_t uptime_sec = (uint32_t)(now_us / 1000000ULL);

    /* free heap */
    size_t free_heap = esp_get_free_heap_size();

    time_t t = time(NULL);
    struct tm tm;
    localtime_r(&t, &tm);
    char timebuf[32];
    strftime(timebuf, sizeof(timebuf), "%Y-%m-%d %H:%M:%S", &tm);

    cJSON *root = cJSON_CreateObject();
    cJSON_AddNumberToObject(root, "uptime_sec", uptime_sec);
    cJSON_AddNumberToObject(root, "free_heap", free_heap);
    cJSON_AddNumberToObject(root, "now", (uint32_t)t);
    cJSON_AddStringToObject(root, "time_str", timebuf);
    cJSON_AddBoolToObject(root, "time_synced", time_sync_synced());
    cJSON_AddStringToObject(root, "device_name", s_sys_current.device_name);
    cJSON_AddStringToObject(root, "device_location", s_sys_current.device_location);
    cJSON_AddNumberToObject(root, "card_count", card_db_count());
    cJSON_AddNumberToObject(root, "event_count", event_log_count());
    return send_json(req, root, 200);
}

/* POST /api/time/sync -> manual NTP sync */
static esp_err_t time_sync_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);
    time_sync_now();
    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", true);
    return send_json(req, ok, 200);
}

/* POST /api/ota -> firmware upload (raw .bin body) */
static esp_err_t ota_upload_handler(httpd_req_t *req)
{
    if (!check_auth(req)) return auth_fail(req);

    esp_err_t err = ota_begin();
    if (err != ESP_OK) {
        cJSON *e = cJSON_CreateObject();
        cJSON_AddBoolToObject(e, "ok", false);
        cJSON_AddStringToObject(e, "error", "ota_begin");
        return send_json(req, e, 400);
    }

    /* stream body in chunks */
    int remaining = req->content_len;
    uint8_t buf[4096];
    bool failed = false;
    while (remaining > 0) {
        int to_read = remaining < (int)sizeof(buf) ? remaining : (int)sizeof(buf);
        int got = httpd_req_recv(req, (char *)buf, to_read);
        if (got <= 0) {
            failed = true;
            break;
        }
        if (ota_write(buf, got) != ESP_OK) {
            failed = true;
            break;
        }
        remaining -= got;
    }

    if (failed) {
        ota_abort();
        cJSON *e = cJSON_CreateObject();
        cJSON_AddBoolToObject(e, "ok", false);
        cJSON_AddStringToObject(e, "error", "write_failed");
        return send_json(req, e, 400);
    }

    err = ota_commit(); /* reboots on success */

    cJSON *ok = cJSON_CreateObject();
    cJSON_AddBoolToObject(ok, "ok", err == ESP_OK);
    return send_json(req, ok, 200);
}

static httpd_handle_t start_server(void)
{
    httpd_config_t cfg = HTTPD_DEFAULT_CONFIG();
    cfg.max_uri_handlers = 32;
    httpd_handle_t h = NULL;
    if (httpd_start(&h, &cfg) == ESP_OK) {
        httpd_uri_t u;

        #define REG(uri_, method_, handler_) do { \
            memset(&u, 0, sizeof(u)); \
            u.uri = uri_; u.method = method_; u.handler = handler_; \
            httpd_register_uri_handler(h, &u); \
        } while(0)

        REG("/", HTTP_GET, root_get_handler);
        REG("/api/config", HTTP_GET, config_get_handler);
        REG("/api/config", HTTP_POST, config_post_handler);
        REG("/api/sysconfig", HTTP_GET, sysconfig_get_handler);
        REG("/api/sysconfig", HTTP_POST, sysconfig_post_handler);
        REG("/api/log", HTTP_GET, log_get_handler);
        REG("/api/log/clear", HTTP_POST, log_clear_handler);
        REG("/api/door/open", HTTP_POST, door_open_handler);
        REG("/api/cards", HTTP_GET, cards_get_handler);
        REG("/api/cards/add", HTTP_POST, cards_add_handler);
        REG("/api/cards/update", HTTP_POST, cards_update_handler);
        REG("/api/cards/remove", HTTP_POST, cards_remove_handler);
        REG("/api/cards/clear", HTTP_POST, cards_clear_handler);
        REG("/api/cards/export", HTTP_GET, cards_export_handler);
        REG("/api/cards/import", HTTP_POST, cards_import_handler);
        REG("/api/cards/learn", HTTP_POST, cards_learn_handler);
        REG("/api/cards/learn", HTTP_GET, cards_learn_handler);
        REG("/api/cards/learn/cancel", HTTP_POST, cards_learn_cancel_handler);
        REG("/api/status", HTTP_GET, status_get_handler);
        REG("/api/time/sync", HTTP_POST, time_sync_handler);
        REG("/api/ota", HTTP_POST, ota_upload_handler);

        #undef REG
    }
    return h;
}

esp_err_t web_server_init(web_config_changed_cb_t on_change,
                          web_sysconfig_changed_cb_t on_sys_change, void *ctx)
{
    s_cb = on_change;
    s_sys_cb = on_sys_change;
    s_cb_ctx = ctx;
    config_load(&s_current);
    sys_config_load(&s_sys_current);

    /* apply syslog config on startup */
    syslog_udp_config(s_sys_current.syslog_host, s_sys_current.syslog_port,
                      s_sys_current.syslog_enabled);

    s_server = start_server();
    if (!s_server) {
        ESP_LOGE(TAG, "HTTP server failed to start");
        return ESP_FAIL;
    }
    ESP_LOGI(TAG, "HTTP server started on port 80 (auth enabled)");
    return ESP_OK;
}
