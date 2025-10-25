# lsc Design Document

This document provides detailed design information for the `lsc` (librescoot control) command-line tool.

## Executive Summary

`lsc` is a user-friendly CLI that abstracts Redis-based communication used by LibreScoot services. It provides intuitive commands for controlling and monitoring electric scooter operations without requiring direct knowledge of Redis keys, hashes, lists, and pub/sub channels.

## System Context

### LibreScoot Architecture

LibreScoot uses a distributed service architecture where all components communicate via Redis:

**Core Services:**
- **vehicle-service**: State machine coordinator, hardware I/O (GPIO, PWM)
- **battery-service**: NFC-based battery monitoring via PN7150 readers
- **ecu-service**: Motor controller interface via CAN bus
- **alarm-service**: Motion-based alarm with BMX055 integration
- **bmx-service**: 9-axis sensor (accelerometer, gyroscope, magnetometer)
- **modem-service**: Cellular connectivity and GPS via ModemManager
- **pm-service**: Power management (suspend/hibernate)
- **settings-service**: TOML configuration file synchronization
- **update-service**: OTA firmware updates
- **scootui**: Flutter-based dashboard UI

**Redis Communication Patterns:**
1. **Hashes**: State storage (HSET/HGET/HGETALL)
2. **Lists**: Command queues (LPUSH by clients, BRPOP by services)
3. **Pub/Sub**: Event notifications (PUBLISH after HSET)
4. **Streams**: Event logs (XADD for fault events)

### Redis Data Model

#### State Hashes

| Hash | Owner Service | Key Fields | Access |
|------|--------------|------------|--------|
| `vehicle` | vehicle-service | state, brake:left/right, blinker:switch/state, kickstand, seatbox:lock, handlebar:position/lock-sensor, horn:button | Read |
| `engine-ecu` | ecu-service | rpm, speed, odometer, motor:voltage, motor:current, temperature, throttle, kers | Read |
| `battery:0`, `battery:1` | battery-service | present, state, voltage, current, charge, temperature:0-3, temperature-state, cycle-count, state-of-health, serial-number | Read |
| `alarm` | alarm-service | status (disabled, disarmed, delay-armed, armed, level-1-triggered, level-2-triggered) | Read |
| `bmx` | bmx-service | initialized, polling-rate-hz, streaming, interrupt, pin, threshold, duration, sensitivity, heading | Read/Write |
| `settings` | settings-service | alarm.enabled, alarm.honk, alarm.duration, scooter.speed_limit, scooter.mode, cellular.apn, etc. | Read/Write |
| `dashboard` | scootui | ready, mode, serial-number | Read |
| `gps` | modem-service | latitude, longitude, altitude, timestamp, speed, course | Read |
| `internet` | modem-service | modem-state, status, unu-cloud, ip-address, access-tech, signal-quality, sim-imei, sim-iccid | Read |
| `power-manager` | pm-service | state, wakeup-source, nrf-reset-count, nrf-reset-reason, hibernate-level | Read |
| `ota` | update-service | system, migration, status, fresh-update | Read |
| `system` | (various) | mdb-version, environment, nrf-fw-version, dbc-version | Read |

#### Command Lists (LPUSH)

| List | Consumer Service | Commands | Description |
|------|-----------------|----------|-------------|
| `scooter:state` | vehicle-service | lock, unlock, lock-hibernate, force-lock | Vehicle state control |
| `scooter:seatbox` | vehicle-service | open | Seatbox lock control |
| `scooter:horn` | vehicle-service | on, off | Horn control |
| `scooter:blinker` | vehicle-service | left, right, both, off | Blinker control |
| `scooter:led:cue` | vehicle-service | <index> | LED cue playback |
| `scooter:led:fade` | vehicle-service | <channel> <index> | LED fade setting |
| `scooter:hardware` | vehicle-service | dashboard:on, dashboard:off, engine:on, engine:off | Direct power control |
| `scooter:update` | vehicle-service | (OTA commands) | Update coordination |
| `scooter:alarm` | alarm-service | start:<duration>, stop, enable, disable | Alarm control |
| `scooter:bmx` | bmx-service | streaming:enable/disable, sensitivity:low/medium/high, pin:int1/int2/none, interrupt:enable/disable, reset, polling:<rate> | Sensor control |
| `scooter:power` | pm-service | run, suspend, hibernate, hibernate-manual, hibernate-timer, reboot | Power management |

#### Pub/Sub Channels

Channels follow the pattern: `<hash-name>` (publishes `<field>` as message) or `<hash-name>:<field>` (publishes field value).

Key channels for monitoring:
- `vehicle` - Vehicle state changes
- `engine-ecu throttle` - Throttle events
- `engine-ecu odometer` - Odometer updates
- `battery:0`, `battery:1` - Battery state changes
- `alarm` - Alarm status changes
- `bmx:sensors` - Sensor readings (10Hz)
- `bmx:magnetometer` - Magnetometer readings (5Hz)
- `bmx:interrupt` - Motion detection events

#### Event Streams

- `events:faults` - System fault events (XADD/XREAD)

## Command Design

### Status Command (`lsc status`)

**Purpose**: Display dashboard of key scooter metrics

**Implementation**:
```go
// Read multiple hashes in parallel
ecuData := HGETALL engine-ecu
batteryData := HGETALL battery:0
vehicleState := HGET vehicle state

// Format and display:
// Vehicle: <state>
// Speed: <speed> km/h | RPM: <rpm>
// Odometer: <odometer/1000> km
// Motor: <voltage> V @ <current> A | Temp: <temperature>°C
// Battery 0: <charge>% | <voltage> V @ <current> A
```

**Output Format**:
```
=== Vehicle Status ===
State: ready-to-drive
Speed: 0 km/h | RPM: 0
Odometer: 632.9 km
Motor: 52.1 V @ 0.0 A | Temp: 16°C

=== Battery Status ===
Battery 0: 85% | 54.2 V @ 0.0 A | Temp: 22°C
Battery 1: Not Present
```

### Vehicle Commands (`lsc vehicle`)

#### lock
- Command: `LPUSH scooter:state lock`
- Effect: Transitions to shutting-down → stand-by
- Feedback: Watch `vehicle` channel for state change

#### unlock
- Command: `LPUSH scooter:state unlock`
- Effect: Transitions to parked or ready-to-drive (based on kickstand)
- Feedback: Watch `vehicle` channel for state change

#### hibernate
- Command: `LPUSH scooter:state lock-hibernate`
- Effect: Lock and request hibernation
- Feedback: Watch `vehicle` and `power-manager` channels

#### seatbox open|close
- Command: `LPUSH scooter:seatbox open`
- Note: No close command (spring-loaded mechanism)
- Feedback: Watch `HGET vehicle seatbox:lock`

### Settings Commands (`lsc settings`)

Settings are stored in the `settings` hash with dot-notation keys (e.g., `alarm.enabled`, `scooter.speed_limit`).

#### list
```go
settings := HGETALL settings
// Display as table or key=value pairs
```

#### get \<key\>
```go
value := HGET settings <key>
// Display value or "not set"
```

#### set \<key\> \<value\>
```go
HSET settings <key> <value>
PUBLISH settings <key>
// Confirm with "Setting <key> = <value>"
```

**Common Settings**:
- `alarm.enabled`: "true"/"false"
- `alarm.honk`: "true"/"false"
- `alarm.duration`: integer (seconds)
- `scooter.speed_limit`: integer (km/h)
- `scooter.mode`: "eco"/"normal"/"sport"
- `cellular.apn`: string

### Alarm Commands (`lsc alarm`)

#### status
```go
status := HGET alarm status
enabled := HGET settings alarm.enabled
// Display: "Alarm: <status> (enabled: <enabled>)"
```

#### arm
```go
HSET settings alarm.enabled true
PUBLISH settings alarm.enabled
// Effect: FSM transitions to armed if vehicle in stand-by
```

#### disarm
```go
HSET settings alarm.enabled false
PUBLISH settings alarm.enabled
// Effect: FSM transitions to disarmed
```

#### trigger
```go
duration := HGET settings alarm.duration
LPUSH scooter:alarm start:<duration>
// Manually trigger alarm for specified duration
```

### Diagnostic Commands (`lsc diag`)

#### faults
```go
// Check all fault sources
vehicleFaults := SMEMBERS vehicle:fault
battery0Faults := SMEMBERS battery:0:faults
battery1Faults := SMEMBERS battery:1:faults

// Display active faults:
// === Active Faults ===
// [vehicle] fault code 123: Description here
// [battery:0] fault code 5: Over temperature
//
// No active faults found.
```

**Options**:
- `--history`: Show fault event stream (see below)
- `--verbose`: Include fault code details

#### events

View the fault event stream:
```go
// Read from events:faults stream
// XREAD [COUNT count] STREAMS events:faults <start-id>
// Default: last 50 events, or all events since timestamp

// Display format:
// [2024-10-25 14:32:15.234] [battery:0] Fault 5: Over temperature
// [2024-10-25 14:30:42.891] [vehicle] Fault 123: Description
// [2024-10-25 14:28:01.456] [battery:1] Fault 12: Cell imbalance

// Each entry has:
// - timestamp (from stream ID or event data)
// - group (service/component)
// - code (numerical fault code)
// - description (human-readable)
```

**Implementation**:
```bash
lsc diag events [--since 1h] [--follow] [--filter battery]

# Options:
# --since <duration>  Show events since duration ago (1h, 24h, 7d)
# --count <n>         Show last N events (default: 50)
# --follow            Tail the stream (like tail -f)
# --filter <regex>    Filter by group/code/description
# --json              JSON output for scripting
```

**Follow Mode**:
```go
// Use XREAD with BLOCK for tail -f behavior
for {
    results := XREAD COUNT 10 BLOCK 1000 STREAMS events:faults <last-id>
    // Display new events as they arrive
    // Update last-id for next iteration
}
```

#### battery [id...]
```go
// If no IDs specified, show all batteries (0, 1)
for id in ids:
  data := HGETALL battery:<id>
  // Display comprehensive battery info:
  // - Present, State, Charge
  // - Voltage, Current
  // - Temperature sensors (0-3)
  // - Cycle count, State of health
  // - Serial number, Manufacturing date, FW version
  // - Active faults
```

#### version
```go
// Collect versions from all system components
system := HGETALL system
ecuFwVersion := HGET engine-ecu fw-version
battery0FwVersion := HGET battery:0 fw-version
battery1FwVersion := HGET battery:1 fw-version
otaSystem := HGET ota system

// Display formatted version inventory:
// === System Versions ===
// MDB:      v1.15.0+430538
// DBC:      v1.15.0+430553
// nRF:      v1.12.0
// Environment: production
//
// === Component Versions ===
// ECU:      0445400C
// Battery 0: 1.2.3 (S/N: ABC123)
// Battery 1: Not Present
//
// === OTA ===
// System:   foundries
// Status:   initializing
```

**Output Options**:
- `--json`: Machine-readable JSON output for scripting
- `--short`: Compact one-line format
- `--check`: Compare against known good versions (from config file)

#### blinkers [off|left|right|both]
```go
LPUSH scooter:blinker <command>
// Watch HGET vehicle blinker:switch for confirmation
```

#### horn [on|off]
```go
LPUSH scooter:horn <command>
// Watch HGET vehicle horn:button for confirmation
```

#### led-cue \<index\>
```go
LPUSH scooter:led:cue <index>
// Trigger LED cue sequence
// Common cues: 0 (all off), 1-2 (standby→parked), 3 (parked→drive),
//              4-5 (brake), 7-8 (parked→standby), 9-12 (blinkers)
```

#### led-fade \<channel\> \<index\>
```go
LPUSH scooter:led:fade <channel> <index>
// Set LED fade for specific channel (0-7)
// Fades loaded from /usr/share/led-curves/
```

## Advanced Features

### Monitor Dashboard (`lsc monitor`)

Real-time terminal dashboard with live updates from Redis pub/sub channels.

**Purpose**: Single-view situational awareness during testing, debugging, or operation.

**Implementation**:
```go
// Use a TUI library like bubbletea or termui
// Subscribe to multiple channels in parallel goroutines
SUBSCRIBE vehicle
SUBSCRIBE engine-ecu throttle
SUBSCRIBE alarm
SUBSCRIBE bmx:sensors
SUBSCRIBE gps

// Layout:
// +----------------------------------+
// | Vehicle: ready-to-drive  Alarm: armed
// | Speed: 0 km/h  RPM: 0    Battery 0: 85%
// | GPS: 52.5200, 13.4050    Battery 1: --
// +----------------------------------+
// | Recent Events:
// | 14:32:15 [vehicle] state changed to ready-to-drive
// | 14:32:10 [alarm] status changed to armed
// | 14:32:05 [battery:0] charge 85%
// +----------------------------------+
// | Press q to quit, r to refresh
// +----------------------------------+

// Refresh rate: 1s for polled data, instant for pub/sub events
```

**Display Sections**:
1. **Status Bar**: Vehicle state, alarm status, battery levels
2. **Metrics**: Speed, RPM, throttle, motor voltage/current/temp
3. **Location**: GPS lat/lon, altitude, speed, heading (from bmx)
4. **Sensors**: BMX accelerometer, gyro readings (if streaming enabled)
5. **Event Log**: Scrolling list of recent pub/sub messages (last 20)

**Features**:
- Color coding (green=good, yellow=warning, red=fault)
- Auto-scroll event log
- Pause/resume updates (spacebar)
- Export snapshot (save current state to file)
- Configurable refresh rate

### Watch Mode (`lsc watch <channel>...`)

Monitor one or more Redis pub/sub channels in real-time.

**Purpose**: Observe specific Redis channels for debugging or monitoring.

**Implementation**:
```go
// Multiple channel subscription
channels := []string{"vehicle", "alarm", "bmx:interrupt"}
pubsub := redisClient.Subscribe(ctx, channels...)

// Display format:
// [14:32:15.234] [vehicle] state
// [14:32:16.105] [alarm] status
// [14:32:17.891] [bmx:interrupt] {"timestamp": 1696089234567, ...}

// Options:
// --format=json    Output as JSON lines for scripting
// --format=raw     Just the message payload, no timestamp/channel
// --timestamps     Show millisecond precision timestamps
// --filter=<regex> Filter messages by content
```

**Useful Channels**:
- `vehicle` - Vehicle state changes (publishes field names)
- `alarm` - Alarm status changes
- `bmx:sensors` - Continuous sensor data (10Hz when enabled)
- `bmx:magnetometer` - Magnetometer readings (5Hz)
- `bmx:interrupt` - Motion detection events
- `engine-ecu throttle` - Throttle events
- `engine-ecu odometer` - Odometer updates
- `engine-ecu kers` - KERS state changes
- `battery:0`, `battery:1` - Battery state changes (publishes field names)
- `gps` - GPS updates
- `buttons` - Immediate button press events
- `dashboard` - Dashboard status changes
- `power-manager` - Power state changes
- `settings` - Settings changes (publishes setting keys)

**Examples**:
```bash
# Watch vehicle and alarm status
lsc watch vehicle alarm

# Watch all battery events
lsc watch battery:0 battery:1

# Watch sensors with JSON output for scripting
lsc watch bmx:sensors --format=json > sensor-log.jsonl

# Watch button presses (for debugging input issues)
lsc watch buttons

# Watch everything (noisy!)
lsc watch vehicle alarm battery:0 battery:1 gps buttons
```

### GPS Commands (`lsc gps`)

#### status
```go
gps := HGETALL gps
internet := HGETALL internet
// Display: lat, lon, altitude, speed, course, timestamp
// Display: modem status, signal quality
```

#### track [duration]
```go
SUBSCRIBE gps
// Display GPS updates for specified duration
```

### Power Management (`lsc power`)

#### status
```go
pm := HGETALL power-manager
busyServices := HGETALL power-manager:busy-services
// Display: state, wakeup-source, hibernate-level
// Display: active inhibitors
```

#### hibernate
```go
LPUSH scooter:power hibernate
// Request hibernation
```

#### suspend
```go
LPUSH scooter:power suspend
// Request suspend
```

### BMX Sensor Commands (`lsc bmx`)

#### status
```go
bmx := HGETALL bmx
// Display: initialized, polling-rate-hz, streaming, interrupt,
//          pin, sensitivity, heading, last-interrupt-timestamp
```

#### stream [duration]
```go
LPUSH scooter:bmx streaming:enable
SUBSCRIBE bmx:sensors
// Display sensor data for duration
LPUSH scooter:bmx streaming:disable
```

#### sensitivity \<low|medium|high\>
```go
LPUSH scooter:bmx sensitivity:<level>
```

## Error Handling

### Connection Errors
- Detect Redis connection failure in `PersistentPreRunE`
- Display clear error: "Cannot connect to Redis at <addr>"
- Exit with code 1

### Command Errors
- Missing keys: "Key '<key>' not found in hash '<hash>'"
- Invalid values: "Invalid value '<value>' for '<command>'"
- Timeout: "Command timeout after <duration>"
- Service not responding: "Service '<service>' not responding"

### State Validation
- Check prerequisites before issuing commands
- Example: Cannot unlock if batteries not present
- Display helpful error messages with context

## Testing Strategy

### Unit Tests
- Redis client wrapper methods
- Command argument parsing
- Output formatting functions

### Integration Tests
- Requires Redis instance (can use Docker)
- Mock service responses
- Test command sequences

### Manual Testing
- Test on actual hardware (Deep Blue, ssh alias: `deep-blue`)
- Verify against tech-reference documentation
- Cross-check with `redis-cli` commands

### Test Scenarios

All test scenarios run on the scooter via SSH (Redis only listens on localhost/192.168.7.1):

**Basic Status**:
```bash
ssh deep-blue
/data/lsc-test status
# Verify: all fields present, formatted correctly
```

**Lock/Unlock Cycle**:
```bash
ssh deep-blue
/data/lsc-test vehicle lock
sleep 5
redis-cli HGET vehicle state  # Should be "stand-by"
/data/lsc-test vehicle unlock
sleep 2
redis-cli HGET vehicle state  # Should be "parked"
```

**Settings Management**:
```bash
ssh deep-blue
/data/lsc-test settings list
/data/lsc-test settings get alarm.enabled
/data/lsc-test settings set alarm.enabled true
/data/lsc-test settings get alarm.enabled  # Verify change
```

**Alarm Flow**:
```bash
ssh deep-blue
/data/lsc-test alarm status
/data/lsc-test alarm arm
sleep 5
redis-cli HGET alarm status  # Should be "armed"
# Shake scooter physically
redis-cli SUBSCRIBE alarm  # Watch for triggers
/data/lsc-test alarm disarm
```

## Future Enhancements

### Interactive Mode
- REPL-style interface
- Tab completion
- Command history

### Monitoring Dashboard
- TUI (terminal UI) with real-time updates
- Multiple panels for different metrics
- Graph/chart support for time-series data

### Scripting Support
- JSON output mode (`--json`)
- CSV output mode (`--csv`)
- Script-friendly exit codes

### Remote Management
- SSH tunnel support
- Multiple scooter profiles
- Fleet management commands

### Extended Diagnostics
- CAN bus message monitoring
- Network diagnostics (ping, traceroute via modem)
- Battery health analysis
- Trip statistics

### Configuration Files
- YAML/TOML config for default settings
- Named profiles for different scooters
- Command aliases

## Security Considerations

### Authentication
- Redis AUTH support (via flag or config)
- SSH key management for remote access

### Authorization
- Command whitelisting/blacklisting
- Read-only mode
- Admin-only commands

### Audit Logging
- Log all commands issued
- Timestamp and user tracking
- Integration with system logs

## Performance Optimization

### Connection Pooling
- Reuse Redis connections across commands
- Connection keepalive

### Parallel Queries
- Use Redis MGET for multiple hash reads
- Use pipelining for bulk operations

### Caching
- Cache static data (versions, serial numbers)
- TTL-based cache invalidation

### Batch Mode
- Execute multiple commands in single invocation
- Reduce connection overhead

## Implementation Roadmap

### Phase 1: Core Commands (CURRENT)
- [x] Project scaffolding
- [x] Redis client wrapper
- [x] `lsc status` command
- [x] Makefile for cross-compilation
- [ ] `lsc vehicle` commands (lock, unlock, hibernate, seatbox)
- [ ] `lsc settings` commands (list, get, set)
- [ ] `lsc alarm` commands (status, arm, disarm, trigger)

### Phase 2: Diagnostics & Monitoring
- [ ] `lsc diag version` - Version inventory
- [ ] `lsc diag faults` - Active faults display
- [ ] `lsc diag events` - Fault event stream viewer
- [ ] `lsc diag battery` - Detailed battery info
- [ ] `lsc watch <channel>...` - Multi-channel pub/sub monitoring
- [ ] `lsc diag blinkers/horn` - Hardware control

### Phase 3: Advanced Monitoring
- [ ] `lsc monitor` - Real-time TUI dashboard
  - Use bubbletea or termui library
  - Multi-panel layout with live updates
  - Event log scrolling
- [ ] `lsc gps` commands (status, tracking)
- [ ] `lsc power` commands (status, hibernate, suspend)
- [ ] `lsc bmx` commands (status, stream, sensitivity)

### Phase 4: Quality of Life
- [ ] JSON/CSV output modes (`--json`, `--csv` flags)
- [ ] Bash/Zsh completion scripts
- [ ] Man pages
- [ ] Configuration file support (~/.lscrc or /etc/lsc.conf)
  - Default Redis address
  - Output formatting preferences
  - Command aliases

### Phase 5: Advanced Features (Future)
- [ ] Profile management (multiple scooters)
- [ ] SSH tunnel support for remote access
- [ ] Trip analytics (requires local database)
- [ ] Configuration backup/restore
- [ ] Performance optimizations (connection pooling, caching)
- [ ] Comprehensive test suite

## Development Guidelines

### Code Organization
- One file per command group in `cmd/lsc/`
- Shared utilities in `internal/` packages
- Constants and types in dedicated files

### Naming Conventions
- Command functions: `<noun>Cmd` (e.g., `statusCmd`, `vehicleCmd`)
- Handler functions: `handle<Action>` (e.g., `handleLock`, `handleUnlock`)
- Redis operations: Use wrapper methods from `internal/redis`

### Error Messages
- Be specific about what went wrong
- Suggest corrective actions when possible
- Use consistent formatting

### Output Formatting
- Structured output for readability
- Consistent use of separators (===, ---)
- Color support (optional, via flags)

### Testing
- Write tests for all new commands
- Document test scenarios in comments
- Use table-driven tests for multiple cases

## References

- [Tech Reference: Redis Interface](../tech-reference/redis/README.md)
- [Tech Reference: Services](../tech-reference/services/README.md)
- [vehicle-service CLAUDE.md](../vehicle-service/CLAUDE.md)
- [battery-service CLAUDE.md](../battery-service/CLAUDE.md)
- [alarm-service CLAUDE.md](../alarm-service/CLAUDE.md)
- [bmx-service CLAUDE.md](../bmx-service/CLAUDE.md)
- [ecu-service CLAUDE.md](../ecu-service/CLAUDE.md)
