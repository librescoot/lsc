# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`lsc` (librescoot control) is a CLI tool that abstracts Redis-based interfaces used by LibreScoot ECU firmware services. It provides user-friendly commands to control and monitor scooters without requiring direct Redis knowledge.

**For comprehensive design information, implementation details, and Redis interface mappings, see [DESIGN.md](DESIGN.md).**

### LibreScoot System Context

LibreScoot runs on unu Scooter Pro hardware with a distributed architecture:
- **MDB (Middle Driver Board)**: Central control at 192.168.7.1, runs Redis and core services
- **DBC (Dashboard Computer)**: i.MX6 processor at 192.168.7.2, runs scootui Flutter app
- **ECU**: BOSCH/Lingbo motor controller, CAN bus communication
- **Batteries**: Dual battery system with NFC communication
- **Sensors**: BMX055 (9-axis), GPS via cellular modem
- **Hardware**: GPIO inputs (brakes, kickstand, buttons), PWM LED outputs, locks/solenoids

## Development Commands

```bash
# Build for ARM (default target - Raspberry Pi on scooter)
make build
# or
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/lsc .

# Build for local development
make build-native
go build -o lsc .

# Run locally (requires Redis connection)
go run . status --redis-addr 127.0.0.1:6379
./lsc status
./lsc --help

# Dependencies
go mod tidy
go mod download

# Testing
go test ./...
go test ./internal/redis

# Deploy to scooter
make build
scp bin/lsc deep-blue:/usr/bin/lsc

# Deploy and test
make build && scp bin/lsc deep-blue:/usr/bin/lsc && ssh deep-blue "lsc status"
```

## Architecture

### Project Structure
```
main.go                      # Entry point, calls cmd/lsc.Execute()
cmd/lsc/
  root.go                    # Root command with Redis connection lifecycle
  status.go                  # Status command implementation
  vehicle.go                 # Vehicle state commands (lock, unlock, hibernate, open)
  shortcuts.go               # Shortcut commands that delegate to main commands
  alarm.go                   # Alarm control commands
  settings.go                # Settings management commands
  led.go                     # LED control commands
  watch.go                   # Watch Redis pub/sub channels
  completion.go              # Shell completion generation
  diag/                      # Diagnostic commands package
    diag.go                  # Parent diagnostic command
    battery.go               # Battery diagnostics
    version.go               # Version information
    faults.go                # Fault display
    events.go                # Event stream viewer
    blinkers.go, horn.go     # Hardware control
    handlebar.go             # Handlebar lock control
  gps/                       # GPS commands package
    gps.go, status.go, watch.go
  power/                     # Power management commands package
    power.go, status.go, run.go, suspend.go, hibernate.go, reboot.go
  ota/                       # OTA update commands package
    ota.go, status.go, install.go
  locations/                 # Location management commands package
    locations.go, list.go, add.go, edit.go, delete.go, show.go, touch.go
  monitor/                   # Real-time monitoring package
    monitor.go, recorder.go, writer.go, tarball.go
  logs/                      # Log extraction package
    logs.go
internal/
  redis/
    client.go                # Redis client wrapper with common operations
  format/
    format.go                # Output formatting utilities
    colors.go                # Color/styling helpers
    units.go                 # Unit conversion (km/h, voltage, etc.)
  confirm/
    confirm.go               # Helper for waiting on Redis state changes
```

### Cobra Command Pattern
- **root.go** manages global Redis connection via `PersistentPreRunE` (connect) and `PersistentPostRun` (cleanup)
- All commands access Redis through the `redisClient` package variable
- Global flags (e.g., `--redis-addr`) defined in `rootCmd.PersistentFlags()`
- New commands register themselves via `rootCmd.AddCommand()` in their `init()` functions

### Shortcut Commands Pattern
Shortcut commands (in `shortcuts.go`) delegate to the real command implementations to avoid code duplication:

```go
// Define shortcut that delegates to real command
var lockCmd = &cobra.Command{
    Use:   "lock",
    Short: "Lock the scooter (shortcut for 'vehicle lock')",
    Run:   vehicleLockCmd.Run,  // Delegates to vehicle.go
}
```

This pattern:
- Eliminates duplicate code between shortcuts and full commands
- Ensures consistent behavior (e.g., `lsc lock` = `lsc vehicle lock`)
- Makes maintenance easier - fix bugs in one place
- Applies to: lock, unlock, open, get, set, and diagnostic shortcuts

### Redis Client Wrapper
The `internal/redis.Client` wraps `github.com/redis/go-redis/v9` with:
- Context management (background context with 5s timeout on connect)
- Common operations: `HGet`, `HSet`, `HGetAll`, `LPush`, `SMembers`, `Subscribe()`
- Connection lifecycle: `Connect()` (with ping), `Close()`
- Pub/sub support: `Subscribe()`, `Publish()` for monitoring state changes

### State Change Confirmation Pattern (CRITICAL)

**Race Condition Prevention**: When sending commands that trigger state changes, you MUST subscribe to the pub/sub channel BEFORE sending the command:

```go
// CORRECT: Subscribe BEFORE sending command
pubsub := redisClient.Subscribe(ctx, "vehicle")
ch := pubsub.Channel()
time.Sleep(100 * time.Millisecond)  // Allow subscription to establish
redisClient.LPush("scooter:state", "unlock")

// Wait for notification
for {
    select {
    case msg := <-ch:
        if msg.Payload == "state" {
            state, _ := redisClient.HGet("vehicle", "state")
            // Check if desired state reached
        }
    case <-timeout:
        // Handle timeout
    }
}
```

**Helper Functions**:
- `confirm.WaitForFieldValueAfterCommand()` - Subscribes, executes command function, then waits
- `confirm.WaitForFieldValue()` - Assumes command already sent, subscribes and waits

**Why This Matters**: Vehicle-service processes commands within milliseconds. If you send the command first, then subscribe, you'll miss the state change notification. This causes timeouts even though the command succeeded.

## Redis Integration Patterns

Services communicate via Redis:
- **Hashes**: Store state (e.g., `engine-ecu`, `battery:0`, `vehicle`, `settings`)
- **Lists**: Command queues (e.g., `scooter:state`, `scooter:horn`, `scooter:seatbox`)
- **Sets**: Fault tracking (e.g., `vehicle:fault`, `battery:<id>:faults`)
- **Streams**: Event logs (e.g., `events:faults`)

### Key Redis Keys by Service
- **ecu-service**: `engine-ecu` hash (rpm, speed, odometer, motor:voltage, motor:current, temperature)
- **battery-service**: `battery:<id>` hashes (state, soc, voltage, current, temperature-state)
- **vehicle-service**: `vehicle` hash (state, blinker:switch, seatbox:lock, kickstand), command lists (`scooter:state`, `scooter:seatbox`, `scooter:horn`)
- **alarm-service**: `settings` hash (alarm.enabled field), `alarm` hash (alarm-active), `scooter:alarm` list for commands

## CLI Interface

The CLI is designed with a hierarchical structure using subcommands for intuitive usage:

### Status Command
- **`lsc status`**: Displays a dashboard of key metrics (Speed, Odometer, Motor Temp, Battery SoC, Vehicle State)

### Vehicle Commands
- **`lsc vehicle lock`**: Locks the scooter
- **`lsc vehicle unlock`**: Unlocks the scooter
- **`lsc vehicle hibernate`**: Puts the scooter into hibernate mode
- **`lsc vehicle seatbox [open|close]`**: Controls the seatbox lock

### Settings Commands
Generic access to configuration stored in Redis `settings` hash:
- **`lsc settings list`**: Lists all global settings
- **`lsc settings get <key>`**: Retrieves a specific setting
- **`lsc settings set <key> <value>`**: Sets a specific setting

### Alarm Commands
User-friendly alarm controls:
- **`lsc alarm status`**: Checks alarm status
- **`lsc alarm arm`**: Enables the alarm
- **`lsc alarm disarm`**: Disables the alarm
- **`lsc alarm trigger`**: Manually triggers the alarm

### Diagnostic Commands
Diagnostic and detailed information:
- **`lsc diag faults`**: Shows all active faults
- **`lsc diag battery [<id>...]`**: Shows detailed battery status (optionally specify battery IDs)
- **`lsc diag version`**: Displays firmware versions
- **`lsc diag blinkers [off|left|right|both]`**: Controls blinkers
- **`lsc diag horn [on|off]`**: Controls the horn
- **`lsc diag led-cue <index>`**: Controls LED cues
- **`lsc diag led-fade <channel> <index>`**: Controls LED fades

## Adding New Commands

1. Create new file in `cmd/lsc/` (e.g., `vehicle.go`)
2. Define command with `&cobra.Command{Use: "...", Run: ...}`
3. Access Redis via `redisClient` package variable
4. Register in `init()`: `rootCmd.AddCommand(vehicleCmd)`
5. For commands with subcommands, set up parent command and add children

## Development Workflow

### Testing on Hardware

When testing on the actual scooter (Deep Blue):

```bash
# Build for ARM
make build

# Copy to target
scp bin/lsc deep-blue:/data/lsc-test

# Run directly
ssh deep-blue "/data/lsc-test status"

# Or copy to /usr/local/bin for permanent installation
ssh deep-blue "cp /data/lsc-test /usr/local/bin/lsc"
```

### Verifying Redis Commands

Always cross-check lsc output with direct Redis commands (on the scooter via SSH):
```bash
# SSH to scooter
ssh deep-blue

# Test lsc (Redis on localhost by default)
./lsc-test status

# Compare with raw Redis data
redis-cli HGETALL vehicle
redis-cli HGETALL engine-ecu
redis-cli HGETALL battery:0
```

### Debugging Redis Communication

Use Redis MONITOR to watch all commands:
```bash
# SSH to scooter, run monitor in one session
ssh deep-blue
redis-cli MONITOR

# In another SSH session to scooter
ssh deep-blue
./lsc-test vehicle lock
```

**Note**: Redis listens on 192.168.7.1 (internal MDB network) and localhost only, not on the external 10.7.0.x interface.

## Related Documentation

- **[DESIGN.md](DESIGN.md)**: Comprehensive design document with all Redis interfaces
- **[../tech-reference/](../tech-reference/)**: Complete hardware and software documentation
- **[../tech-reference/redis/README.md](../tech-reference/redis/README.md)**: Redis key structure reference
- **[../tech-reference/services/README.md](../tech-reference/services/)**: Individual service documentation
