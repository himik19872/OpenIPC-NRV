idf_component_register(
    SRCS "web_server.c"
    INCLUDE_DIRS "include"
    EMBED_FILES "www/index.html"
    REQUIRES esp_http_server config door_hw event_log syslog_udp cjson card_db time_sync ota
)
