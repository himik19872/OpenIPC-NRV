#pragma once

#include "config.h"
#include <time.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Check whether access is allowed right now according to schedules.
 *
 * Logic:
 *   - If floating schedule is enabled: allowed only within [start, end].
 *   - Weekly: for today's weekday, if enabled, must be within [on, off]
 *     (minutes-from-midnight). If no day enabled, weekly has no effect.
 *   - If no schedules are configured at all (no weekly day enabled and
 *     no floating), access is allowed (no restriction).
 *
 * @param cfg  system config with schedules
 * @param now  current time (can pass NULL to use time(NULL))
 * @return true if allowed, false if blocked by a schedule
 */
bool schedule_allowed(const sys_config_t *cfg, const time_t *now);

#ifdef __cplusplus
}
#endif
