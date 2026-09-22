idf_component_register(
    SRCS "main.c"
    INCLUDE_DIRS "."
    REQUIRES nvs_flash esp_event esp_netif esp_eth esp_http_server
    PRIV_REQUIRES eth_netif wiegand door_hw config web_server event_log syslog_udp buzzer schedule card_db time_sync storage ota event_push fw_info
)
