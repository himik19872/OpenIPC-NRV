idf_component_register(
    SRCS "ota.c"
    INCLUDE_DIRS "include"
    REQUIRES app_update esp_partition esp_app_format esp_system nvs_flash
)
