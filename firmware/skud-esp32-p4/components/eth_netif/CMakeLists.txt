idf_component_register(
    SRCS "eth_netif.c"
    INCLUDE_DIRS "include"
    REQUIRES esp_eth esp_netif esp_event nvs_flash esp_driver_gpio
    PRIV_REQUIRES ip101
)
