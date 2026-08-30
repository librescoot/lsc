# Librescoot LSC

Part of the [Librescoot](https://librescoot.org/) open-source platform.

## Overview

`lsc` is the Librescoot control and diagnostics command-line client. It talks to
vehicle services through the Redis-compatible datastore used by a Librescoot
system; it is not a standalone vehicle controller.

## Capabilities

- Show consolidated vehicle, motor, battery, connectivity, GPS, map, update,
  and fault information.
- Control vehicle state, the seatbox and handlebar locks, alarm, LEDs, USB
  mode, modem, power state, and selected dashboard and engine functions.
- Manage keycards, saved locations, settings, systemd services, update
  operations, logs, and diagnostic recordings.
- Watch datastore pub/sub channels and emit machine-readable JSON for
  automation.
- Generate shell completion for Bash, Zsh, Fish, and PowerShell.

## Operation and interfaces

Run `lsc --help` and `lsc <command> --help` for the authoritative command and
argument reference. The primary command groups are `vehicle`, `alarm`, `led`,
`diag`, `gps`, `modem`, `nav`, `ota`, `power`, `service`, `settings`, `usb`,
`keycard`, `locations`, `logs`, `monitor`, and `watch`. Common shortcuts include
`status`, `lock`, `unlock`, `open`, `battery`, `faults`, and `maps`.

Most commands read hashes and publish events or push commands through the
Redis-compatible datastore. State-changing vehicle and alarm operations wait
for their expected state change by default; their `--no-block` option sends the
request without that confirmation. Use it only when asynchronous behaviour is
intentional.

Examples:

```sh
lsc status
lsc vehicle lock
lsc settings get dashboard.theme
lsc --json gps status
lsc watch vehicle battery:0
```

## Configuration

`lsc` has no configuration file. Its persistent options are:

| Option | Default | Purpose |
| --- | --- | --- |
| `--redis-addr <host:port>` | `192.168.7.1:6379` | Datastore endpoint |
| `--json` | disabled | Emit JSON where supported |
| `--verbose`, `-v` | disabled | Report client connection activity on stderr |

Settings commands operate on the `settings` hash. When the deployed system
provides `settings:schema`, `lsc settings list` uses it to describe known
settings and `lsc settings set` validates values unless `--force` is supplied.
It also publishes changed setting keys on the `settings` channel so services can
react.

## Build and test

The module declares Go 1.24. Build a host binary or the ARMv7 target binary
with the supplied Makefile:

```sh
make build-host    # bin/lsc for the build host
make build         # statically linked Linux ARMv7 binary in bin/lsc
make test          # go test -v ./...
make lint          # requires golangci-lint
```

`make build` sets `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=arm`, and `GOARM=7`.
Use `go build` directly when a different target is required.

## Deployment and runtime dependencies

The binary needs network access to a Redis-compatible datastore and the
Librescoot services that define the hashes, queues, and pub/sub channels it
uses. The Yocto layer packages it as `lsc` and installs shell completion under
`/etc/profile.d/`; it can otherwise be installed anywhere on `PATH`.

Service and power-management commands invoke local `systemctl`, `journalctl`,
and related platform facilities. Run them on the intended target with the
privileges those operations require.

## Operational notes

Commands can lock a vehicle, change its power state, control hardware, modify
settings, and manage OTA operations. Confirm the target endpoint before
executing state-changing commands, especially when overriding `--redis-addr`.
A successful send is not necessarily a successful vehicle action; keep the
default confirmation behaviour when possible and inspect status after critical
operations.

## License

This project is licensed under the [Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International License](LICENSE).

Made with ❤️ by the Librescoot community
