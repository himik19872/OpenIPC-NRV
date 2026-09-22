#pragma once

#include "esp_err.h"
#include "esp_netif.h"

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Initialize Ethernet (EMAC + RMII PHY), ESP-NETIF and start DHCP
 * (or fall back to static IP if configured).
 */
esp_err_t eth_netif_init(void);

/**
 * Return the netif instance for the Ethernet interface (may be NULL
 * before init).
 */
esp_netif_t *eth_netif_get(void);

#ifdef __cplusplus
}
#endif
