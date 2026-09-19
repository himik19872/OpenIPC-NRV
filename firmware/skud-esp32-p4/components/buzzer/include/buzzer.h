#pragma once

#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Init buzzer on a GPIO (passive buzzer driven via LEDC/PWM).
 * gpio < 0 disables. */
esp_err_t buzzer_init(int gpio);

/* Deinit */
esp_err_t buzzer_deinit(void);

/* Emit a beep pattern. n = number of beeps, freq_hz = tone frequency,
 * dur_ms = each beep duration, gap_ms = gap between beeps. */
esp_err_t buzzer_beep(int n, int freq_hz, int dur_ms, int gap_ms);

/* Continuous tone on/off */
esp_err_t buzzer_tone(int freq_hz);
esp_err_t buzzer_stop(void);

#ifdef __cplusplus
}
#endif
