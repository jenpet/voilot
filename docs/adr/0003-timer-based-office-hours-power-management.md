# Timer-based office hours power management

The production Mac wakes at 06:00 CEST via `pmset repeat` and relies on macOS native auto-sleep (15 min idle) after 23:00 instead of custom idle-detection logic or Wake-on-LAN. During office hours (06:00–23:00) auto-sleep is disabled. Active user sessions — HID input, TCP traffic, disk I/O from running agents — naturally prevent sleep after hours, so no hard cutoff occurs mid-activity.

## Considered Options

- **Custom idle script** — `check_idle_and_sleep()` querying backend health + HID idle time, calling `pmset sleepnow` explicitly. Works, but reimplements what macOS auto-sleep already does natively. More shell code to maintain.
- **`pmset repeat sleep` hard schedule** — simple but creates a hard cutoff that interrupts active user sessions. Contradicts the requirement.
- **macOS native auto-sleep with time-of-day toggle** (chosen) — `update.sh` sets `pmset -a sleep 15` at 23:00 and `pmset -a sleep 0` at 06:00 on transition edges only. Zero new scripts or plists. macOS handles user-activity awareness (HID, TCP, disk I/O) natively.

## Consequences

- Requires a sudoers entry (`%admin ALL=(root) NOPASSWD: /usr/bin/pmset`) so the user-level LaunchAgent can toggle the sleep setting.
- `VOILOT_IDLE_TIMEOUT` and `VOILOT_USER_IDLE_THRESHOLD` env vars are removed — no longer needed.
- A state file (`tmp/.power-mode`) prevents redundant `pmset` calls; the toggle only fires on day/night transitions.
- Long-running agent tasks (OpenCode doing constant I/O) prevent sleep naturally without requiring `caffeinate` assertions.
