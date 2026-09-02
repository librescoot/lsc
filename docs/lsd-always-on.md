# Always-on lsd: reachability without usb0

*Side note from the lsd introduction: how the web interface could stay
reachable even when usb0 is down. Today lsd deliberately follows the same
gate as everything else on the management network.*

## Where we are

usb0 is the MDB's USB gadget network link to the DBC-side connector (and to
a host PC when the scooter is parked next to a laptop). vehicle-service owns
the link:

- `scooter.usb0-policy` = `auto` (default): usb0 follows dashboard power,
  but the gate stays open while too few keycards are paired
  ((master ≥ 1 AND authorized ≥ 1) OR authorized ≥ 2 is the threshold),
  because usb0 is the recovery path into a scooter whose keycards do not work.
- `scooter.usb0-policy` = `always-on` (transient): usb0 stays up regardless
  of dashboard state; the installer/diagnostic mode.
- In USB mass-storage mode (`lsc usb ums`, ums-service) the network gadget is
  replaced by a mass-storage gadget: usb0 does not exist at all.
- `librescoot-usb0-failsafe.timer` raises usb0 if vehicle-service never
  recorded a gate decision, the last-resort recovery path.

So lsd on `192.168.7.1:8090` is reachable when the gate is open or the
policy is `always-on`, and unreachable when the DBC is off or UMS mode is
active. That is fine for a bench tool but limits remote maintenance.

## Option A: keep the gate open more often (cheapest)

The gate exists to close an attack surface and save the USB link power when
the scooter sleeps. Both concerns are small: the link is point-to-point to
the DBC or a parked host, and an idle NCM gadget draws little. Making the
gate default open after N keycards are paired (e.g. only close when ≥ 2
authorized cards exist *and* the user has not opted out) keeps recovery
available at near-zero cost. This is a policy change in vehicle-service, not
in lsd.

Trade-off: the gate's current semantics are the documented installer
behaviour; changing the default needs a look at who else listens on usb0
(data-server exposes /data read-write, dropbear may be enabled).

## Option B: composite USB gadget (proper fix)

Today the gadget config switches between two single-function setups: NCM
network (usb0) and mass storage (UMS). Linux `g_multi`/functionfs can
present several functions at once:

    MDB gadget
    ├── NCM  (usb0, 192.168.7.1)     <- lsd / data-server / ssh
    ├── Mass Storage (/data image)   <- UMS
    └── (optional) ACM serial console

Each function is independent, so the mass-storage and network functions can
coexist. UMS mode then no longer removes the network path, and lsd stays
reachable whenever the cable is plugged in.

Implementation notes:
- ums-service would stop swapping gadget configs and instead enable/disable
  only the mass-storage function (configfs: `functions/mass_storage.0`
  unbind/bind from the config), or use `g_multi` with the UMS function
  created/removed on demand.
- USB gadget functions can only change while the UDC is unbound, so a
  configfs dance (unbind UDC, modify, rebind) is required; that is what
  the current mode switch does anyway.
- Firewalling per interface already exists in netconfig's iptables; rules can
  treat usb0 as trusted and, say, the modem interface as hostile today.

Trade-offs: the DBC also consumes usb0 (dashboard link); a composite gadget
must not confuse the DBC's netconfig. The DBC only uses usb0 as NCM, and
adding functions does not change the NCM address, so risk is low, but it
needs a real test on hardware (the DBC is a hard requirement, installer
boot flows depend on usb0 coming up cleanly).

## Option C: dedicated management subnet / interface

Keep one NCM gadget but expose lsd additionally on a second logical network:
a second CIDR over the same NCM (alias address 192.168.8.1) or a separate
ECM/RNDIS gadget interface. Firewall per port rather than per interface:
always allow 8090 (lsd) + 8080 (data-server, read-only mode?) while keeping
Redis and SSH gated behind the keycard logic. This weakens the gate without
new hardware paths, useful when Option B is too invasive.

## Option D: serial console

An ACM serial gadget function is always present, cheap, and needs no
network: lsd could speak a terminal UI over `/dev/ttyGS0` (the boot logs
already use serial). Full control with zero network attack surface; the UX
is `lsc` in a browser-less shell. Combined with `lsc` this may be all a
service technician needs when the gate is closed, the dashboards'
service-mode screen could even render lsd's dashboard in text.

## Option E: BLE

mdb-nrf52 provides an always-on BLE path (bluetooth-service). A GATT
characteristic bridging a small subset of lsd (status + settings) would be
reachable with a phone, no gate involved. Bandwidth is tiny and pairing UX
needs design; this is the "app does it" path rather than a maintenance one.

## Recommendation

1. Short term: keep the usb0 gate as is (lsd matches it), rely on
   `scooter.usb0-policy=always-on` for servicing, and note that service mode
   already pins that policy plus keeps the vehicle awake.
2. Mid term: Option B (composite gadget with NCM + mass storage), because it
   removes the whole class of "UMS mode locked me out" and keeps lsd
   reachable whenever a cable is attached.
3. Option D as an escape hatch alongside B: a serial console costs one
   gadget function and survives every network misconfiguration.
