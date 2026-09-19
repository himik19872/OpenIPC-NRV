#pragma once

#include "esp_err.h"
#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Configure the syslog target. host may be empty/disabled. */
esp_err_t syslog_udp_config(const char *host, uint16_t port, bool enabled);

/* Send a syslog message (RFC3164-ish).
 * facility: syslog facility constant (e.g. 16=local0), severity: 0..7 */
esp_err_t syslog_udp_send(int facility, int severity, const char *tag,
                          const char *message);

/* Return current enabled state */
bool syslog_udp_enabled(void);

#ifdef __cplusplus
}
#endif
