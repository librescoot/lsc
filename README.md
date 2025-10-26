# lsc - LibreScoot Control CLI

A command-line interface for controlling and monitoring LibreScoot electric scooters via Redis.

## Features

- **Vehicle Control**: Lock, unlock, hibernate, and force-lock vehicle states
- **LED Control**: Trigger LED cues and fade animations
- **Power Management**: Control power states (run, suspend, hibernate, reboot)
- **Service Management**: Start, stop, restart, enable, disable systemd services and view logs
- **OTA Updates**: View update status and install updates from files or URLs
- **GPS**: Monitor GPS status and track location
- **Battery Diagnostics**: View detailed battery information and health
- **Alarm System**: Arm, disarm, and trigger the vehicle alarm
- **Hardware Control**: Manage dashboard, engine, handlebar, and seatbox
- **Settings**: Get and set vehicle configuration
- **Diagnostics**: Monitor faults, view firmware versions, and stream events
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
lsc set scooter.mode sport

# View battery status
lsc bat

# Show active faults
lsc faults

# Watch GPS location
lsc gps watch

# View OTA update status
lsc ota status
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

- `lsc ota status` - View OTA update status
- `lsc ota install <file-or-url>` - Install update from local file or URL

### GPS

- `lsc gps status` - Show GPS status
- `lsc gps watch` - Monitor GPS location in real-time

### Diagnostics

- `lsc diag battery [id...]` - Show battery information
- `lsc diag version` - Display firmware versions
- `lsc diag faults` - Show active faults
- `lsc diag events` - View fault event stream
  - `--follow` - Follow events like tail -f
  - `--since <duration>` - Show events since duration (e.g., 1h, 24h, 7d)
  - `--filter <regex>` - Filter events by regex pattern
- `lsc diag blinkers [off|left|right|both]` - Control blinkers
- `lsc diag horn [on|off]` - Control horn
- `lsc diag handlebar [lock|unlock]` - Control handlebar lock

### Alarm

- `lsc alarm status` - Check alarm status
- `lsc alarm arm` - Enable the alarm
- `lsc alarm disarm` - Disable the alarm
- `lsc alarm trigger` - Manually trigger the alarm

### Settings

- `lsc settings` - List all settings
- `lsc settings get <key>` - Get a setting value
- `lsc settings set <key> <value>` - Set a setting value

### Hardware

- `lsc diag hardware <command>` - Send hardware commands
  - `dashboard:on` / `dashboard:off` - Control dashboard power
  - `engine:on` / `engine:off` - Control engine power

### Shortcuts

Quick access to common commands:

- `lsc lock` - Lock the scooter
- `lsc unlock` - Unlock the scooter
- `lsc open` - Open seatbox
- `lsc get <key>` - Get setting
- `lsc set <key> <value>` - Set setting
- `lsc dbc [on|off]` - Control dashboard power
- `lsc engine [on|off]` - Control engine power
- `lsc bat [id...]` - Show battery info
- `lsc ver` - Show firmware versions
- `lsc faults` - Show active faults
- `lsc events` - View fault events

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

Settings can be viewed with `lsc settings` and modified with `lsc set`:

- `alarm.enabled` - Enable/disable alarm (true/false)
- `alarm.honk` - Enable horn during alarm (true/false)
- `alarm.duration` - Alarm duration in seconds
- `scooter.speed_limit` - Speed limit in km/h
- `scooter.mode` - Driving mode (eco/normal/sport)
- `cellular.apn` - Cellular APN string

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
