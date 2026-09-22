#include <stdio.h>
#include <string.h>
#include <time.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "esp_log.h"
#include "esp_system.h"
#include "nvs_flash.h"
#include "esp_event.h"
#include "esp_netif.h"

#include "config.h"
#include "eth_netif.h"
#include "wiegand.h"
#include "door_hw.h"
#include "web_server.h"
#include "event_log.h"
#include "syslog_udp.h"
#include "buzzer.h"
#include "schedule.h"
#include "card_db.h"
#include "time_sync.h"
#include "event_push.h"
#include "fw_info.h"
#include "ota.h"

static const char *TAG = "main";

static pin_config_t s_pins;
static sys_config_t s_sys;

/* antipassback state (strict): last recognized direction */
static uint8_t s_apb_state = 0;  /* 0=unknown, 1=inside, 2=outside */

/* log an event locally + to syslog + (if network mode) push to server */
static void log_event(event_type_t type, uint8_t facility, uint16_t card,
                      const char *note, int severity)
{
    event_log_add(type, facility, card, note);

    /* push to central server in network mode */
    event_push_send((int)type, facility, card, note, (uint32_t)time(NULL), 0);

    char msg[64];
    if (card) {
        snprintf(msg, sizeof(msg), "facility=%u card=%u %s", facility, card,
                 note ? note : "");
    } else {
        snprintf(msg, sizeof(msg), "%s", note ? note : "");
    }
    syslog_udp_send(16 /* local0 */, severity, "skud", msg);
}

/* called when a card is read; direction: 0 = IN reader, 1 = OUT reader */
static void on_card_reader(const wiegand_card_t *card, int direction)
{
    ESP_LOGI(TAG, ">>> ACCESS REQUEST [%s]: facility=%u card=%u",
             direction == 0 ? "IN" : "OUT", card->facility, card->card);

    /* learn mode: capture the card instead of access decision */
    if (card_db_learn_active()) {
        if (card_db_learn_capture(card->facility, card->card)) {
            buzzer_beep(2, 2400, 80, 80);
        }
        return;
    }

    /* whitelist check */
    int idx = card_db_find(card->facility, card->card);
    if (idx < 0) {
        log_event(EVT_CARD_DENIED, card->facility, card->card, "not-in-whitelist", 4);
        buzzer_beep(3, 2000, 80, 120);
        return;
    }

    card_record_t rec;
    card_db_get(idx, &rec);
    if (!rec.active) {
        log_event(EVT_CARD_DENIED, card->facility, card->card, "card-disabled", 4);
        buzzer_beep(3, 2000, 80, 120);
        return;
    }

    /* schedule check: scheduled cards must be within schedule */
    if (rec.access == CARD_ACCESS_SCHEDULED) {
        if (!schedule_allowed(&s_sys, NULL)) {
            log_event(EVT_CARD_DENIED, card->facility, card->card, "out-of-schedule", 4);
            buzzer_beep(3, 2000, 80, 120);
            return;
        }
    }

    /* antipassback (strict, direction based) */
    bool granted = true;
    if (s_sys.antipassback) {
        if (direction == 0) {
            /* entering: must be outside */
            if (s_apb_state == 1) granted = false; /* already inside */
            else if (s_apb_state != 0) s_apb_state = 1;
        } else {
            /* leaving: must be inside */
            if (s_apb_state == 2) granted = false; /* already outside */
            else if (s_apb_state != 0) s_apb_state = 2;
        }
        if (!granted) {
            log_event(EVT_CARD_DENIED, card->facility, card->card, "antipassback", 4);
            buzzer_beep(3, 2000, 80, 120);
            return;
        }
    }

    log_event(EVT_CARD_GRANTED, card->facility, card->card, rec.name, 6);
    buzzer_beep(1, 2000, 120, 0);
    door_hw_open();
}

static void on_card(const wiegand_card_t *card, void *ctx)
{
    on_card_reader(card, 0); /* reader IN */
}

static void on_card_out(const wiegand_card_t *card, void *ctx)
{
    on_card_reader(card, 1); /* reader OUT */
}

static void on_door_event(door_event_t evt, void *ctx)
{
    switch (evt) {
    case DOOR_EVT_RTE_PRESSED:
        ESP_LOGI(TAG, "RTE pressed -> open door");
        door_hw_open();
        log_event(EVT_RTE_PRESSED, 0, 0, "rte", 6);
        buzzer_beep(1, 2000, 120, 0);
        break;
    case DOOR_EVT_SENSOR_OPEN:
        ESP_LOGW(TAG, "Door OPENED");
        log_event(EVT_DOOR_OPEN, 0, 0, "sensor", 5);
        event_log_mark_last_pass();  /* pass completed via reed switch */
        /* continuous warning beep while door is open */
        buzzer_beep(5, 1800, 60, 120);
        break;
    case DOOR_EVT_SENSOR_CLOSED:
        ESP_LOGI(TAG, "Door closed");
        log_event(EVT_DOOR_CLOSED, 0, 0, "sensor", 6);
        buzzer_stop();
        break;
    }
}

/* Re-init peripherals with new pins (called from web on config change) */
static void apply_pins(const pin_config_t *cfg)
{
    s_pins = *cfg;

    wiegand_deinit();
    door_hw_deinit();
    buzzer_deinit();

    door_hw_init(cfg->lock_gpio, cfg->rte_gpio, cfg->sensor_gpio,
                 cfg->lock_pulse_ms, on_door_event, NULL);

    wiegand_init(cfg->wiegand_d0, cfg->wiegand_d1, on_card, NULL);

    /* second reader if two-reader mode */
    if (s_sys.door_type == DOOR_TYPE_TWO_READERS
        && cfg->wiegand2_d0 >= 0 && cfg->wiegand2_d1 >= 0) {
        wiegand_init(cfg->wiegand2_d0, cfg->wiegand2_d1, on_card_out, NULL);
    }

    buzzer_init(cfg->buzzer_gpio);

    ESP_LOGI(TAG, "Pins re-applied");
}

static void on_config_changed(const pin_config_t *cfg, void *ctx)
{
    apply_pins(cfg);
}

static void on_sysconfig_changed(const sys_config_t *cfg, void *ctx)
{
    s_sys = *cfg;
    syslog_udp_config(cfg->syslog_host, cfg->syslog_port, cfg->syslog_enabled);
    time_sync_set_timezone(cfg->timezone_min);
    time_sync_start(cfg->ntp_server, cfg->ntp_interval_sec);
    /* re-apply pins so two-reader mode is reflected */
    apply_pins(&s_pins);
    /* configure event push to central server */
    event_push_configure(cfg->server_host, cfg->server_port, cfg->server_path,
                         cfg->device_name, cfg->op_mode == MODE_NETWORK);
    ESP_LOGI(TAG, "System config re-applied");
}

void app_main(void)
{
    ESP_LOGI(TAG, "SKUD controller starting (ESP32-P4) fw %s (%s)",
             FW_VERSION_STRING, FW_BUILD_DATE);

    /* NVS */
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    /* Читаем результат прошлой попытки OTA. Делается сразу после
     * инициализации NVS: если предыдущее обновление прервалось, причина
     * останется только здесь — консольный лог теряется при перезагрузке. */
    ota_read_journal();

    /* Network stack */
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    /* Ethernet */
    ESP_ERROR_CHECK(eth_netif_init());

    /* Event log */
    event_log_init();

    /* Card database */
    card_db_init();

    /* Load pin + system config */
    config_load(&s_pins);
    sys_config_load(&s_sys);

    /* Time sync (SNTP) + timezone */
    time_sync_set_timezone(s_sys.timezone_min);
    time_sync_start(s_sys.ntp_server, s_sys.ntp_interval_sec);

    /* Event push to central server (network mode) */
    event_push_init();
    event_push_configure(s_sys.server_host, s_sys.server_port, s_sys.server_path,
                         s_sys.device_name, s_sys.op_mode == MODE_NETWORK);

    /* Schedules checked at this point (placeholder) */

    /* Door hardware */
    door_hw_init(s_pins.lock_gpio, s_pins.rte_gpio, s_pins.sensor_gpio,
                 s_pins.lock_pulse_ms, on_door_event, NULL);

    /* Wiegand 26 reader(s) */
    wiegand_init(s_pins.wiegand_d0, s_pins.wiegand_d1, on_card, NULL);
    if (s_sys.door_type == DOOR_TYPE_TWO_READERS
        && s_pins.wiegand2_d0 >= 0 && s_pins.wiegand2_d1 >= 0) {
        wiegand_init(s_pins.wiegand2_d0, s_pins.wiegand2_d1, on_card_out, NULL);
    }

    /* Buzzer */
    buzzer_init(s_pins.buzzer_gpio);

    /* Web server (REST + UI) */
    ESP_ERROR_CHECK(web_server_init(on_config_changed, on_sysconfig_changed, NULL));

    log_event(EVT_SYSTEM_START, 0, 0, "boot", 6);

    /* Подтверждаем прошивку как рабочую.
     *
     * Делается только здесь — после поднятия сети и HTTP-сервера: если
     * новая прошивка упадёт раньше, подтверждения не будет, и загрузчик
     * вернётся к предыдущей версии. Подтверждение в начале app_main
     * отменило бы эту защиту. */
    confirm_firmware();

    ESP_LOGI(TAG, "Controller initialized");
}
