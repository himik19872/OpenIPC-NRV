idf_component_register(
    SRCS "syslog_udp.c"
    INCLUDE_DIRS "include"
    REQUIRES lwip esp_netif
)
