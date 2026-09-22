#pragma once

#include "esp_err.h"
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Begin an OTA update (finds next OTA partition and erases it).
 * After this, feed data with ota_write() and finish with ota_commit().
 *
 * image_size is the total size of the firmware about to be written.
 * The size must be known here: the update layer uses it to reserve the
 * buffers needed for verification. Passing 0 makes it allocate for the
 * whole partition, and on a 3 MB partition that exhausts the heap —
 * the device then reboots without any diagnostics. */
esp_err_t ota_begin(size_t image_size);

/* Write a chunk of the new firmware image. */
esp_err_t ota_write(const uint8_t *data, size_t len);

/* Finalize the update and reboot to the new image. */
esp_err_t ota_commit(void);

/* Abort an update in progress (safe to call when none is running). */
void ota_abort(void);

/* Confirm the running firmware as working.
 *
 * When rollback is enabled, the bootloader boots a new image in a
 * "pending verify" state and waits for confirmation. Until this is called,
 * a reboot returns to the previous firmware. Call it only after the device
 * is fully operational (network and web server up): otherwise a firmware
 * that fails later would never be rolled back. */
void confirm_firmware(void);

/* Report the outcome of the previous OTA attempt, if any.
 *
 * The result is kept in NVS because a failed update usually reboots the
 * device, and the console log is lost. Called once at startup. */
void ota_read_journal(void);

/* Read the OTA journal without clearing it (for diagnostics over HTTP). */
void ota_peek_journal(char *stage, size_t stage_len,
                      char *detail, size_t detail_len);

/* Abort an in-progress update. */
void ota_abort(void);

#ifdef __cplusplus
}
#endif
