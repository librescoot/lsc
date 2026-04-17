# lsc - LibreScoot Control CLI

A command-line interface for controlling and monitoring LibreScoot electric scooters via Redis.

## Features

- **Vehicle Control**: Lock, unlock, hibernate, and force-lock vehicle states
- **LED Control**: Trigger LED cues and fade animations
- **Power Management**: Control power states (run, suspend, hibernate, reboot)
- **Service Management**: Start, stop, restart, enable, disable systemd services and view logs
- **OTA Updates**: View update status and install updates
- **GPS**: Monitor GPS status and track location
- **Battery Diagnostics**: View detailed battery information and health
- **Alarm System**: Arm, disarm, and trigger the vehicle alarm
- **Keycard Management**: Authorize/revoke keycards for vehicle access
- **Location Management**: Save and manage frequently visited locations
- **Hardware Control**: Manage dashboard, engine, handlebar, and seatbox
- **Settings**: Get and set vehicle configuration
- **Diagnostics**: Monitor faults, view firmware versions, and stream events
- **Metrics Recording**: Capture detailed system metrics over time for debugging
- **Log Extraction**: Extract service logs and Redis snapshots for analysis
- **JSON Output**: All commands support `--json` flag for automation

## Installation

Build for ARM (e.g., Raspberry Pi):
```bash
GOOS=linux GOARCH=arm GOARM=7 go build -o lsc .
```

Build for your local system:
```bash
go build -o lsc .
```

## Quick Start

```bash
# Show overall status
lsc status

# Lock the scooter
lsc lock

# Unlock the scooter
lsc unlock

# View all settings
lsc settings

# Get a specific setting
lsc get alarm.enabled

# Set a setting
lsc set alarm.duration 15

# View battery status
lsc battery

# Show active faults
lsc faults

# Watch GPS location
lsc gps watch

# View OTA update status
lsc ota status

# Manage keycards
lsc keycard list
lsc keycard add ABC123DEF456

# Manage saved locations
lsc locations list
lsc locations add 51.5074 -0.1278 "office"
```

## Command Reference

### Vehicle Control

- `lsc vehicle lock` - Lock the scooter
- `lsc vehicle unlock` - Unlock the scooter
- `lsc vehicle force-lock` - Force standby without waiting for locks
- `lsc vehicle hibernate` - Lock and request hibernation
- `lsc vehicle open` - Open seatbox

### LED Control

- `lsc led cue <index>` - Trigger LED cue by index
- `lsc led fade <channel> <index>` - Trigger LED fade animation

### Keycard Management

- `lsc keycard list` - List all authorized keycards
- `lsc keycard add <uid>` - Add a keycard UID
- `lsc keycard remove <uid>` - Remove a keycard UID
- `lsc keycard add-master <uid>...` - Add master keycard(s)
- `lsc keycard remove-master <uid>...` - Remove master keycard(s)
- `lsc keycard export <file>` - Export keycards to file
- `lsc keycard import <file>` - Import keycards from file

### Location Management

- `lsc locations list` - List all saved locations
- `lsc locations add <latitude> <longitude> <label>` - Add a saved location
- `lsc locations show <label>` - Show location details
- `lsc locations edit <label>` - Edit a saved location
- `lsc locations delete <label>` - Delete a saved location
- `lsc locations touch <label>` - Update last-used timestamp

### Power Management

- `lsc power status` - Show power manager status
- `lsc power run` - Set power state to run (normal operation)
- `lsc power suspend` - Set power state to suspend (low power)
- `lsc power hibernate` - Set power state to hibernate (power off)
- `lsc power reboot` - Reboot the system

### Service Management

- `lsc service list` (or `lsc svc list`) - List all services with status
- `lsc service start <service>` - Start a service
- `lsc service stop <service>` - Stop a service
- `lsc service restart <service>` - Restart a service
- `lsc service enable <service>` - Enable service to start on boot
- `lsc service disable <service>` - Disable service from starting on boot
- `lsc service status <service>` - Show detailed service status
- `lsc service logs <service>` - View recent service logs
  - `--follow` or `-f` - Follow logs in real-time
  - `--lines <n>` or `-n <n>` - Number of lines to show (default: 50)

**Service Name Shortcuts**: Use shorthand names like `vehicle`, `battery`, `ecu`, `alarm`, `modem`, `settings`, `bluetooth`, `pm`, etc. instead of full names like `librescoot-vehicle`.

**Examples:**
```bash
# List all services
lsc svc list

# Restart vehicle service (shorthand)
lsc svc restart vehicle

# Or use full name
lsc svc restart librescoot-vehicle

# Follow logs in real-time (shorthand)
lsc svc logs battery -f

# View last 100 log lines
lsc svc logs redis -n 100
```

### OTA Updates

- `lsc ota status` - View OTA update status and configuration
- `lsc ota install <file-or-url>` - Install update from local file or URL
- `lsc ota check` - Check for available updates

### GPS

- `lsc gps status` - Show GPS status and fix information
- `lsc gps watch` - Monitor GPS location in real-time
  - `--compact` - One-line format output

### Monitoring

- `lsc watch` - Watch Redis pub/sub channels for real-time events

### Diagnostics

- `lsc diag battery [id...]` - Show detailed battery information
- `lsc diag version` - Display firmware versions
- `lsc diag faults` - Show active faults
- `lsc diag events` - View fault event stream
  - `--follow` - Follow events like tail -f
  - `--since <duration>` - Show events since duration (e.g., 1h, 24h, 7d)
  - `--filter <regex>` - Filter events by regex pattern
- `lsc diag blinkers [off|left|right|both]` - Control blinkers
- `lsc diag horn [on|off]` - Control horn
- `lsc diag handlebar [lock|unlock]` - Control handlebar lock
- `lsc diag dashboard` - Control dashboard power
  - `on` / `off` - Power on/off
  - `status` - Show power status
  - `ping` - Check connectivity
  - `on-wait` - Power on and wait until ready
  - `off-wait` - Power off and wait until unreachable
  - `--force` - Force off even during updates
- `lsc diag engine` - Control engine power

### Metrics Recording

Record detailed metrics for debugging and analysis:

- `lsc monitor <subsystems...>` - Record metrics over time
  - Available subsystems: `gps`, `battery`, `vehicle`, `motor`, `power`, `modem`, `events`, `all`
  - `--duration <time>` - Recording duration (e.g., 1h, 5m, 24h)
  - `--interval <time>` - Polling interval (e.g., 1s, 5s, 100ms)
  - `--format <format>` - Output format (jsonl, csv)
  - `--output <dir>` - Output directory

**Examples:**
```bash
lsc monitor gps --duration 1h
lsc monitor battery vehicle --duration 10m --interval 5s
lsc monitor all --duration 30m --output /data/debug
```

### Log Extraction

Extract service logs and Redis snapshots:

- `lsc logs [services...]` - Extract logs for analysis
  - Available services: `vehicle`, `battery`, `ecu`, `modem`, `pm`, `update`, `settings`, `keycard`, `bluetooth`, `ums`, `radio-gaga`, `all`
  - `--since <time>` - Start time (e.g., 24h, 1d, "2025-10-25 10:00")
  - `--until <time>` - End time
  - `--priority <level>` - Log level (err, warning, info, debug)
  - `--output <dir>` - Output directory

**Examples:**
```bash
lsc logs                          # Extract all services (last 24h)
lsc logs vehicle --since 1h
lsc logs battery ecu --since 24h --output /data/debug
lsc logs all --priority err       # Show only errors
```

### Alarm

- `lsc alarm status` - Check alarm status
- `lsc alarm arm` - Enable the alarm
- `lsc alarm disarm` - Disable the alarm
- `lsc alarm trigger` - Manually trigger the alarm

### Settings

- `lsc settings` - List all settings
- `lsc settings get <key>` - Get a setting value
- `lsc settings set <key> <value>` - Set a setting value

### Shortcuts

Quick access to common commands:

**Vehicle:**
- `lsc lock` - Lock the scooter
- `lsc unlock` - Unlock the scooter
- `lsc open` - Open seatbox

**Settings:**
- `lsc get <key>` - Get setting value
- `lsc set <key> <value>` - Set setting value
- `lsc del <key>` - Delete setting key

**Diagnostics:**
- `lsc battery` or `lsc bat` - Show battery info
- `lsc version` or `lsc ver` - Show firmware versions
- `lsc faults` - Show active faults
- `lsc events` - View fault event stream
- `lsc dashboard` or `lsc dbc` or `lsc dash` - Control dashboard power
- `lsc engine` - Control engine power
- `lsc blinkers` or `lsc blink` - Control blinkers

All shortcuts support `--json` output and vehicle commands support `--no-block` flag.

### Development

**Dev only — hidden from `lsc --help`.** `lsc boot` pokes U-Boot env and raw block devices to swap the Mender A/B rootfs selection, bypassing `mender-update` and OTA. Worst case you end up in a boot-loop and have to stop at U-Boot over UART to fix `mender_boot_part` by hand — annoying, not destructive.

Same binary on MDB and DBC; it operates on the local slot.

- **pending** = `upgrade_available=1`. U-Boot increments `bootcount` each boot and rolls back to the other slot once `bootcount > bootlimit` (default `bootlimit=1` — one tentative boot, next reboot falls back).
- **committed** = `upgrade_available=0`. Boot is permanent.

Commands:

- `lsc boot status` - current slot, next-boot target, pending/committed state.
- `lsc boot set <a|b|other|current|N>` - persistently switch the next-boot slot.
- `lsc boot try-other [-y]` - one-shot boot into the *other* slot; any reboot without commit rolls back.
- `lsc boot armor [-y]` - tentative boot of the *current* slot with fallback to the other. Use before a risky change; reboot-loops auto-fall-back.
- `lsc boot commit [-y]` - clear a pending one-shot, making the current slot permanent.
- `lsc boot clone [--arm] [-y]` - `dd` the running rootfs onto the other slot (live/fuzzy snapshot; fsck cleans up on first mount). `--arm` also sets next-boot to the clone.

Recipes:

```bash
# Safety net: clean copy of A on B, keep hacking in A. Any reboot lands on B.
lsc boot clone --arm -y

# Risky kernel / initramfs change: armor, reboot, commit if it works.
lsc boot armor -y
reboot
# ... if A comes up clean:
lsc boot commit -y
# ... if A reboot-loops, U-Boot falls back to B automatically.

# Just poke at the other slot once; reboot restores sanity.
lsc boot try-other -y
reboot
```

`armor` only fires on *counted* reboots — a kernel that hangs early without a watchdog reboot won't fall back on its own.

## Global Flags

- `--json` - Output in JSON format for automation
- `--redis-addr <host:port>` - Redis server address (default: 192.168.7.1:6379)
- `--no-block` - Don't wait for state change confirmation (vehicle commands)

## JSON Output

All commands support JSON output for scripting and automation:

```bash
# Get status in JSON format
lsc status --json

# Lock and capture result
lsc lock --json

# Get setting value
lsc get alarm.enabled --json
```

Example JSON output:
```json
{
  "vehicle": {
    "state": "parked",
    "kickstand": "up",
    "brakes": {
      "left": "released",
      "right": "released"
    }
  },
  "motor": {
    "speed_kph": 0,
    "odometer_km": 1234.5,
    "temperature_c": 25
  },
  "batteries": [...]
}
```

## Common Settings

Settings can be viewed with `lsc settings list` and modified with `lsc set <key> <value>`:

**Alarm:**
- `alarm.enabled` - Enable/disable alarm (true/false)
- `alarm.honk` - Enable horn during alarm (true/false)
- `alarm.duration` - Alarm duration in seconds

**Updates:**
- `updates.mdb.method` - MDB update method (delta/full)
- `updates.mdb.channel` - MDB update channel (nightly/stable/etc)
- `updates.dbc.method` - Dashboard update method
- `updates.dbc.channel` - Dashboard update channel

**Network:**
- `cellular.apn` - Cellular APN string

**Dashboard Display:**
- `dashboard.theme` - UI theme (dark/light)
- `dashboard.mode` - Dashboard mode (navigation/etc)
- `dashboard.show-raw-speed` - Show raw speed (true/false)
- `dashboard.show-clock` - Clock visibility (always/riding/never)
- `dashboard.show-gps` - GPS indicator visibility
- `dashboard.show-bluetooth` - Bluetooth indicator visibility
- `dashboard.show-cloud` - Cloud indicator visibility
- `dashboard.show-internet` - Internet indicator visibility
- `dashboard.battery-display-mode` - Battery display mode (percentage/range)
- `dashboard.map.type` - Map source (offline/online) - offline uses local MBTiles, online uses CartoDB tiles
- `dashboard.map.render-mode` - Map render mode (raster)
- `dashboard.valhalla-url` - Routing service URL

**Battery:**
- `battery.ignore-seatbox` - Ignore seatbox in battery calculations

**Power:**
- `hibernation-timer` - Hibernation timer duration

## Bash Completion

Generate shell completion scripts:

```bash
# Bash
lsc completion bash > /etc/bash_completion.d/lsc

# Zsh
lsc completion zsh > "${fpath[1]}/_lsc"

# Fish
lsc completion fish > ~/.config/fish/completions/lsc.fish

# PowerShell
lsc completion powershell > lsc.ps1
```

## Architecture

lsc communicates with LibreScoot services via Redis:

- **Command Queues**: LPUSH to `scooter:*` lists for commands
- **State Hashes**: HGET/HSET on `vehicle`, `battery:*`, etc.
- **Pub/Sub**: Subscribe to state change notifications
- **Streams**: XREAD for event history

## Development

```bash
# Install dependencies
go mod download

# Build for ARM (target platform)
make build

# Build for your local platform
make build-native

# Or manually:
go build -o lsc .
GOOS=linux GOARCH=arm GOARM=7 go build -o lsc .

# Run tests
go test ./...
```

## License

Part of the LibreScoot open-source electric scooter platform.
