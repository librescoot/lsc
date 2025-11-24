# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`lsc` (librescoot control) is a CLI tool that abstracts Redis-based interfaces used by LibreScoot ECU firmware services. It provides user-friendly commands to control and monitor scooters without requiring direct Redis knowledge.

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

# Build for AMD64 (Linux development/testing)
make build-amd64

# Build for native platform
make build-native

# Clean build artifacts
make clean

# Run tests
make test

# Run linter
make lint

# Run locally (requires Redis connection)
go run . status
./bin/lsc-native --help

# Install dependencies
make deps
# or
go mod tidy

# Deploy to Deep Blue and test
make deploy
make deploy-test
```

## Architecture

### Project Structure
```
main.go                      # Entry point, calls cmd/lsc.Execute()

cmd/lsc/
  root.go                    # Root command with Redis connection lifecycle
  status.go                  # Status command implementation
  vehicle.go                 # Vehicle state commands (lock, unlock, hibernate)
  alarm.go                   # Alarm control commands
  settings.go                # Settings management commands
  led.go                     # LED control commands
  watch.go                   # Watch Redis pub/sub channels
  shortcuts.go               # Shortcut commands that delegate to main commands
  completion.go              # Shell completion generation

  diag/                      # Diagnostic commands package
    diag.go                  # Parent diagnostic command
    battery.go               # Battery diagnostics
    version.go               # Version information
    faults.go                # Fault display
    events.go                # Event stream viewer
    blinkers.go, horn.go     # Hardware control
    handlebar.go             # Handlebar lock control
    dashboard.go             # Dashboard power control
    engine.go                # Engine power control

  gps/                       # GPS commands package
  power/                     # Power management commands package
  ota/                       # OTA update commands package
  locations/                 # Location management commands package
  keycard/                   # Keycard authorization management
  monitor/                   # Real-time monitoring package
  logs/                      # Log extraction package
  service/                   # systemd service management package

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

### Internal Packages

#### `internal/redis`
Wrapper around `github.com/redis/go-redis/v9` with:
- `Connect()` - Establishes connection with ping verification
- `Close()` - Graceful connection cleanup
- `HGet(key, field)`, `HSet(key, field, value)`, `HGetAll(key)` - Hash operations
- `LPush(key, values...)` - List push for command queues
- `SMembers(key)` - Get all set members (for faults)
- `Subscribe(ctx, channels...)` - Pub/sub subscriptions
- Automatic context management with 5-second timeouts

#### `internal/format`
Output formatting utilities:
- `format.go` - Table/text formatting helpers
- `colors.go` - Terminal color support
- `units.go` - Unit conversion (km/h, voltage, amperage, temperature)

#### `internal/confirm`
State change helpers:
- `WaitForFieldValueAfterCommand()` - Subscribe, execute function, wait for expected value
- `WaitForFieldValue()` - Subscribe and wait (for pre-sent commands)
- Handles race condition prevention automatically

### State Change Confirmation Pattern (CRITICAL)

**Race Condition Prevention**: When sending commands that trigger state changes, you MUST subscribe to the pub/sub channel BEFORE sending the command. Use the helper:

```go
import "librescoot/lsc/internal/confirm"

err := confirm.WaitForFieldValueAfterCommand(
    ctx,
    redisClient,
    "vehicle",        // channel to watch
    "state",          // field to check
    "parked",         // expected value
    30 * time.Second, // timeout
    func() error {
        // This runs AFTER subscription is established
        return redisClient.LPush(ctx, "scooter:state", "unlock")
    },
)
```

**Why This Matters**: Vehicle-service processes commands within milliseconds. If you send the command first, then subscribe, you'll miss the state change notification, causing timeouts even though the command succeeded.

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

1. **Simple command** (e.g., `status.go`): Create file with `var <name>Cmd = &cobra.Command{...}` and register in `init()`
2. **Command group** (e.g., `diag/`): Create package with parent command and subcommand files (e.g., `diag.go`, `battery.go`)
3. Define command with `&cobra.Command{Use: "...", Run: ...}`
4. Access Redis via `redisClient` package variable
5. For state changes, use `confirm.WaitForFieldValueAfterCommand()` to avoid race conditions
6. Always handle errors and print helpful messages

### Command Registration Pattern

```go
// In cmd/lsc/<file>.go
var exampleCmd = &cobra.Command{
    Use:   "example",
    Short: "Short description",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation
        return nil
    },
}

func init() {
    rootCmd.AddCommand(exampleCmd)
}
```

For subcommands in packages (e.g., `diag/`):
```go
// In diag/diag.go
var diagCmd = &cobra.Command{Use: "diag", ...}

func init() {
    rootCmd.AddCommand(diagCmd)
    diagCmd.AddCommand(batteryCmd)
    diagCmd.AddCommand(versionCmd)
}
```

## Development Workflow

### Testing Locally

Before deploying to hardware, test with local Redis:

```bash
# Build native binary
make build-native

# Run against local Redis
./bin/lsc-native status --redis-addr 127.0.0.1:6379

# Run tests
make test

# Run linter to catch issues early
make lint
```

### Testing on Hardware (Deep Blue)

When testing on the actual scooter:

```bash
# Build for ARM
make build

# Deploy with timestamp (helps track multiple versions)
make deploy

# Test via SSH
make deploy-test

# Or manual deployment:
scp bin/lsc deep-blue:/data/lsc-test
ssh deep-blue "/data/lsc-test status"

# Copy to /usr/local/bin for persistent testing
ssh deep-blue "cp /data/lsc-test /usr/local/bin/lsc"
```

### Verifying Redis Commands

Always cross-check lsc output with direct Redis commands on the scooter:

```bash
ssh deep-blue

# Test lsc
lsc status

# Compare with raw Redis
redis-cli HGETALL vehicle
redis-cli HGETALL engine-ecu
redis-cli HGETALL battery:0
redis-cli HGETALL settings
```

### Debugging Redis Communication

Monitor Redis commands in real-time using two SSH sessions:

```bash
# Session 1: Monitor all Redis commands
ssh deep-blue
redis-cli MONITOR

# Session 2: Run commands to debug
ssh deep-blue
lsc vehicle lock
```

**Important Notes**:
- Redis listens on 192.168.7.1 (MDB internal network) and localhost only
- Commands are processed within milliseconds, so race conditions in state verification are common
- Always use `confirm.WaitForFieldValueAfterCommand()` pattern for state changes

## Common Patterns and Gotchas

### JSON Output
Most commands support `--json` flag for automation:
```bash
lsc status --json
lsc lock --json
lsc settings get alarm.enabled --json
```

Check `format.JSONOutput()` to see how to add JSON support to new commands.

### Vehicle Command Flags
Vehicle commands support:
- `--no-block` - Return immediately without waiting for state confirmation
- Useful for scripting when you want to poll state separately

### Error Handling Best Practices
- Check Redis connection in `PersistentPreRunE` (done in `root.go`)
- Return descriptive errors from command functions
- Don't silently ignore missing keys - inform the user
- Validate input before sending commands to Redis

### Adding Subcommand Packages
When creating a new command group (e.g., `newfeature/`):
1. Create directory: `cmd/lsc/newfeature/`
2. Add parent command in `newfeature.go` with `var newfeatureCmd = &cobra.Command{...}`
3. Add subcommands in separate files (e.g., `newfeature/list.go`, `newfeature/add.go`)
4. Register subcommands in parent's `init()`: `newfeatureCmd.AddCommand(listCmd, addCmd)`
5. Register parent in `cmd/lsc/root.go` or create `cmd/lsc/newfeature.go` and register there

### Testing Tips
- Use `go test ./...` to run all tests
- Use `go test -v ./cmd/lsc` to test only command-level code
- Integration tests on hardware are more valuable than mocked tests since Redis behavior is critical
- Always verify output format matches README examples

## Related Documentation

- **[README.md](README.md)**: User-facing command reference and quick start guide
