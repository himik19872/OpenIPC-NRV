#pragma once

#include "esp_err.h"
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

/* -1 = GPIO_NUM_NC (not used) */
#define PIN_DISABLED    (-1)

#define CFG_LOGIN_MAX       32
#define CFG_PASS_MAX        32
#define CFG_SYSLOG_HOST_MAX 48
#define CFG_SERVER_HOST_MAX 64
#define CFG_SERVER_PATH_MAX 64
#define CFG_NTP_HOST_MAX    64
#define CFG_DEV_NAME_MAX    32
#define CFG_DEV_LOC_MAX     48

/* Door type */
typedef enum {
    DOOR_TYPE_ONE_READER = 0,   /* 1 reader + RTE button */
    DOOR_TYPE_TWO_READERS = 1   /* reader IN + reader OUT */
} door_type_t;

/* Operating mode */
typedef enum {
    MODE_AUTONOMOUS = 0,        /* fully offline, whitelist local */
    MODE_NETWORK = 1            /* server sync + events */
} op_mode_t;

/* Network mode */
typedef enum {
    NET_DHCP = 0,
    NET_STATIC = 1
} net_mode_t;

/* Pin configuration persisted in NVS */
typedef struct {
    int16_t lock_gpio;      /* relay / lock output */
    int16_t rte_gpio;       /* request-to-exit button (input) */
    int16_t sensor_gpio;    /* door reed switch (input) */
    int16_t wiegand_d0;     /* Wiegand DATA0 (reader IN) */
    int16_t wiegand_d1;     /* Wiegand DATA1 (reader IN) */
    int16_t wiegand2_d0;    /* Wiegand DATA0 (reader OUT, two-reader mode) */
    int16_t wiegand2_d1;    /* Wiegand DATA1 (reader OUT) */
    int16_t buzzer_gpio;    /* passive buzzer (LEDC PWM) */

    uint32_t lock_pulse_ms; /* lock open pulse duration */
} pin_config_t;

/* System configuration (auth + syslog + network + server + door) */
typedef struct {
    char login[CFG_LOGIN_MAX];
    char password[CFG_PASS_MAX];

    bool   syslog_enabled;
    char   syslog_host[CFG_SYSLOG_HOST_MAX];
    uint16_t syslog_port;

    /* network */
    uint8_t net_mode;            /* NET_DHCP / NET_STATIC */
    uint8_t ip[4];
    uint8_t mask[4];
    uint8_t gw[4];

    /* server */
    uint8_t op_mode;             /* MODE_AUTONOMOUS / MODE_NETWORK */
    char   server_host[CFG_SERVER_HOST_MAX];
    uint16_t server_port;
    char   server_path[CFG_SERVER_PATH_MAX];

    /* door behaviour */
    uint8_t door_type;           /* DOOR_TYPE_ONE_READER / TWO_READERS */
    bool   antipassback;         /* strict direction-based anti-passback */

    /* schedules (weekly: 7 days x on/off minutes; floating: date interval) */
    uint8_t weekly_enabled[7];   /* per weekday 0=off */
    uint16_t weekly_on[7];       /* minutes-from-midnight */
    uint16_t weekly_off[7];

    bool   floating_enabled;
    uint32_t floating_start;     /* unix ts */
    uint32_t floating_end;

    /* time sync (SNTP) */
    char   ntp_server[CFG_NTP_HOST_MAX];
    uint32_t ntp_interval_sec;   /* 0 = manual only */
    int16_t timezone_min;        /* offset from UTC in minutes (e.g. 180 = MSK) */

    /* device identity */
    char   device_name[CFG_DEV_NAME_MAX];
    char   device_location[CFG_DEV_LOC_MAX];
} sys_config_t;

/* Load config from NVS, applying defaults for missing entries */
esp_err_t config_load(pin_config_t *cfg);

/* Save config to NVS */
esp_err_t config_save(const pin_config_t *cfg);

/* Reset config to defaults (does not immediately write to NVS) */
void config_defaults(pin_config_t *cfg);

/* Load system config (auth + syslog + network + server + door) */
esp_err_t sys_config_load(sys_config_t *cfg);

/* Save system config (auth + syslog + network + server + door) */
esp_err_t sys_config_save(const sys_config_t *cfg);

/* Reset system config to defaults */
void sys_config_defaults(sys_config_t *cfg);

#ifdef __cplusplus
}
#endif
