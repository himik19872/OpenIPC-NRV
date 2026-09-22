idf_component_register(
    SRCS "event_push.c"
    INCLUDE_DIRS "include"
    REQUIRES esp_http_client cjson
)
