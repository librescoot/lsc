# lsd, the Librescoot Daemon

Part of the [Librescoot](https://librescoot.org/) open-source platform.

## Overview

`lsd` is the web-based complement to the `lsc` CLI: a small HTTP server that
runs on the MDB and presents scooter status, settings, files, cloud
connectivity and services in the browser. It speaks the same Redis
interfaces the vehicle services already expose and adds no new service
contracts.

## What it does

- **Dashboard**: vehicle state, batteries (main packs, auxiliary, connectivity
  box), odometer, motor controller, handlebar, seatbox, keycards, mobile
  network, GPS, the usb0 link state, board versions and serials, active
  faults and recent fault events. Data arrives over Server-Sent Events: the
  first event is a full snapshot, every later one patches a single field, so
  the page never polls while the stream is up. Controls cover lock and
  unlock, seatbox, blinkers, horn, power (stay awake, suspend, hibernate,
  reboot) and the service-mode overlay. Destructive actions ask first.
- **Settings**: schema-driven editor over the Redis `settings` hash. The
  schema comes from `settings:schema` (published by settings-service) and is
  rendered per type: switches for bools, selects for enums, range-checked
  numbers, duration and URL validation. Writes follow the documented
  contract: hash write first, then one publish on the `settings` channel per
  changed key. Clearing a value removes the key so the default applies
  again. Settings marked user-visible show by default; the rest sit behind
  "Show advanced".
- **Files**: browser for `/data` with upload (drag and drop or picker),
  download, delete, folder creation and folder download as a tar archive.
  Uploads go through a temporary file, fsync and atomic rename, like the
  standalone data-server.
- **Cloud**: shows the scooter's identifiers (VIN, IMEI, MDB and DBC serials)
  and the state of radio-gaga and uplink-service. Connecting to Sunshine
  uses the same exchange radio-gaga's own bootstrap mode performs: the user
  generates an installer credential in Sunshine's Account settings (no
  developer mode required), then pastes its bootstrap token or online installer
  link. LSD posts the hardware identifiers to `POST /api/v1/scooters/bootstrap`
  and installs the returned radio-gaga config. New scooters wait for the account
  owner to accept their claim in Sunshine. Online installer links must point to
  the configured Sunshine server; offline claim links run in the scooter's shell
  instead. A pasted config file can be installed for either service instead.
- **Updates**: per-board update status, progress and errors from
  update-service, planned full/delta download size on the current channel,
  channel look-up and switch, check now, and installing an uploaded `.mender`
  or `.delta`. MDB files install in place; DBC files are
  staged on the MDB and copied to the dashboard over usb0 on install.
- **System**: log bundles (created with `lsc logs`, downloadable), a
  journal viewer per unit, installed map and routing tiles, and modem
  details.
- **Navigation**: the dashboard's current destination and ordered route plan,
  with stops added from coordinates, the scooter's position, or saved locations.
  Stops can be removed or skipped; setting a single destination replaces the
  plan. Saved locations can be added, edited and deleted.
- **Keycards**: the authorized and master card lists, authorizing by UID or
  by tapping cards at the reader (keycard-service's learn mode, with per-tap
  events streamed live), adding a master by teach-in, and the last card the
  reader saw with a one-click authorize for unknown cards.
- **Services**: Librescoot's systemd units with their state, filters, and
  start, stop and restart.
- **Shell**: a command console on the MDB, collapsed at the bottom of the
  System page. Each command is its own `sh -c`, so only the working directory
  carries over; output streams as it arrives, stdout and stderr are told
  apart, and a running command can be interrupted and then killed. It is not
  a terminal: there is no pty, so nothing interactive works (no editors, no
  prompts, no password entry) and colour escapes are stripped rather than
  rendered. Output is capped at 4 MB and a command at 10 minutes. Every
  command is logged to the journal.

The switch in the top bar turns on **advanced mode** and is remembered per
browser. It shows the shell, destructive actions, settings not marked as
user-visible, full unit names, main-battery fault codes, and the current
contents of Redis hashes used by lsd. Advanced mode changes visibility only;
all endpoint checks still apply.

The interface is available in English and German. It follows the scooter's
`dashboard.language` setting, so the display and the web page speak the same
language; the selector in the top bar overrides that per browser. Strings
live in `internal/lsd/static/de.js`, keyed by the English text.

## Running it

```sh
lsd [-addr ADDRESS] [-wg-interface NAME] [-redis-addr ADDRESS] [-data DIRECTORY] [-token TOKEN] [-sunshine-url URL]
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `192.168.7.1:8090` | Primary HTTP listen address (MDB usb0). The default also listens on `wg0` if it has an IPv4 address. |
| `-wg-interface` | `wg0` with default `-addr`; otherwise *(none)* | Additional WireGuard interface to listen on; its IPv4 address is followed as it appears or changes. |
| `-redis-addr` | `localhost:6379` | Redis/Valkey address. |
| `-data` | `/data` | Directory exposed in the file browser. |
| `-token` | *(empty)* | When set, every request must carry this bearer token (`Authorization` header, or `?token=` for the SSE stream and downloads). |
| `-sunshine-url` | `https://sunshine.rescoot.org` | Sunshine instance the Cloud page talks to. |
| `-no-shell` | *(off)* | Disable the shell page and its API. |

### Availability and the usb0 gate

The primary listener retries every 5 seconds until its address is available.
The optional WireGuard listener starts independently when its interface has
an IPv4 address, and follows address changes. Neither listener binds to the
mobile-network interface. The USB address follows the usb0 link, which
vehicle-service owns (`system[usb0-gate]` records the decision;
`scooter.usb0-policy` can pin it `always-on` for servicing). The dashboard
shows the current gate state. [docs/lsd-always-on.md](docs/lsd-always-on.md)
discusses USB reachability without usb0.

### Security posture

The daemon has full control over the scooter: it queues power and vehicle
commands, writes settings, manipulates `/data`, restarts services and, on the
Shell page, runs arbitrary commands as root. That last one is a root shell in
a browser tab, so treat reaching lsd as equal to having the board's shell;
`-no-shell` removes the page and the API when that is too much. The default
binds only usb0 and an assigned `wg0` IPv4 address, not the mobile-network
interface. Every WireGuard peer that can reach the port can control the
scooter and run the shell unless `-token` or `-no-shell` is configured; use
those flags or firewall the port when peers are not fully trusted. lsd has no
TLS; the USB link is point-to-point and WireGuard encrypts its own traffic.

State-changing requests are rejected when the browser marks them as
cross-origin (`Origin` or `Sec-Fetch-Site`), so a page open in another tab
cannot drive the scooter through the laptop's usb0 connection. The two shell
endpoints go further, because that check passes requests that carry no
`Origin` at all: they also require `Content-Type: application/json` and an
`X-Lsd-Shell: 1` header. A cross-site page can send neither without a
preflight, which lsd never answers, so the only ways in are this page and a
deliberate `curl`. Commands are
validated against a fixed table, file paths are contained under `-data`,
and service actions only reach systemd for Librescoot unit names.

## API

| Request | Behaviour |
|---|---|
| `GET /` | Embedded single-page UI. |
| `GET /api/info` | Daemon version, data directory, Sunshine URL, whether a token is required. |
| `GET /api/status` | Snapshot: the raw status hashes plus active fault sets. |
| `GET /api/stream` | SSE. First event `status` is a snapshot; later messages are `{h, f, v}` field patches or `{h, f: "fault", set}` fault-set updates. |
| `GET /api/faults`, `GET /api/events` | Active fault sets; recent `events:faults` entries. |
| `GET /api/settings`, `GET /api/settings/schema` | Current values; raw schema. |
| `PUT /api/settings/set` | Validate and apply `{values: {key: value}}`. Returns `applied` and per-key `failures`. |
| `POST /api/control` | Queue a command: `{"action": "power-suspend"}`. |
| `GET/PUT/DELETE /api/files?path=` | List, upload, delete under `-data`. Folders need `recursive=1`. |
| `POST /api/files/mkdir` | Create a folder. |
| `GET /files/<path>?download=1` | Download a file, or a folder as tar. |
| `GET /api/services`, `POST /api/services/action` | Units and start/stop/restart/enable/disable. |
| `GET /api/updates` | The `ota` hash, board versions, `updates.*` settings and staged update files. |
| `PUT /api/updates/upload?name=&board=` | Store a `.mender` or `.delta` under `/data/ota/<board>/`, returns its SHA-256. Official filenames identify the board; other names require `board=mdb` or `board=dbc`. A conflicting board is rejected. |
| `POST /api/updates/action` | `{board, action}`: `check`, `preview`/`channel` with `channel`, `install`/`delete` with `file`. DBC installs copy the file to the dashboard's data-server first. |
| `GET/POST /api/system/logs` | List log bundles; create one with `{since}` via `lsc logs`. |
| `GET /api/system/journal?unit=&lines=` | Journal tail for a known unit, all units, or `dmesg`. |
| `GET /api/navigation`, `POST /api/navigation` | Current destination, route plan and saved locations; set `{latitude, longitude, address?, location-id?}`, replace with `{waypoints: [{latitude, longitude, label?}, ...]}`, or clear with `{clear: true}`. |
| `POST /api/navigation/plan` | Append `{action: "append", stop: {lat, lon, label?}}`, remove `{action: "remove", index, expected_revision}`, or skip `{action: "skip", expected_plan_id, expected_stop_id}`. Returns the owner's route plan. |
| `PUT/DELETE /api/navigation/locations` | Create or update `{id?, label, latitude, longitude}`; delete by `?id=`. Stored as `dashboard.saved-locations.<id>.*` in settings. |
| `GET /api/keycards` | Authorized and master card UIDs from keycard-service's files, plus the last card seen. |
| `POST /api/keycards/command` | `{command, uid?}`: add, remove, set-master, learn:start/stop, learn:master:start/stop, reset via `scooter:keycard`; waits for `command-result`. |
| `POST /api/shell` | `{cmd, cwd, id}`: run one command, streaming newline-delimited JSON frames (`{"o"}` stdout, `{"e"}` stderr, `{"x", "cwd"}` last). Needs `Content-Type: application/json` and `X-Lsd-Shell: 1`. |
| `POST /api/shell/signal` | `{id, signal}`: send `int`, `term` or `kill` to a running command's process group. Same two headers. |
| `GET /api/cloud` | Identity, connectivity service states, Sunshine URL. |
| `POST /api/cloud/bootstrap` | `{token}`: bootstrap token or online installer URL; register the scooter in Sunshine and install the returned radio-gaga config. |
| `POST /api/cloud/config` | `{service, yaml, config-path?}`: write a pasted config and restart the service. |

## Build and test

```sh
make build        # ARM binaries: bin/lsc and bin/lsd
make build-host   # host binaries
make test
make lint
```

The UI is plain HTML, CSS and JavaScript under `internal/lsd/static`,
embedded into the binary. Fonts (Abel, Hanken Grotesk, JetBrains Mono, all
SIL Open Font License) and the logo come from the Librescoot website.

## Deployment

Packaging is not done yet. [deploy/librescoot-lsd.service](deploy/librescoot-lsd.service)
is the intended unit; a meta-librescoot recipe that installs the binary and
the unit is still to be written. For development:

```sh
make deploy-lsd   # build ARM, copy to deep-blue:/data/lsd/, run on :8090
```

That runs the binary from `/data` without a unit, bound to all interfaces so
it is reachable over WireGuard as well as usb0. It survives until reboot or
until it is stopped by hand.

For UI work without redeploying, `make run-lsd-remote` runs lsd on the
development machine against deep-blue's Valkey over WireGuard. Everything
that is Redis (status, live stream, settings, vehicle and power commands)
acts on the scooter; the Services and Files pages and the Cloud page's
service states come from the local machine, because those use the local
systemctl and filesystem.

## Notes

- The daemon runs as root on the MDB; it has to, to restart services and
  write `/data`. Everything it can do, `lsc` can do from the board's shell.
- The MDB suspends in standby like everything else: when the scooter sleeps
  the page loses its stream and reconnects on wake.
- Installer credentials are reusable and revocable in Sunshine Account settings.
  A credential cannot take ownership of a scooter registered to another account.

## License

This project is licensed under the [Creative Commons
Attribution-NonCommercial 4.0 International License](LICENSE).
