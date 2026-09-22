#include <string.h>
#include <stdio.h>
#include "syslog_udp.h"
#include "esp_log.h"
#include "lwip/sockets.h"
#include "lwip/netdb.h"
#include "esp_netif.h"

static const char *TAG = "syslog";

static char  s_host[48];
static uint16_t s_port = 514;
static bool  s_enabled = false;
static int   s_sock = -1;

esp_err_t syslog_udp_config(const char *host, uint16_t port, bool enabled)
{
    s_enabled = enabled;
    s_port = port;
    if (host) strncpy(s_host, host, sizeof(s_host) - 1);
    else s_host[0] = '\0';
    s_host[sizeof(s_host) - 1] = '\0';

    if (s_sock >= 0) {
        close(s_sock);
        s_sock = -1;
    }

    ESP_LOGI(TAG, "syslog %s -> %s:%u",
             s_enabled ? "enabled" : "disabled",
             s_host[0] ? s_host : "(none)", s_port);
    return ESP_OK;
}

bool syslog_udp_enabled(void)
{
    return s_enabled;
}

esp_err_t syslog_udp_send(int facility, int severity, const char *tag,
                          const char *message)
{
    if (!s_enabled || s_host[0] == '\0') return ESP_OK; /* silently skip */

    /* Build RFC3164 packet: "<PRI>TIMESTAMP HOST TAG: MESSAGE" */
    int pri = (facility * 8) + severity;
    char buf[512];
    int n = snprintf(buf, sizeof(buf), "<%d>%s %s: %s",
                     pri, "skud", tag, message);

    struct sockaddr_in dest;
    memset(&dest, 0, sizeof(dest));
    dest.sin_family = AF_INET;
    dest.sin_port = htons(s_port);

    struct hostent *he = gethostbyname(s_host);
    if (he && he->h_addr_list[0]) {
        memcpy(&dest.sin_addr, he->h_addr_list[0], sizeof(dest.sin_addr));
    } else {
        dest.sin_addr.s_addr = inet_addr(s_host);
        if (dest.sin_addr.s_addr == INADDR_NONE) {
            return ESP_ERR_INVALID_ARG;
        }
    }

    if (s_sock < 0) {
        s_sock = socket(AF_INET, SOCK_DGRAM, 0);
        if (s_sock < 0) return ESP_FAIL;
    }

    sendto(s_sock, buf, n, 0, (struct sockaddr *)&dest, sizeof(dest));
    return ESP_OK;
}
