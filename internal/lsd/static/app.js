/* Librescoot management console. Vanilla JS, no build step.
 *
 * State is a copy of the Redis hashes the daemon streams. The first SSE
 * event is a full snapshot; every later event patches one field, so the
 * page never polls while the stream is up. */
"use strict";

const $ = (sel, el = document) => el.querySelector(sel);
const $$ = (sel, el = document) => [...el.querySelectorAll(sel)];

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

// ---------- API ----------

const API = {
  token: localStorage.getItem("lsd-token") || "",
  headers(json = true) {
    const h = {};
    if (json) h["Content-Type"] = "application/json";
    if (this.token) h["Authorization"] = "Bearer " + this.token;
    return h;
  },
  async req(method, path, body) {
    const opt = { method, headers: this.headers() };
    if (body !== undefined) opt.body = JSON.stringify(body);
    const resp = await fetch(path, opt);
    let data = null;
    try { data = await resp.json(); } catch { /* not JSON */ }
    // 422 is a partial result (per-key failures), not a transport error.
    if (!resp.ok && resp.status !== 422) {
      if (resp.status === 401) askToken();
      throw new Error(data && data.error ? data.error : `HTTP ${resp.status}`);
    }
    return data;
  },
  get(p) { return this.req("GET", p); },
  post(p, b) { return this.req("POST", p, b); },
  put(p, b) { return this.req("PUT", p, b); },
  del(p) { return this.req("DELETE", p); },
  // URL for navigation and EventSource, which cannot set headers.
  url(path, params = {}) {
    const u = new URL(path, location.origin);
    if (this.token) u.searchParams.set("token", this.token);
    for (const [k, v] of Object.entries(params)) u.searchParams.set(k, v);
    return u.toString();
  },
};

let tokenPrompting = false;
async function askToken() {
  if (tokenPrompting) return;
  tokenPrompting = true;
  const t = await promptDialog({ title: "Access token", body: "This lsd requires a token (its -token flag).", placeholder: "Token", password: true });
  tokenPrompting = false;
  if (t === null) return;
  API.token = t.trim();
  localStorage.setItem("lsd-token", API.token);
  connectStream();
  route();
}

// ---------- notices ----------

function notify(msg, isErr = false) {
  const el = document.createElement("div");
  el.className = "toast" + (isErr ? " is-error" : "");
  el.innerHTML = `<span class="msg">${esc(msg)}</span><button type="button" aria-label="Dismiss">&times;</button>`;
  $("button", el).addEventListener("click", () => el.remove());
  $("#notices").appendChild(el);
  if (!isErr) setTimeout(() => el.remove(), 4000);
  else setTimeout(() => el.remove(), 12000);
}

// ---------- dialogs ----------

const dlg = $("#dialog");
function openDialog({ title, body, ok = "OK", danger = false, input = null }) {
  return new Promise(resolve => {
    $("#dialog-title").textContent = title;
    $("#dialog-body").textContent = body || "";
    $("#dialog-body").hidden = !body;
    const inp = $("#dialog-input");
    inp.hidden = !input;
    if (input) {
      inp.value = input.value || "";
      inp.placeholder = input.placeholder || "";
      inp.type = input.password ? "password" : "text";
    }
    const okBtn = $("#dialog-ok");
    okBtn.textContent = ok;
    okBtn.classList.toggle("btn-danger", danger);
    okBtn.classList.toggle("btn-primary", !danger);
    const done = () => {
      dlg.removeEventListener("close", done);
      resolve(dlg.returnValue === "ok" ? (input ? inp.value : true) : (input ? null : false));
    };
    dlg.addEventListener("close", done);
    dlg.returnValue = "cancel";
    dlg.showModal();
    if (input) inp.focus(); else okBtn.focus();
  });
}
$("#dialog-cancel").addEventListener("click", () => dlg.close("cancel"));
$("#dialog form").addEventListener("submit", e => { e.preventDefault(); dlg.close("ok"); });
const confirmDialog = (o) => openDialog(o);
const promptDialog = ({ title, body, placeholder, value, password, ok = "OK" }) =>
  openDialog({ title, body, ok, input: { placeholder, value, password } });

// ---------- theme ----------

const themeBtn = $("#theme-toggle");
function applyTheme() {
  const saved = localStorage.getItem("lsd-theme");
  const dark = saved ? saved === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
  document.documentElement.dataset.theme = dark ? "dark" : "light";
}
themeBtn.addEventListener("click", () => {
  const dark = document.documentElement.dataset.theme === "dark";
  localStorage.setItem("lsd-theme", dark ? "light" : "dark");
  applyTheme();
});
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", applyTheme);
applyTheme();

// ---------- routing ----------

const Views = {};
let currentView = "";
function route() {
  const view = (location.hash.slice(1) || "dashboard").split("/")[0];
  const known = Views[view] ? view : "dashboard";
  $$(".view").forEach(v => { v.hidden = v.id !== "view-" + known; });
  $$(".tabs a").forEach(a => a.classList.toggle("is-active", a.dataset.view === known));
  currentView = known;
  Views[known]();
}
window.addEventListener("hashchange", route);

// ---------- live state ----------

const state = { hashes: {}, faults: {}, live: false };
const H = (name) => state.hashes[name] || {};

let es = null;
function connectStream() {
  if (es) es.close();
  es = new EventSource(API.url("/api/stream"));
  es.addEventListener("status", e => {
    const snap = JSON.parse(e.data);
    state.hashes = snap.hashes || {};
    state.faults = snap.faults || {};
    setConn("live");
    scheduleRender();
  });
  es.onmessage = e => {
    const ev = JSON.parse(e.data);
    if (ev.h === "keycard:events") { onKeycardEvent(ev.f, ev.ts); scheduleRender(); return; }
    if (ev.set !== undefined) {
      if (ev.set.length) state.faults[ev.h] = ev.set; else delete state.faults[ev.h];
      if (currentView === "dashboard") loadEvents();
    } else {
      const h = state.hashes[ev.h] || (state.hashes[ev.h] = {});
      if (ev.v === undefined || ev.v === null) delete h[ev.f]; else h[ev.f] = ev.v;
      if (ev.h === "settings") onSettingChanged(ev.f, ev.v);
      if (ev.h === "keycard" && ev.f === "uid") kc.lastSeenAt = Date.now();
    }
    scheduleRender();
  };
  es.onopen = () => setConn("live");
  es.onerror = () => setConn("offline");
}
function setConn(s) {
  state.live = s === "live";
  const el = $("#conn");
  el.dataset.state = s;
  $(".conn-text", el).textContent = s;
  if (currentView === "dashboard") syncCommandAvailability();
}

let renderQueued = false;
function scheduleRender() {
  if (renderQueued) return;
  renderQueued = true;
  requestAnimationFrame(() => {
    renderQueued = false;
    if (currentView === "dashboard") renderDashboard();
    if (currentView === "keycards") renderLastCard();
    if (currentView === "navigation") renderDestination();
  });
}

// ---------- formatting helpers ----------

const has = (v) => v !== undefined && v !== null && v !== "";
const num = (v) => { const n = Number(v); return isFinite(n) ? n : null; };
function human(s) {
  if (!has(s)) return "";
  s = String(s).replace(/[-_]/g, " ").replace(/\bsim\b/i, "SIM").replace(/\busb\b/i, "USB");
  return s.charAt(0).toUpperCase() + s.slice(1);
}
const fmtV = (mv) => { const n = num(mv); return n === null ? null : (n / 1000).toFixed(2) + " V"; };
const fmtKm = (m) => { const n = num(m); return n === null ? null : (n / 1000).toFixed(1) + " km"; };
const fmtTemp = (c) => { const n = num(c); return n === null ? null : `${Math.round(n)} °C`; };
const fmtPct = (p) => { const n = num(p); return n === null ? null : `${Math.round(n)} %`; };
function ago(iso) {
  const t = Date.parse(iso);
  if (!isFinite(t)) return "";
  const s = Math.round((Date.now() - t) / 1000);
  if (s < 5) return "just now";
  if (s < 90) return `${s} s ago`;
  if (s < 5400) return `${Math.round(s / 60)} min ago`;
  return new Date(t).toLocaleString();
}

const GOOD = new Set(["ok", "ideal", "active", "running", "run", "connected", "on", "home", "present", "fix-established", "open", "normal", "closed", "enabled", "ready-to-drive", "parked", "float-charge", "charging", "locked", "down", "idle"]);
const WARN = new Set(["stand-by", "standby", "suspending", "pre-suspend", "searching", "roaming", "not-charging", "warning", "unknown", "hop-on", "waiting-seatbox", "shutting-down", "updating", "sim-locked", "inactive", "dead", "off", "disabled", "2d", "none"]);
const BAD = new Set(["fault", "error", "failed", "critical", "triggered", "missing", "denied", "disconnected", "sim-missing", "registration-denied", "registration-failed", "no-modem", "status-error", "powered-off", "locked-out"]);
function tone(v) {
  const s = String(v ?? "").toLowerCase();
  if (GOOD.has(s)) return "is-good";
  if (BAD.has(s)) return "is-bad";
  if (WARN.has(s)) return "is-warn";
  return "";
}
const status = (v, label) => has(v) ? `<span class="status ${tone(v)}">${esc(label ?? human(v))}</span>` : null;

// Facts: array of [label, valueHTML|null, asideText?]; null values are skipped.
function renderFacts(el, rows) {
  const out = rows.filter(r => r && has(r[1])).map(([label, value, aside]) =>
    `<dt>${esc(label)}</dt><dd>${value}${has(aside) ? `<span class="aside">${esc(aside)}</span>` : ""}</dd>`);
  el.innerHTML = out.join("") || `<div class="empty">No data yet.</div>`;
}

// ---------- dashboard ----------

const VEHICLE_TONE = {
  "ready-to-drive": "is-good", "parked": "is-good", "stand-by": "", "hop-on": "is-warn", "hop-on-learning": "is-warn",
  "waiting-seatbox": "is-warn", "shutting-down": "is-warn", "updating": "is-warn",
  "waiting-hibernation": "is-warn", "waiting-hibernation-seatbox": "is-warn", "waiting-hibernation-confirm": "is-warn",
};

function renderDashboard() {
  const v = H("vehicle"), ecu = H("engine-ecu"), pm = H("power-manager"), sys = H("system");
  const gps = H("gps"), net = H("internet"), modem = H("modem"), alarm = H("alarm");
  const vmdb = H("version:mdb"), vdbc = H("version:dbc");

  // Hero: the vehicle state, then what the scooter is.
  const sw = $("#hero-state");
  if (has(v.state)) {
    sw.textContent = human(v.state);
    sw.className = "state-word " + (VEHICLE_TONE[v.state] ?? "");
  } else if (!state.live) {
    sw.textContent = "Not connected";
    sw.className = "state-word is-bad";
  }
  // Under the state word: the firmware line, then status chips that only
  // appear when they say something (alarm state, power manager not idle).
  const chips = [];
  if (has(alarm.status)) {
    const st = alarm.status;
    const t = /trigger/.test(st) ? "is-bad" : /armed$/.test(st) ? "is-good" : "";
    chips.push(`<span class="status ${t}">Alarm ${esc(human(st).toLowerCase())}</span>`);
  }
  if (has(pm.state) && pm.state !== "running") chips.push(`<span class="status is-warn">Power manager ${esc(human(pm.state).toLowerCase())}</span>`);
  if (has(sys["usb0-gate"])) chips.push(`<span class="status ${sys["usb0-gate"] === "open" ? "is-good" : ""}">USB link ${sys["usb0-gate"] === "open" ? "held open" : "follows dashboard"}</span>`);
  $("#hero-sub").innerHTML = `${has(vmdb.pretty_name) ? `<div class="hero-version">${esc(vmdb.pretty_name)}</div>` : ""}${chips.length ? `<div class="hero-chips">${chips.join("")}</div>` : ""}`;

  // Batteries.
  const batts = [];
  for (const id of ["0", "1"]) {
    const b = H("battery:" + id);
    if (!Object.keys(b).length) continue;
    const present = b.present !== "false";
    const temps = [0, 1, 2, 3].map(i => num(b["temperature:" + i])).filter(t => t !== null);
    const meta = [fmtV(b.voltage), temps.length ? `${Math.max(...temps)} °C` : null,
      has(b["state-of-health"]) ? `health ${b["state-of-health"]} %` : null, has(b["cycle-count"]) ? `${b["cycle-count"]} cycles` : null];
    batts.push(battRow(`Battery ${Number(id) + 1}`, present ? num(b.charge) : null, present ? b.state : "not present", meta, !present));
  }
  const aux = H("aux-battery");
  if (Object.keys(aux).length) {
    batts.push(battRow("Auxiliary 12 V", num(aux.charge), aux["charge-status"], [fmtV(aux.voltage)], false, true));
  }
  const cbb = H("cb-battery");
  if (Object.keys(cbb).length) {
    const present = cbb.present !== "false";
    batts.push(battRow("Connectivity box", present ? num(cbb.charge) : null, present ? cbb["charge-status"] : "not present",
      [fmtTemp(cbb.temperature), has(cbb["state-of-health"]) ? `health ${cbb["state-of-health"]} %` : null], !present));
  }
  $("#hero-batteries").innerHTML = batts.join("") || `<div class="muted">No battery data.</div>`;

  // Vehicle facts.
  const ecuTemp = fmtTemp(ecu.temperature);
  renderFacts($("#facts-vehicle"), [
    ["Odometer", esc(fmtKm(ecu.odometer))],
    ["Speed", has(ecu.speed) ? `${esc(ecu.speed)} km/h` : null, has(ecu.rpm) && num(ecu.rpm) > 0 ? `${ecu.rpm} rpm` : null],
    ["Motor", status(ecu["ecu-status"]), has(ecu["motor:voltage"]) ? fmtV(ecu["motor:voltage"]) : null],
    ["Controller temperature", ecuTemp ? esc(ecuTemp) : null],
    ["Handlebar", status(v["handlebar:lock-state"]), has(v["handlebar:position"]) ? human(v["handlebar:position"]) : null],
    ["Seatbox", status(v["seatbox:lock"])],
    ["Kickstand", status(v.kickstand)],
    ["Dashboard", status(v["dashboard:power"], v["dashboard:power"] === "on" ? "Powered" : "Off")],
    ["Blinkers", has(v["blinker:state"]) ? esc(human(v["blinker:state"])) : null],
    ["Keycards", has(sys["keycard-authorized-count"]) ? `${esc(sys["keycard-authorized-count"])} authorized` : null,
      has(sys["keycard-master-count"]) ? `${sys["keycard-master-count"]} master` : null],
    ["Energy", has(ecu["energy:consumed"]) ? `${(num(ecu["energy:consumed"]) / 1000).toFixed(1)} kWh used` : null,
      has(ecu["energy:recovered"]) ? `${(num(ecu["energy:recovered"]) / 1000).toFixed(1)} kWh recovered` : null],
  ]);

  // Connectivity.
  const coord = (has(gps.latitude) && has(gps.longitude)) ? `${num(gps.latitude).toFixed(5)}, ${num(gps.longitude).toFixed(5)}` : null;
  const gpsState = gps.state === "fix-established" ? `Fix (${gps.fix || "3d"})` : human(gps.state);
  renderFacts($("#facts-conn"), [
    ["USB link", status(sys["usb0-gate"], sys["usb0-gate"] === "open" ? "Held open" : sys["usb0-gate"] === "closed" ? "Follows dashboard" : null),
      sys["usb0-gate"] === "closed" ? "this page drops when the dashboard is off" : null],
    ["Internet", status(net.status), [net["access-tech"], has(net["signal-quality"]) ? `signal ${net["signal-quality"]} %` : null].filter(Boolean).join(", ")],
    ["Mobile network", has(modem["operator-name"]) ? esc(modem["operator-name"]) : status(modem["power-state"]),
      [modem.registration, modem["error-state"] && modem["error-state"] !== "ok" ? modem["error-state"] : null].filter(Boolean).map(human).join(", ")],
    ["SIM", status(modem["sim-state"]), has(net["sim-iccid"]) ? `ICCID ${net["sim-iccid"]}` : null],
    ["IP address", has(net["ip-address"]) ? `<span class="mono">${esc(net["ip-address"])}</span>` : null],
    ["GPS", status(gps.state, gpsState),
      [has(gps["satellites-used"]) ? `${gps["satellites-used"]} of ${gps["satellites-visible"] || "?"} satellites` : null, has(gps.updated) ? `updated ${ago(gps.updated)}` : null].filter(Boolean).join(", ")],
    ["Position", coord ? `<span class="mono">${esc(coord)}</span>` : null,
      [has(gps.altitude) ? `altitude ${Math.round(num(gps.altitude))} m` : null, has(gps.speed) && num(gps.speed) >= 1 ? `${num(gps.speed).toFixed(0)} km/h` : null].filter(Boolean).join(", ")],
    ["Bluetooth", has(sys["nrf-fw-version"]) ? "Module present" : null],
  ]);

  // Boards.
  const boards = [
    ["MDB", vmdb.version || vmdb.version_id, vmdb.serial_number_real],
    ["Dashboard", vdbc.version || vdbc.version_id, vdbc.serial_number_real],
    ["Motor controller", has(ecu["fw-version"]) ? `firmware ${ecu["fw-version"]}` : null, null],
    ["Bluetooth module", has(sys["nrf-fw-version"]) ? `firmware ${sys["nrf-fw-version"]}` : null, null],
  ].filter(r => has(r[1]));
  $("#boards tbody").innerHTML = boards.map(([n, ver, sn]) =>
    `<tr><td>${esc(n)}</td><td>${esc(ver)}${sn ? `<div class="serial">${esc(sn)}</div>` : ""}</td></tr>`).join("")
    || `<tr><td class="muted">Not reported yet.</td></tr>`;

  // Faults.
  const SRC = { "vehicle": "Vehicle", "engine-ecu": "Motor controller", "battery:0": "Battery 1", "battery:1": "Battery 2" };
  const rows = [];
  for (const [src, list] of Object.entries(state.faults)) for (const f of list) rows.push([SRC[src] || src, f]);
  $("#faults").innerHTML = rows.length
    ? rows.map(([s, f]) => `<div class="fault"><span class="src">${esc(s)}</span><code>${esc(f)}</code></div>`).join("")
    : `<div class="none">None.</div>`;

  syncCommandAvailability();
}

function battRow(name, pct, stateText, meta, absent = false, coarse = false) {
  const p = pct === null ? null : Math.max(0, Math.min(100, pct));
  const barTone = p === null ? "" : p <= 10 ? "is-bad" : p <= 25 ? "is-warn" : "";
  const val = p === null ? esc(human(stateText)) : `${Math.round(p)}<small>%${coarse ? " approx." : ""}</small>`;
  return `<div class="batt ${absent ? "is-absent" : ""}">
    <div class="batt-name">${esc(name)}</div>
    <div class="batt-value">${val}${p !== null && has(stateText) ? `<small>${esc(human(stateText))}</small>` : ""}</div>
    ${p === null ? "" : `<div class="batt-bar ${barTone}"><span style="width:${p}%"></span></div>`}
    ${meta.filter(Boolean).length ? `<div class="batt-meta">${meta.filter(Boolean).map(m => `<span>${esc(m)}</span>`).join("")}</div>` : ""}
  </div>`;
}

async function loadEvents() {
  const tb = $("#events tbody");
  try {
    const evs = await API.get("/api/events");
    tb.innerHTML = (evs || []).slice(0, 10).map(ev => {
      const ms = Number(String(ev.id).split("-")[0]);
      const t = isFinite(ms) ? new Date(ms).toLocaleString() : ev.id;
      return `<tr><td>${esc(t)}</td><td><code>${esc(ev.code || "")}</code><div class="sub">${esc(ev.group || "")}${ev.description ? ": " + esc(ev.description) : ""}</div></td></tr>`;
    }).join("") || `<tr><td class="muted" colspan="2">None since boot.</td></tr>`;
  } catch (err) {
    tb.innerHTML = `<tr><td class="muted" colspan="2">${esc(err.message)}</td></tr>`;
  }
}

// ---------- commands ----------
// Every button posts one action. Buttons with data-hold require the pointer
// (or Enter/Space) to be held for that many milliseconds; releasing early
// cancels. A native click after a completed hold must not fire the command
// a second time, so clicks on hold buttons are always swallowed.

const CMD_LABEL = {
  "unlock": "Unlocking", "lock": "Locking", "seatbox-open": "Opening seatbox", "honk": "Honk",
  "alarm-arm": "Arming alarm", "alarm-disarm": "Disarming alarm", "alarm-stop": "Stopping alarm", "alarm-trigger": "Sounding alarm",
  "blinkers-left": "Blinking left", "blinkers-right": "Blinking right", "blinkers-both": "Hazards on", "blinkers-off": "Blinkers off",
  "power-run": "Power-off cancelled", "power-hibernate-manual": "Hibernating", "power-reboot": "Rebooting",
  "power-hibernate-for": "Sleeping, wake timer set", "power-hibernate-cancel": "Wake timer cancelled",
  "service-mode-on": "Service mode on", "service-mode-off": "Service mode off",
};

async function sendCommand(btn) {
  const cmd = btn.dataset.cmd;
  const body = { action: cmd };
  if (cmd === "power-hibernate-for") body.seconds = Number($("#sleep-for-duration").value);
  btn.disabled = true;
  try {
    await API.post("/api/control", body);
    notify(CMD_LABEL[cmd] || `${cmd}: sent`);
  } catch (err) { notify(err.message, true); }
  finally { btn.disabled = false; syncCommandAvailability(); }
}

function attachHold(btn, duration) {
  const bar = document.createElement("span"); bar.className = "hold-bar"; bar.setAttribute("aria-hidden", "true");
  const tip = document.createElement("span"); tip.className = "hold-tip"; tip.setAttribute("aria-hidden", "true"); tip.textContent = "Hold to confirm";
  btn.append(bar, tip);
  btn.setAttribute("aria-description", `Hold for ${(duration / 1000).toFixed(1).replace(/\.0$/, "")} seconds to confirm`);
  let startedAt = null, frame = null, done = false;
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;

  const reset = () => {
    if (frame) cancelAnimationFrame(frame);
    frame = null; startedAt = null;
    btn.classList.remove("is-holding");
    btn.style.removeProperty("--hold");
  };
  const tick = () => {
    if (startedAt === null) return;
    const p = Math.min((performance.now() - startedAt) / duration, 1);
    btn.style.setProperty("--hold", p.toFixed(3));
    tip.textContent = p >= 1 ? "Sending" : p >= 0.5 ? "Keep holding" : "Hold to confirm";
    if (p >= 1) { done = true; reset(); sendCommand(btn); return; }
    frame = requestAnimationFrame(tick);
  };
  const start = (e) => {
    if (btn.disabled || startedAt !== null) return;
    if (e.type === "pointerdown" && e.button !== 0) return;
    e.preventDefault();
    done = false;
    startedAt = performance.now();
    btn.classList.add("is-holding");
    if (reduced) btn.style.setProperty("--hold", "1");
    frame = requestAnimationFrame(tick);
  };
  const stop = () => { if (startedAt !== null) { reset(); tip.textContent = "Hold to confirm"; } };

  btn.addEventListener("pointerdown", start);
  btn.addEventListener("pointerup", stop);
  btn.addEventListener("pointerleave", stop);
  btn.addEventListener("pointercancel", stop);
  btn.addEventListener("blur", stop);
  btn.addEventListener("keydown", e => {
    if (e.key === "Escape") return stop();
    if ((e.key === "Enter" || e.key === " ") && !e.repeat) start(e);
  });
  btn.addEventListener("keyup", e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); stop(); } });
  btn.addEventListener("click", e => { e.preventDefault(); done = false; });
  btn.addEventListener("contextmenu", e => e.preventDefault());
}

$$("[data-cmd]").forEach(btn => {
  if (btn.dataset.hold) {
    attachHold(btn, Number(btn.dataset.hold));
    return;
  }
  btn.addEventListener("click", async () => {
    if (btn.dataset.confirm) {
      const ok = await confirmDialog({ title: btn.dataset.title || btn.textContent.trim(), body: btn.dataset.confirm, ok: btn.textContent.trim() });
      if (!ok) return;
    }
    sendCommand(btn);
  });
});

// Commands need the live connection; the alarm buttons follow the alarm
// state and the active blinker is outlined.
function syncCommandAvailability() {
  const live = state.live;
  $("#cmd-offline").hidden = live;
  $$("#commands [data-cmd]").forEach(b => { if (!b.classList.contains("is-busy")) b.disabled = !live; });

  const status = H("alarm").status || "";
  const mode = /trigger/.test(status) ? "triggered" : /armed$/.test(status) ? "armed" : status === "disarmed" ? "disarmed" : "";
  $$("[data-alarm]").forEach(b => { b.hidden = !(b.dataset.alarm === mode || (mode === "triggered" && b.dataset.alarm === "armed")); });
  if (!mode) $$("[data-alarm]").forEach(b => { b.hidden = b.dataset.alarm !== "disarmed"; b.disabled = true; });

  const blink = H("vehicle")["blinker:state"] || "off";
  $$("[data-blinker]").forEach(b => b.classList.toggle("is-on", b.dataset.blinker === blink));

  const pm = H("power-manager");
  const wakeSecs = num(pm["wake-timer-seconds"]);
  const armed = pm["wake-timer-armed"] === "true" || (wakeSecs !== null && wakeSecs > 0);
  $("#cmd-cancel-wake").hidden = !armed;
  $("#cmd-cancel-power").hidden = !/pending|imminent/.test(pm.state || "");

  const settings = H("settings");
  const idle = num(settings["pm.hibernation-timer"]);
  const sched = settings["pm.scheduled-hibernate-enabled"] === "true";
  const parts = [];
  parts.push(`Idle target: ${settings["pm.default-state"] || "suspend"}.`);
  if (idle !== null) parts.push(idle > 0 ? `Inactivity hibernation after ${humanDuration(idle)}.` : "Inactivity hibernation off.");
  parts.push(sched ? `Scheduled hibernation on (${settings["pm.scheduled-hibernate-cron"] || "?"}, wake after ${settings["pm.scheduled-hibernate-duration"] || "?"}).` : "Scheduled hibernation off.");
  if (armed) parts.push(`A wake timer is armed${wakeSecs ? ` for ${humanDuration(wakeSecs)}` : ""}.`);
  if (has(pm.state) && pm.state !== "running") parts.unshift(`Power manager: ${human(pm.state)}.`);
  $("#power-hint").innerHTML = `${esc(parts.join(" "))} <a href="#settings/pm-service">Change in Settings</a>`;
}

function humanDuration(secs) {
  if (secs % 86400 === 0) return `${secs / 86400} d`;
  if (secs % 3600 === 0) return `${secs / 3600} h`;
  if (secs % 60 === 0) return `${secs / 60} min`;
  return `${secs} s`;
}

Views.dashboard = function () {
  renderDashboard();
  loadEvents();
  if (!state.live) API.get("/api/status").then(snap => { state.hashes = snap.hashes || {}; state.faults = snap.faults || {}; renderDashboard(); }).catch(() => {});
};

// ---------- settings ----------

let schema = null;
let values = {};
const dirty = new Map();
let failures = {};

Views.settings = async function () {
  try {
    if (!schema) {
      [schema, values] = await Promise.all([API.get("/api/settings/schema"), API.get("/api/settings").then(r => r.values || {})]);
    }
    const target = decodeURIComponent(location.hash.split("/")[1] || "");
    if (target && !document.getElementById("group-" + target)) {
      // The linked service only has advanced settings; reveal them.
      const advanced = Object.values(schema).some(m => m.service === target && !m["user-visible"]);
      if (advanced) $("#settings-advanced").checked = true;
    }
    renderSettings();
    if (target) document.getElementById("group-" + target)?.scrollIntoView({ block: "start" });
  } catch (err) { notify("Settings unavailable: " + err.message, true); }
};

// Live settings changes from other writers (lsc, the dashboard) update the
// form unless the user is editing that very key.
function onSettingChanged(key, value) {
  if (key.startsWith("dashboard.saved-locations.") && currentView === "navigation" && nav.editing === null) { Views.navigation(); return; }
  if (!schema) return;
  if (value === null || value === undefined) delete values[key]; else values[key] = value;
  if (dirty.has(key)) return;
  const row = $(`.setting[data-key="${CSS.escape(key)}"]`);
  if (row && document.activeElement && row.contains(document.activeElement)) return;
  if (row) row.replaceWith(settingRowEl(key, schema[key]));
}

function renderSettings() {
  const filter = $("#settings-filter").value.trim().toLowerCase();
  const advanced = $("#settings-advanced").checked;
  const groups = new Map();
  for (const key of Object.keys(schema).sort()) {
    const m = schema[key];
    if (!advanced && !m["user-visible"] && !dirty.has(key)) continue;
    if (filter && ![key, m.label, m.description, m.service].some(s => (s || "").toLowerCase().includes(filter))) continue;
    const svc = m.service || "other";
    if (!groups.has(svc)) groups.set(svc, []);
    groups.get(svc).push(key);
  }
  const container = $("#settings-groups");
  container.innerHTML = "";
  $("#settings-jump").innerHTML = [...groups.keys()].map(s => `<a href="#settings/${esc(s)}" data-jump="${esc(s)}">${esc(s)}</a>`).join("");
  if (!groups.size) {
    container.innerHTML = `<div class="settings-empty">Nothing matches.${advanced ? "" : " Advanced settings are hidden."}</div>`;
  }
  for (const [svc, keys] of groups) {
    const g = document.createElement("section");
    g.className = "group";
    g.id = "group-" + svc;
    g.innerHTML = `<h2>${esc(svc)}</h2>`;
    for (const key of keys) g.appendChild(settingRowEl(key, schema[key]));
    container.appendChild(g);
  }
  updateSavebar();
  markCurrentGroup();
}

// The sticky service tabs underline the group whose heading was scrolled
// past most recently.
function markCurrentGroup() {
  const groups = $$("#settings-groups .group");
  if (!groups.length) return;
  const line = 120;
  let current = groups[0];
  for (const g of groups) if (g.getBoundingClientRect().top <= line) current = g;
  const svc = current.id.replace(/^group-/, "");
  $$("#settings-jump a").forEach(a => a.classList.toggle("is-current", a.dataset.jump === svc));
}
window.addEventListener("scroll", () => { if (currentView === "settings") markCurrentGroup(); }, { passive: true });
$("#settings-jump").addEventListener("click", e => {
  const a = e.target.closest("[data-jump]");
  if (!a) return;
  e.preventDefault();
  const g = document.getElementById("group-" + a.dataset.jump);
  if (g) g.scrollIntoView({ behavior: "smooth", block: "start" });
});

function currentValue(key) {
  return dirty.has(key) ? dirty.get(key) : (values[key] ?? "");
}

function settingRowEl(key, m) {
  const row = document.createElement("div");
  row.className = "setting" + (dirty.has(key) ? " is-changed" : "") + (failures[key] ? " is-failed" : "");
  row.dataset.key = key;
  const cur = currentValue(key);
  const def = m.default === null || m.default === undefined ? "" : String(m.default);
  const tags = [m.transient ? `<span class="tag" title="Kept in memory only, reset on reboot">until reboot</span>` : "",
    m["read-only"] ? `<span class="tag">read only</span>` : ""].join("");
  let control;
  const id = "s-" + key.replace(/[^a-z0-9]/gi, "-");
  if (m["read-only"]) {
    control = `<div class="setting-ro">${esc(cur || def || "(not set)")}</div>`;
  } else if (m.type === "bool") {
    const on = cur === "" ? def === "true" : cur === "true";
    control = `<div class="row"><label class="switch"><input type="checkbox" id="${id}" data-key="${esc(key)}" ${on ? "checked" : ""}><span class="track"></span></label><label for="${id}" class="muted">${on ? "On" : "Off"}</label></div>`;
  } else if (m.type === "enum") {
    const opts = (m.values || []).map(v => `<option value="${esc(v.value)}" ${v.value === cur ? "selected" : ""}>${esc(v.label || v.value)}</option>`).join("");
    const defLabel = (m.values || []).find(v => v.value === def);
    control = `<div class="row"><select id="${id}" data-key="${esc(key)}"><option value="" ${cur === "" ? "selected" : ""}>${esc(def !== "" ? `Default (${defLabel ? defLabel.label || defLabel.value : def})` : "Not set")}</option>${opts}</select></div>`;
  } else if (m.type === "int" || m.type === "float") {
    const attrs = [has(m.min) ? `min="${m.min}"` : "", has(m.max) ? `max="${m.max}"` : "", `step="${m.type === "int" ? 1 : "any"}"`].join(" ");
    control = `<div class="row"><input type="number" id="${id}" data-key="${esc(key)}" value="${esc(cur)}" placeholder="${esc(def)}" ${attrs}>${m.unit ? `<span class="unit">${esc(m.unit)}</span>` : ""}</div>`;
  } else {
    control = `<div class="row"><input type="text" id="${id}" data-key="${esc(key)}" value="${esc(cur)}" placeholder="${esc(def || m.example || "")}" spellcheck="false"></div>`;
  }
  const range = (m.type === "int" || m.type === "float") && (has(m.min) || has(m.max)) ? ` (${has(m.min) ? m.min : "any"} to ${has(m.max) ? m.max : "any"})` : "";
  const defLine = m["read-only"] ? "" :
    `<div class="setting-default">${def !== "" ? `Default ${esc(def)}${esc(range)}` : `No default${esc(range)}`}${(cur !== "" && !m["read-only"]) ? ` <a href="#" data-reset="${esc(key)}">Reset</a>` : ""}</div>`;
  row.innerHTML = `
    <div>
      <div><label class="setting-label" for="${id}">${esc(m.label || key)}</label><span class="setting-key">${esc(key)}</span></div>
      <div class="setting-desc">${esc(m.description || "")}${tags}</div>
    </div>
    <div class="setting-control">${control}${defLine}${failures[key] ? `<div class="setting-error">${esc(failures[key])}</div>` : ""}</div>`;

  const input = $("[data-key]", row);
  if (input) {
    const handler = () => {
      const val = input.type === "checkbox" ? (input.checked ? "true" : "false") : input.value;
      if (val === (values[key] ?? "") || (input.type === "checkbox" && !has(values[key]) && val === def)) dirty.delete(key); else dirty.set(key, val);
      delete failures[key];
      row.classList.toggle("is-changed", dirty.has(key));
      row.classList.remove("is-failed");
      if (input.type === "checkbox") $(`label[for="${id}"].muted`, row).textContent = input.checked ? "On" : "Off";
      updateSavebar();
    };
    input.addEventListener(input.type === "checkbox" || input.tagName === "SELECT" ? "change" : "input", handler);
  }
  const reset = $("[data-reset]", row);
  if (reset) reset.addEventListener("click", e => {
    e.preventDefault();
    if ((values[key] ?? "") === "") dirty.delete(key); else dirty.set(key, "");
    row.replaceWith(settingRowEl(key, m));
    updateSavebar();
  });
  return row;
}

function updateSavebar() {
  const n = dirty.size;
  $("#savebar").hidden = n === 0 || currentView !== "settings";
  $("#savebar-text").textContent = n === 1 ? "1 change" : `${n} changes`;
}

$("#settings-save").addEventListener("click", async () => {
  if (!dirty.size) return;
  const btn = $("#settings-save");
  btn.classList.add("is-busy");
  try {
    const res = await API.put("/api/settings/set", { values: Object.fromEntries(dirty) });
    for (const [k, v] of Object.entries(res.applied || {})) { if (v === "") delete values[k]; else values[k] = v; dirty.delete(k); }
    failures = res.failures || {};
    const nf = Object.keys(failures).length, na = Object.keys(res.applied || {}).length;
    if (nf) notify(`${na} saved, ${nf} rejected`, true);
    else notify(na === 1 ? "Saved 1 setting" : `Saved ${na} settings`);
    renderSettings();
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});
$("#settings-revert").addEventListener("click", () => { dirty.clear(); failures = {}; renderSettings(); });
$("#settings-filter").addEventListener("input", debounce(renderSettings, 150));
$("#settings-advanced").addEventListener("change", renderSettings);

function debounce(fn, ms) { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; }

// ---------- navigation ----------

const nav = { locations: [], editing: null };
const fmtCoord = (lat, lon) => `${Number(lat).toFixed(5)}, ${Number(lon).toFixed(5)}`;

Views.navigation = async function () {
  try {
    const data = await API.get("/api/navigation");
    state.hashes.navigation = data.destination || {};
    nav.locations = data.locations || [];
    renderDestination();
    renderLocations();
  } catch (err) { notify(err.message, true); }
};

function renderDestination() {
  const d = H("navigation");
  const el = $("#nav-current");
  const set = has(d.latitude) && has(d.longitude);
  $("#nav-current-actions").hidden = !set;
  if (!set) { el.className = "nav-current muted"; el.textContent = "None."; return; }
  el.className = "nav-current";
  el.innerHTML = `${has(d.address) ? `<div class="name">${esc(d.address)}</div>` : ""}<div class="coords">${esc(fmtCoord(d.latitude, d.longitude))}</div>${has(d.timestamp) ? `<div class="when">set ${esc(ago(d.timestamp))}</div>` : ""}`;
}

function renderLocations() {
  const el = $("#nav-locations");
  if (!nav.locations.length) { el.innerHTML = `<div class="kc-empty">None saved.</div>`; return; }
  el.innerHTML = nav.locations.map(l => {
    const editing = nav.editing === l.id;
    return `<div class="loc" data-id="${l.id}">
      <div><span class="name">${esc(l.label || "Unnamed")}</span>${l["last-used-at"] ? `<span class="when">used ${esc(ago(l["last-used-at"]))}</span>` : ""}</div>
      <span class="row-actions">
        <button type="button" class="btn btn-small btn-quiet" data-loc-go="${l.id}">Navigate</button>
        <button type="button" class="btn btn-small btn-quiet" data-loc-edit="${l.id}">${editing ? "Cancel" : "Edit"}</button>
        <button type="button" class="btn btn-small btn-quiet" data-loc-del="${l.id}">Delete</button>
      </span>
      <div class="coords">${esc(fmtCoord(l.latitude, l.longitude))}</div>
      ${editing ? `<form class="loc-edit" data-loc-form="${l.id}">
        <input class="label" value="${esc(l.label)}" placeholder="Name" aria-label="Name" required>
        <input value="${l.latitude.toFixed(6)}" placeholder="Latitude" aria-label="Latitude" inputmode="decimal" required>
        <input value="${l.longitude.toFixed(6)}" placeholder="Longitude" aria-label="Longitude" inputmode="decimal" required>
        <button type="submit" class="btn btn-small btn-primary">Save</button>
      </form>` : ""}
    </div>`;
  }).join("");
}

function readCoords(latEl, lonEl) {
  const lat = Number(latEl.value.trim().replace(",", ".")), lon = Number(lonEl.value.trim().replace(",", "."));
  if (!isFinite(lat) || !isFinite(lon) || latEl.value.trim() === "" || lonEl.value.trim() === "") throw new Error("Latitude and longitude are needed, as decimal degrees");
  return { latitude: lat, longitude: lon };
}

async function navigateTo(latitude, longitude, address, locationId) {
  await API.post("/api/navigation", { latitude, longitude, address: address || "", "location-id": locationId ?? null });
  notify(address ? `Navigating to ${address}` : "Destination set");
  Views.navigation();
}

$("#nav-form").addEventListener("submit", async e => {
  e.preventDefault();
  try {
    const c = readCoords($("#nav-lat"), $("#nav-lon"));
    await navigateTo(c.latitude, c.longitude, $("#nav-label").value.trim(), null);
  } catch (err) { notify(err.message, true); }
});
$("#nav-save").addEventListener("click", async () => {
  try {
    const c = readCoords($("#nav-lat"), $("#nav-lon"));
    const label = $("#nav-label").value.trim();
    if (!label) return notify("Give the location a name first", true);
    await API.put("/api/navigation/locations", { label, ...c });
    notify(`Saved ${label}`);
    $("#nav-form").reset();
    Views.navigation();
  } catch (err) { notify(err.message, true); }
});
$("#nav-use-gps").addEventListener("click", () => {
  const g = H("gps");
  if (!has(g.latitude) || !has(g.longitude) || g.fix === "none") return notify("No GPS fix right now", true);
  $("#nav-lat").value = Number(g.latitude).toFixed(6);
  $("#nav-lon").value = Number(g.longitude).toFixed(6);
});
$("#nav-clear").addEventListener("click", async () => {
  try { await API.post("/api/navigation", { clear: true }); notify("Destination cleared"); Views.navigation(); }
  catch (err) { notify(err.message, true); }
});
$("#nav-locations").addEventListener("click", async e => {
  const go = e.target.closest("[data-loc-go]"), ed = e.target.closest("[data-loc-edit]"), del = e.target.closest("[data-loc-del]");
  if (go) {
    const l = nav.locations.find(x => x.id === Number(go.dataset.locGo));
    try { await navigateTo(l.latitude, l.longitude, l.label, l.id); } catch (err) { notify(err.message, true); }
  } else if (ed) {
    const id = Number(ed.dataset.locEdit);
    nav.editing = nav.editing === id ? null : id;
    renderLocations();
    if (nav.editing !== null) $(`[data-loc-form="${id}"] input`)?.focus();
  } else if (del) {
    const l = nav.locations.find(x => x.id === Number(del.dataset.locDel));
    const ok = await confirmDialog({ title: "Delete location", body: `Remove ${l.label || "this location"} from the saved locations?`, ok: "Delete", danger: true });
    if (!ok) return;
    try { await API.del(`/api/navigation/locations?id=${l.id}`); notify(`Deleted ${l.label}`); Views.navigation(); }
    catch (err) { notify(err.message, true); }
  }
});
$("#nav-locations").addEventListener("submit", async e => {
  const form = e.target.closest("[data-loc-form]");
  if (!form) return;
  e.preventDefault();
  const [labelEl, latEl, lonEl] = $$("input", form);
  try {
    const c = readCoords(latEl, lonEl);
    await API.put("/api/navigation/locations", { id: Number(form.dataset.locForm), label: labelEl.value.trim(), ...c });
    nav.editing = null;
    notify("Location saved");
    Views.navigation();
  } catch (err) { notify(err.message, true); }
});

// ---------- keycards ----------

const kc = { authorized: [], master: [], learning: null, learned: [], lastSeenAt: 0 };

Views.keycards = async function () {
  try {
    const data = await API.get("/api/keycards");
    kc.authorized = data.authorized || [];
    kc.master = data.master || [];
    if (data.last) { state.hashes.keycard = data.last; kc.lastSeenAt = Date.now(); }
    renderKeycards();
    renderLastCard();
  } catch (err) { notify(err.message, true); }
};

const fmtUID = (u) => u.replace(/(..)(?=.)/g, "$1 ");

function renderKeycards() {
  const row = (uid, kind, extra = "") => `<div class="kc-row ${extra}"><span class="uid">${esc(fmtUID(uid))}</span>${kind ? `<span class="tag">${esc(kind)}</span>` : ""}
    ${kind === "master" ? "" : `<button type="button" class="btn btn-small btn-quiet" data-kc-remove="${esc(uid)}" ${kc.authorized.length <= 1 ? 'disabled title="The last card stays"' : ""}>Remove</button>`}</div>`;
  const learned = kc.learned.filter(u => !kc.authorized.includes(u));
  $("#kc-authorized").innerHTML = [
    ...kc.authorized.map(u => row(u, "")),
    ...learned.map(u => row(u, "tapped, unsaved", "is-new")),
  ].join("") || `<div class="kc-empty">No authorized cards yet.</div>`;
  $("#kc-master").innerHTML = kc.master.map(u => row(u, "master")).join("") || `<div class="kc-empty">No master card. The next card tapped at the reader becomes one.</div>`;

  const learn = $("#kc-learn-start").closest(".kc-learn");
  learn.classList.toggle("is-active", kc.learning === "cards");
  $("#kc-learn-start").hidden = kc.learning === "cards";
  $("#kc-learn-stop").hidden = kc.learning !== "cards";
  $("#kc-learn-hint").textContent = kc.learning === "cards"
    ? `Tap cards at the reader${learned.length ? `, ${learned.length} so far` : ""}.`
    : "";
  $("#kc-master-start").hidden = kc.learning === "master";
  $("#kc-master-stop").hidden = kc.learning !== "master";
  $("#kc-master-start").closest(".kc-learn").classList.toggle("is-active", kc.learning === "master");
  if (kc.learning === "master") $("#kc-master-start").insertAdjacentHTML("afterend", "");
}

function renderLastCard() {
  const k = H("keycard");
  const el = $("#kc-last");
  if (!has(k.uid)) return;
  const known = kc.authorized.includes(k.uid) || kc.master.includes(k.uid);
  const verdict = k.authentication === "passed" ? `<span class="status is-good">Accepted</span>` : `<span class="status is-bad">Rejected</span>`;
  const when = kc.lastSeenAt ? ago(new Date(kc.lastSeenAt).toISOString()) : "";
  el.classList.remove("muted");
  el.innerHTML = `<span class="uid">${esc(fmtUID(k.uid))}</span>${verdict}${has(k.type) ? `<span class="muted">${esc(k.type)} card</span>` : ""}<span class="muted">${esc(when)}</span>
    ${known ? "" : `<button type="button" class="btn btn-small" data-kc-authorize="${esc(k.uid)}">Authorize this card</button>`}`;
}

function onKeycardEvent(ev, ts) {
  const [kind, ...rest] = ev.split(":");
  const uid = rest[rest.length - 1];
  switch (kind) {
    case "card-learned": if (!kc.learned.includes(uid)) kc.learned.push(uid); kc.learning = kc.learning || "cards"; notify(`Tapped ${fmtUID(uid)}`); break;
    case "card-duplicate": notify(`${fmtUID(uid)} is already authorized`); break;
    case "mode-entered": if (rest[0] === "master") kc.learning = "master"; break;
    case "mode-exited": if (kc.learning === "master") kc.learning = null; break;
    case "master-learned": notify(`Master ${fmtUID(uid)} added`); kc.learning = null; Views.keycards(); return;
    case "rejected": notify(`${fmtUID(uid)} is already authorized, so it cannot be a master`, true); break;
    case "error": notify(`Could not save ${fmtUID(uid)}`, true); break;
    case "reset": kc.learning = null; kc.learned = []; Views.keycards(); return;
  }
  if (currentView === "keycards") renderKeycards();
}

async function keycardCommand(command, uid, btn) {
  if (btn) btn.classList.add("is-busy");
  try {
    const resp = await fetch("/api/keycards/command", { method: "POST", headers: API.headers(), body: JSON.stringify({ command, uid }) });
    const data = await resp.json().catch(() => ({}));
    if (data.authorized) kc.authorized = data.authorized;
    if (data.master) kc.master = data.master;
    if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
    return data;
  } finally { if (btn) btn.classList.remove("is-busy"); renderKeycards(); }
}

$("#kc-add-form").addEventListener("submit", async e => {
  e.preventDefault();
  const uid = $("#kc-add-uid").value;
  if (!uid.trim()) return;
  try { await keycardCommand("add", uid, $("button", e.target)); $("#kc-add-uid").value = ""; notify("Card authorized"); }
  catch (err) { notify(err.message, true); }
});
$("#view-keycards").addEventListener("click", async e => {
  const rm = e.target.closest("[data-kc-remove]");
  if (rm) {
    const uid = rm.dataset.kcRemove;
    const ok = await confirmDialog({ title: "Remove card", body: `${fmtUID(uid)} will no longer unlock the scooter.`, ok: "Remove", danger: true });
    if (!ok) return;
    try { await keycardCommand("remove", uid, rm); notify("Card removed"); } catch (err) { notify(err.message, true); }
  }
  const au = e.target.closest("[data-kc-authorize]");
  if (au) {
    try { await keycardCommand("add", au.dataset.kcAuthorize, au); notify("Card authorized"); renderLastCard(); } catch (err) { notify(err.message, true); }
  }
});
$("#kc-learn-start").addEventListener("click", async e => {
  try { await keycardCommand("learn:start", "", e.target); kc.learning = "cards"; kc.learned = []; renderKeycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-learn-stop").addEventListener("click", async e => {
  try { await keycardCommand("learn:stop", "", e.target); kc.learning = null; kc.learned = []; notify("Cards saved"); Views.keycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-master-start").addEventListener("click", async e => {
  try { await keycardCommand("learn:master:start", "", e.target); kc.learning = "master"; renderKeycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-master-stop").addEventListener("click", async e => {
  try { await keycardCommand("learn:master:stop", "", e.target); kc.learning = null; renderKeycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-reset").addEventListener("click", async e => {
  const ok = await confirmDialog({ title: "Forget all cards", body: "Removes all cards. Until a new master is taught in at the reader, no card unlocks the scooter.", ok: "Forget all cards", danger: true });
  if (!ok) return;
  try { await keycardCommand("reset", "", e.target); kc.learning = null; kc.learned = []; notify("All cards forgotten"); Views.keycards(); } catch (err) { notify(err.message, true); }
});

// ---------- files ----------

let filesPath = "";
const encPath = (p) => p.split("/").map(encodeURIComponent).join("/");
const ICON_DIR = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`;
const ICON_FILE = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M6 3h8l5 5v13a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z"/><path d="M14 3v5h5"/></svg>`;

Views.files = function () {
  const p = location.hash.split("/").slice(1).join("/");
  filesPath = decodeURIComponent(p || "");
  renderFiles();
};
function filesGo(p) { location.hash = "files" + (p ? "/" + encPath(p) : ""); }

async function renderFiles() {
  const tb = $("#files-table tbody");
  try {
    const data = await API.get(`/api/files?path=${encodeURIComponent(filesPath)}`);
    renderCrumbs();
    const rows = [];
    if (filesPath) {
      rows.push(`<tr><td><a href="#files/${esc(encPath(filesPath.split("/").slice(0, -1).join("/")))}" class="is-dir">${ICON_DIR}<span class="fname">..</span></a></td><td></td><td></td><td></td></tr>`);
    }
    for (const e of data.entries || []) {
      const full = filesPath ? `${filesPath}/${e.name}` : e.name;
      const dl = API.url(`/files/${encPath(full)}`, { download: "1" });
      const link = e.dir
        ? `<a href="#files/${esc(encPath(full))}" class="is-dir">${ICON_DIR}<span class="fname">${esc(e.name)}</span></a>`
        : `<a href="${esc(dl)}">${ICON_FILE}<span class="fname">${esc(e.name)}</span></a>`;
      rows.push(`<tr>
        <td>${link}</td>
        <td class="num">${e.dir ? "" : esc(humanSize(e.size))}</td>
        <td>${e.mtime ? esc(new Date(e.mtime * 1000).toLocaleString()) : ""}</td>
        <td class="actions"><span class="row-actions">
          <a class="btn btn-small btn-quiet" href="${esc(dl)}">${e.dir ? "Download .tar" : "Download"}</a>
          <button type="button" class="btn btn-small btn-quiet" data-del="${esc(full)}" data-dir="${e.dir ? 1 : 0}">Delete</button>
        </span></td>
      </tr>`);
    }
    tb.innerHTML = rows.join("") || `<tr class="files-empty"><td colspan="4">Empty folder.</td></tr>`;
  } catch (err) { notify(err.message, true); }
}

function humanSize(n) {
  if (n === undefined || n === null) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} kB`;
  if (n < 1073741824) return `${(n / 1048576).toFixed(1)} MB`;
  return `${(n / 1073741824).toFixed(2)} GB`;
}

function renderCrumbs() {
  const parts = filesPath ? filesPath.split("/") : [];
  let html = parts.length ? `<a href="#files">/data</a>` : `<span class="here">/data</span>`;
  let acc = "";
  parts.forEach((p, i) => {
    acc = acc ? `${acc}/${p}` : p;
    html += `<span class="sep">/</span>`;
    html += i === parts.length - 1 ? `<span class="here">${esc(p)}</span>` : `<a href="#files/${esc(encPath(acc))}">${esc(p)}</a>`;
  });
  $("#files-crumbs").innerHTML = html;
}

$("#files-table").addEventListener("click", async e => {
  const del = e.target.closest("[data-del]");
  if (!del) return;
  const name = del.dataset.del.split("/").pop();
  const isDir = del.dataset.dir === "1";
  const ok = await confirmDialog({
    title: isDir ? "Delete folder" : "Delete file",
    body: isDir ? `Delete "${name}" and everything inside it? This cannot be undone.` : `Delete "${name}"? This cannot be undone.`,
    ok: "Delete", danger: true,
  });
  if (!ok) return;
  try {
    await API.del(`/api/files?path=${encodeURIComponent(del.dataset.del)}${isDir ? "&recursive=1" : ""}`);
    notify(`Deleted ${name}`);
    renderFiles();
  } catch (err) { notify(err.message, true); }
});

$("#files-upload").addEventListener("click", () => $("#file-input").click());
$("#file-input").addEventListener("change", async e => { await uploadFiles([...e.target.files]); e.target.value = ""; });

async function uploadFiles(files) {
  for (const f of files) {
    const target = filesPath ? `${filesPath}/${f.name}` : f.name;
    try {
      const resp = await fetch(`/api/files?path=${encodeURIComponent(target)}`, { method: "PUT", headers: API.headers(false), body: f });
      if (!resp.ok) {
        let msg = `HTTP ${resp.status}`;
        try { msg = (await resp.json()).error || msg; } catch { /* keep */ }
        throw new Error(`${f.name}: ${msg}`);
      }
      notify(`Uploaded ${f.name}`);
    } catch (err) { notify(err.message, true); }
  }
  renderFiles();
}

const dz = $("#dropzone");
dz.addEventListener("dragover", e => { e.preventDefault(); dz.classList.add("is-drag"); });
dz.addEventListener("dragleave", () => dz.classList.remove("is-drag"));
dz.addEventListener("drop", async e => { e.preventDefault(); dz.classList.remove("is-drag"); await uploadFiles([...e.dataTransfer.files]); });

$("#files-mkdir").addEventListener("click", async () => {
  const name = await promptDialog({ title: "New folder", body: `In /data${filesPath ? "/" + filesPath : ""}`, placeholder: "Folder name", ok: "Create" });
  if (!name || !name.trim()) return;
  const target = filesPath ? `${filesPath}/${name.trim()}` : name.trim();
  try {
    await API.post("/api/files/mkdir", { path: target });
    filesGo(target);
  } catch (err) { notify(err.message, true); }
});

// ---------- cloud ----------

Views.cloud = async function () {
  try { renderCloud(await API.get("/api/cloud")); } catch (err) { notify(err.message, true); }
};

function renderCloud(data) {
  const id = data.identity || {};
  const sunshine = data["sunshine-url"] || "https://sunshine.rescoot.org";
  $("#cloud-sunshine-link").href = sunshine + "/settings";
  renderFacts($("#cloud-identity"), [
    ["Identifier", has(id.vin) ? `<span class="mono">${esc(id.vin)}</span>` : `<span class="muted">none yet</span>`],
    ["IMEI", has(id.imei) ? `<span class="mono">${esc(id.imei)}</span>` : `<span class="muted">modem not ready</span>`],
    ["MDB serial", has(id["mdb-serial"]) ? `<span class="mono">${esc(id["mdb-serial"])}</span>` : null],
    ["Dashboard serial", has(id["dbc-serial"]) ? `<span class="mono">${esc(id["dbc-serial"])}</span>` : null],
  ]);

  const services = data.services || {};
  const order = ["radio-gaga", "uplink"];
  const NAMES = { "radio-gaga": "radio-gaga", "uplink": "uplink-service" };
  $("#cloud-services").innerHTML = order.filter(n => services[n]).map(n => {
    const s = services[n];
    const lines = [];
    if (!s.installed) lines.push(`<span class="muted">Not installed</span>`);
    else lines.push(status(s.active, s.active === "active" ? "Running" : human(s.active)));
    if (s.configured) {
      lines.push(`${s.backend === "sunshine" ? "Connected to Sunshine as" : "Custom backend,"} <span class="mono">${esc(s.identifier)}</span>`);
      if (s.backend === "custom") lines.push(`<span class="muted">${esc(s["server-url"])}</span>`);
    } else {
      lines.push(`<span class="muted">Not configured</span>`);
    }
    lines.push(`<span class="muted mono">${esc(s["config-path"])}</span>`);
    return `<div class="svc"><div class="svc-name">${esc(NAMES[n])}<span class="svc-unit">${esc(s.unit)}</span></div><div class="svc-lines">${lines.join("")}</div></div>`;
  }).join("");

  const connected = order.map(n => services[n]).filter(s => s && s.configured && s.backend === "sunshine");
  const box = $("#cloud-connected");
  if (connected.length) {
    box.hidden = false;
    box.textContent = `Already connected to Sunshine as ${connected[0].identifier}. Connecting again moves the scooter to the token owner's account and replaces the config.`;
    $("#cloud-bootstrap-form button").textContent = "Reconnect";
  } else {
    box.hidden = true;
    $("#cloud-bootstrap-form button").textContent = "Connect";
  }
}

function showResult(el, obj, isErr = false) {
  el.hidden = false;
  el.classList.toggle("is-error", isErr);
  el.textContent = typeof obj === "string" ? obj : JSON.stringify(obj, null, 2);
}

$("#cloud-bootstrap-form").addEventListener("submit", async e => {
  e.preventDefault();
  const token = $("#cloud-token").value.trim();
  if (!token) return;
  const btn = $("button", e.target);
  const out = $("#cloud-bootstrap-out");
  btn.classList.add("is-busy");
  try {
    const res = await API.post("/api/cloud/bootstrap", { token });
    const problems = [res.error, res["restart-error"], res["enable-error"]].filter(Boolean);
    showResult(out, res, problems.length > 0);
    if (problems.length) notify("Config written, but " + problems.join("; "), true);
    else notify(`Connected to Sunshine as ${res.identifier || "this scooter"}`);
    $("#cloud-token").value = "";
    Views.cloud();
  } catch (err) {
    showResult(out, err.message, true);
    notify(err.message, true);
  } finally { btn.classList.remove("is-busy"); }
});

$("#cloud-config-form").addEventListener("submit", async e => {
  e.preventDefault();
  const yaml = $("#cloud-yaml").value;
  if (!yaml.trim()) return notify("Paste a config first", true);
  const btn = $("button[type=submit]", e.target);
  const out = $("#cloud-config-out");
  btn.classList.add("is-busy");
  try {
    const res = await API.post("/api/cloud/config", { service: $("#cloud-service").value, yaml, "config-path": $("#cloud-path").value.trim() });
    const problems = [res.error, res["restart-error"], res["enable-error"]].filter(Boolean);
    showResult(out, res, problems.length > 0);
    notify(problems.length ? "Config written, but " + problems.join("; ") : `Config installed, ${res.service} restarted`, problems.length > 0);
    Views.cloud();
  } catch (err) {
    showResult(out, err.message, true);
    notify(err.message, true);
  } finally { btn.classList.remove("is-busy"); }
});

// ---------- services ----------

let units = [];
let unitFilter = "all";

Views.services = async function () {
  try {
    units = (await API.get("/api/services")).units || [];
    renderServices();
  } catch (err) { notify(err.message, true); }
};

function unitTone(u) {
  if (u.load === "masked") return "is-warn";
  if (u.active === "active") return "is-good";
  if (u.active === "failed") return "is-bad";
  return "";
}
function unitStateLabel(u) {
  if (u.load === "masked") return "Masked";
  if (u.active === "failed") return "Failed";
  if (u.active === "active") return u.sub === "exited" ? "Completed" : "Running";
  if (u.active === "activating") return "Starting";
  if (u.active === "deactivating") return "Stopping";
  return "Stopped";
}

function renderServices() {
  const rows = units.filter(u => {
    switch (unitFilter) {
      case "masked": return u.load === "masked";
      case "failed": return u.active === "failed";
      case "active": return u.active === "active";
      case "inactive": return u.active !== "active" && u.active !== "failed" && u.load !== "masked";
      default: return true;
    }
  });
  $("#services-table tbody").innerHTML = rows.map(u => {
    const st = unitStateLabel(u);
    const running = u.active === "active" && u.sub !== "exited";
    return `<tr>
      <td><span class="unit">${esc(u.unit.replace(/\.service$/, ""))}</span><div class="sub">${esc(u.description || "")}</div></td>
      <td><span class="status ${unitTone(u)}">${st}</span>${u.sub && !["running", "dead", "exited"].includes(u.sub) ? `<span class="sub">${esc(u.sub)}</span>` : ""}</td>
      <td class="actions"><span class="row-actions">
        <button type="button" class="btn btn-small btn-quiet" data-svc="${esc(u.unit)}" data-act="restart">Restart</button>
        ${running ? `<button type="button" class="btn btn-small btn-quiet" data-svc="${esc(u.unit)}" data-act="stop">Stop</button>`
                  : `<button type="button" class="btn btn-small btn-quiet" data-svc="${esc(u.unit)}" data-act="start">Start</button>`}
      </span></td>
    </tr>`;
  }).join("") || `<tr><td colspan="3" class="muted">Nothing matches this filter.</td></tr>`;
}

$$(".chip").forEach(btn => btn.addEventListener("click", () => {
  unitFilter = btn.dataset.filter;
  $$(".chip").forEach(b => b.classList.toggle("is-active", b === btn));
  renderServices();
}));
$("#services-refresh").addEventListener("click", Views.services);

$("#services-table").addEventListener("click", async e => {
  const btn = e.target.closest("[data-svc]");
  if (!btn) return;
  const { svc, act } = btn.dataset;
  const name = svc.replace(/\.service$/, "");
  const critical = ["valkey.service", "redis.service", "librescoot-vehicle.service", "librescoot-pm.service"].includes(svc);
  if (act !== "start" && critical) {
    const ok = await confirmDialog({ title: `${human(act)} ${name}`, body: `${name} is a core service. Stopping or restarting it interrupts the whole scooter briefly and may disconnect this page.`, ok: human(act), danger: true });
    if (!ok) return;
  }
  btn.classList.add("is-busy");
  try {
    await API.post("/api/services/action", { unit: svc, action: act });
    notify(`${ {restart: "Restarted", stop: "Stopped", start: "Started"}[act] || human(act)} ${name}`);
    Views.services();
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});

// ---------- boot ----------

connectStream();
route();
API.get("/api/info").then(info => { if (info.version) document.title = `Librescoot ${info.version}`; }).catch(() => {});
