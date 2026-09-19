#pragma once

#include "esp_err.h"
#include "config.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Callback to notify main that pin config changed and should be re-applied */
typedef void (*web_config_changed_cb_t)(const pin_config_t *cfg, void *ctx);

/* Callback to notify main that system config (auth/syslog) changed */
typedef void (*web_sysconfig_changed_cb_t)(const sys_config_t *cfg, void *ctx);

esp_err_t web_server_init(web_config_changed_cb_t on_change,
                          web_sysconfig_changed_cb_t on_sys_change, void *ctx);

#ifdef __cplusplus
}
#endif
