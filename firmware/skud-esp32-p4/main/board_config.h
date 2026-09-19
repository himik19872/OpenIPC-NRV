/*
 * board_config.h
 * Hardware pin and board definitions for Waveshare ESP32-P4-Module-Dev-Kit
 * (ESP32-P4-NR16 module, 16MB PSRAM).
 *
 * NOTE: Ethernet RMII/MDC-MDIO pins below are PLACEHOLDERS and MUST be
 * verified against the actual PHY chip (DP83848 / RTL8201 / LAN8720 / IP101)
 * and the Dev-Kit schematic before use.
 */
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

/* ================= Ethernet (RMII, IP101 PHY) ================= */
/* Waveshare ESP32-P4 family default wiring (from ESP32-P4-Platform examples): */
#define ETH_PHY_ADDR            1        /* IP101 SMI address */
#define ETH_MDC_GPIO            31
#define ETH_MDIO_GPIO           52
#define ETH_PHY_RST_GPIO        51       /* PHY reset */

/* RMII pins: internal EMAC handles these; leave NC unless custom wiring */
#define ETH_RMII_CLK_GPIO       (-1)
#define ETH_RMII_TX_EN_GPIO     (-1)
#define ETH_RMII_TXD0_GPIO      (-1)
#define ETH_RMII_TXD1_GPIO      (-1)
#define ETH_RMII_CRS_DV_GPIO    (-1)
#define ETH_RMII_RXD0_GPIO      (-1)
#define ETH_RMII_RXD1_GPIO      (-1)

/* ================= Wiegand 26 reader ================= */
#define WIEGAND_D0_GPIO         4
#define WIEGAND_D1_GPIO         5

/* ================= Door hardware ================= */
#define DOOR_LOCK_GPIO          8       /* relay / transistor -> lock */
#define DOOR_RTE_GPIO           (-1)    /* request-to-exit button (set later) */
#define DOOR_SENSOR_GPIO        (-1)    /* reed switch (set later) */
#define DOOR_BUZZER_GPIO        (-1)    /* optional buzzer */
#define DOOR_LED_GPIO           (-1)    /* optional status LED */

#ifdef __cplusplus
}
#endif
