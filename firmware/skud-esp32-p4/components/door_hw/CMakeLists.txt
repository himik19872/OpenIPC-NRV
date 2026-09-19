idf_component_register(
    SRCS "door_hw.c"
    INCLUDE_DIRS "include"
    REQUIRES esp_driver_gpio esp_timer
)
