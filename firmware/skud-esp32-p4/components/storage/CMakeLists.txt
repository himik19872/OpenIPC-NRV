idf_component_register(
    SRCS "storage.c"
    INCLUDE_DIRS "include"
    REQUIRES esp_partition littlefs vfs
)
