/* Librescoot management console. Vanilla JS, no build step.
 *
 * State is a copy of the Redis hashes the daemon streams. The first SSE
 * event is a full snapshot; every later event patches one field, so the
 * page never polls while the stream is up. */
"use strict";
I18N.lang = I18N.pick();

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
  const tok = await promptDialog({ title: t("Access token"), body: t("This lsd requires a token (its -token flag)."), placeholder: t("Token"), password: true });
  tokenPrompting = false;
  if (tok === null) return;
  API.token = tok.trim();
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

// ---------- sticky offsets ----------
// The header wraps to two rows on phones and the settings tabs stick under
// it; both heights feed the CSS so sticky offsets and jump targets match.
function measureSticky() {
  const bar = $(".topbar").getBoundingClientRect().height;
  const jump = $$(".jump").find(j => j.offsetParent !== null && j.childElementCount);
  const stack = bar + (jump ? jump.getBoundingClientRect().height : 0);
  document.documentElement.style.setProperty("--topbar-h", `${Math.round(bar)}px`);
  document.documentElement.style.setProperty("--sticky-h", `${Math.round(stack)}px`);
}
new ResizeObserver(measureSticky).observe($(".topbar"));
$$(".jump").forEach(j => new ResizeObserver(measureSticky).observe(j));

// ---------- routing ----------

const Views = {};
let currentView = "";
function route() {
  if (location.hash.startsWith("#services")) { location.replace("#system/services"); return; }
  const view = (location.hash.slice(1) || "dashboard").split("/")[0];
  const known = Views[view] ? view : "dashboard";
  $$(".view").forEach(v => { v.hidden = v.id !== "view-" + known; });
  $$(".tabs a").forEach(a => a.classList.toggle("is-active", a.dataset.view === known));
  currentView = known;
  updateSavebar();
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
    setLanguage(I18N.pick(H("settings")["dashboard.language"]));
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
  $(".conn-text", el).textContent = t(s);
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
    if (currentView === "updates") renderUpdates();
    if (currentView === "system") renderSystemFacts();
  });
}

// ---------- formatting helpers ----------

const has = (v) => v !== undefined && v !== null && v !== "";
const num = (v) => { const n = Number(v); return isFinite(n) ? n : null; };
function human(s) {
  if (!has(s)) return "";
  s = String(s).replace(/[-_]/g, " ").replace(/\bsim\b/i, "SIM").replace(/\busb\b/i, "USB");
  return t(s.charAt(0).toUpperCase() + s.slice(1));
}
const fmtV = (mv) => { const n = num(mv); return n === null ? null : (n / 1000).toFixed(2) + " V"; };
const fmtKm = (m) => { const n = num(m); return n === null ? null : (n / 1000).toFixed(1) + " km"; };
const fmtTemp = (c) => { const n = num(c); return n === null ? null : `${Math.round(n)} °C`; };
const fmtPct = (p) => { const n = num(p); return n === null ? null : `${Math.round(n)} %`; };
function ago(iso) {
  const ts = Date.parse(iso);
  if (!isFinite(ts)) return "";
  const s = Math.round((Date.now() - ts) / 1000);
  if (s < 5) return t("just now");
  if (s < 90) return t("{n} s ago", { n: s });
  if (s < 5400) return t("{n} min ago", { n: Math.round(s / 60) });
  if (s < 48 * 3600) return t("{n} h ago", { n: Math.round(s / 3600) });
  if (s < 30 * 86400) return t("{n} d ago", { n: Math.round(s / 86400) });
  return new Date(ts).toLocaleDateString();
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
// SIM states share words with the handlebar lock; keep them apart for translation.
const simState = (v) => v === "locked" ? t("SIM locked") : v === "ready" ? t("SIM ready") : v === "missing" ? t("SIM missing") : undefined;
const status = (v, label) => has(v) ? `<span class="status ${tone(v)}">${esc(label ?? human(v))}</span>` : null;

// Facts: array of [label, valueHTML|null, asideText?]; null values are skipped.
function renderFacts(el, rows) {
  const out = rows.filter(r => r && has(r[1])).map(([label, value, aside]) =>
    `<dt>${esc(label)}</dt><dd>${value}${has(aside) ? `<span class="aside">${esc(aside)}</span>` : ""}</dd>`);
  el.innerHTML = out.join("") || `<div class="empty">${t("No data yet.")}</div>`;
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
    sw.textContent = t("Not connected");
    sw.className = "state-word is-bad";
  }
  // Under the state word: the firmware line, then status chips that only
  // appear when they say something (alarm state, power manager not idle).
  const chips = [];
  if (has(alarm.status)) {
    const st = alarm.status;
    const tone = /trigger/.test(st) ? "is-bad" : /armed$/.test(st) ? "is-good" : "";
    chips.push(`<span class="status ${tone}">${t("Alarm")} ${esc(human(st).toLowerCase())}</span>`);
  }
  if (has(pm.state) && pm.state !== "running") chips.push(`<span class="status is-warn">${t("Power manager")} ${esc(human(pm.state).toLowerCase())}</span>`);
  if (sys["usb0-gate"] === "open") chips.push(`<span class="status is-good">${t("USB link held on")}</span>`);
  const nFaults = Object.values(state.faults).reduce((n, l) => n + l.length, 0);
  if (nFaults) chips.push(`<a class="status is-bad" href="#dashboard/faults">${nFaults === 1 ? t("1 fault") : t("{n} faults", { n: nFaults })}</a>`);
  $("#hero-sub").innerHTML = `${has(vmdb.pretty_name) ? `<div class="hero-version">${esc(vmdb.pretty_name)}</div>` : ""}${chips.length ? `<div class="hero-chips">${chips.join("")}</div>` : ""}`;

  // Batteries.
  const batts = [];
  for (const id of ["0", "1"]) {
    const b = H("battery:" + id);
    if (!Object.keys(b).length) continue;
    const present = b.present !== "false";
    const temps = [0, 1, 2, 3].map(i => num(b["temperature:" + i])).filter(t => t !== null);
    const meta = [fmtV(b.voltage), temps.length ? `${Math.max(...temps)} °C` : null,
      has(b["state-of-health"]) ? `${t("health")} ${b["state-of-health"]} %` : null, has(b["cycle-count"]) ? t("{n} cycles", { n: b["cycle-count"] }) : null];
    batts.push(battRow(t("Battery {n}", { n: Number(id) + 1 }), present ? num(b.charge) : null, present ? b.state : "not present", present ? meta : [], !present));
  }
  const aux = H("aux-battery");
  if (Object.keys(aux).length) {
    batts.push(battRow(t("AUX (12V)"), num(aux.charge), aux["charge-status"], [fmtV(aux.voltage)], false, true));
  }
  const cbb = H("cb-battery");
  if (Object.keys(cbb).length) {
    const present = cbb.present !== "false";
    batts.push(battRow(t("Connectivity (CBB)"), present ? num(cbb.charge) : null, present ? cbb["charge-status"] : "not present",
      present ? [fmtTemp(cbb.temperature), has(cbb["state-of-health"]) ? `${t("health")} ${cbb["state-of-health"]} %` : null] : [], !present));
  }
  $("#hero-batteries").innerHTML = batts.join("") || `<div class="muted">${t("No battery data.")}</div>`;

  // Vehicle facts.
  const ecuTemp = fmtTemp(ecu.temperature);
  renderFacts($("#facts-vehicle"), [
    [t("Odometer"), esc(fmtKm(ecu.odometer))],
    [t("Speed"), has(ecu.speed) ? `${esc(ecu.speed)} km/h` : null, has(ecu.rpm) && num(ecu.rpm) > 0 ? `${ecu.rpm} rpm` : null],
    [t("Motor"), status(ecu["ecu-status"]), has(ecu["motor:voltage"]) ? fmtV(ecu["motor:voltage"]) : null],
    [t("Controller temperature"), ecuTemp ? esc(ecuTemp) : null],
    [t("Handlebar"), status(v["handlebar:lock-state"]), has(v["handlebar:position"]) ? human(v["handlebar:position"]) : null],
    [t("Seatbox"), status(v["seatbox:lock"])],
    [t("Kickstand"), status(v.kickstand)],
    [t("Display"), status(v["dashboard:power"], v["dashboard:power"] === "on" ? t("On") : t("Off"))],
    [t("Blinkers"), has(v["blinker:state"]) ? esc(human(v["blinker:state"])) : null],
    [t("Keycards"), has(sys["keycard-authorized-count"]) ? `${esc(sys["keycard-authorized-count"])} ${t("authorized")}` : null,
      has(sys["keycard-master-count"]) ? `${sys["keycard-master-count"]} ${t("master")}` : null],
    [t("Energy"), has(ecu["energy:consumed"]) ? `${(num(ecu["energy:consumed"]) / 1000).toFixed(1)} kWh ${t("used")}` : null,
      has(ecu["energy:recovered"]) ? `${(num(ecu["energy:recovered"]) / 1000).toFixed(1)} kWh ${t("recovered")}` : null],
  ]);

  // Connectivity.
  const coord = (has(gps.latitude) && has(gps.longitude)) ? `${num(gps.latitude).toFixed(5)}, ${num(gps.longitude).toFixed(5)}` : null;
  const gpsState = gps.state === "fix-established" ? `${t("Fix")} (${gps.fix || "3d"})` : human(gps.state);
  renderFacts($("#facts-conn"), [
    [t("Internet"), status(net.status), [net["access-tech"], has(net["signal-quality"]) ? `${t("signal")} ${net["signal-quality"]} %` : null].filter(Boolean).join(", ")],
    [t("Mobile network"), has(modem["operator-name"]) ? esc(modem["operator-name"]) : status(modem["power-state"]),
      [modem.registration, modem["error-state"] && modem["error-state"] !== "ok" ? modem["error-state"] : null].filter(Boolean).map(human).join(", ")],
    [t("SIM"), status(modem["sim-state"], simState(modem["sim-state"])), has(net["sim-iccid"]) ? `ICCID ${net["sim-iccid"]}` : null],
    [t("IP address"), has(net["ip-address"]) ? `<span class="mono">${esc(net["ip-address"])}</span>` : null],
    [t("GPS"), status(gps.state, gpsState),
      [has(gps["satellites-used"]) ? t("{used} of {visible} satellites", { used: gps["satellites-used"], visible: gps["satellites-visible"] || "?" }) : null, has(gps.updated) ? `${t("updated")} ${ago(gps.updated)}` : null].filter(Boolean).join(", ")],
    [t("Position"), coord ? `<span class="mono">${esc(coord)}</span>` : null,
      [has(gps.altitude) ? `${t("altitude")} ${Math.round(num(gps.altitude))} m` : null, has(gps.speed) && num(gps.speed) >= 1 ? `${num(gps.speed).toFixed(0)} km/h` : null].filter(Boolean).join(", ")],
    [t("Bluetooth"), has(sys["nrf-fw-version"]) ? t("Module present") : null],
  ]);

  // Faults.
  const SRC = { "vehicle": t("Vehicle"), "engine-ecu": t("Motor controller"), "battery:0": t("Battery {n}", { n: 1 }), "battery:1": t("Battery {n}", { n: 2 }) };
  const rows = [];
  for (const [src, list] of Object.entries(state.faults)) for (const f of list) rows.push([SRC[src] || src, f]);
  $("#faults").innerHTML = rows.length
    ? rows.map(([s, f]) => `<div class="fault"><span class="src">${esc(s)}</span><code>${esc(f)}</code></div>`).join("")
    : `<div class="none">${t("None.")}</div>`;

  syncCommandAvailability();
}

function battRow(name, pct, stateText, meta, absent = false, coarse = false) {
  const p = pct === null ? null : Math.max(0, Math.min(100, pct));
  const barTone = p === null ? "" : p <= 10 ? "is-bad" : p <= 25 ? "is-warn" : "";
  const val = p === null ? esc(human(stateText)) : `${Math.round(p)}<small>%${coarse ? ` ${t("approx.")}` : ""}</small>`;
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
      const when = isFinite(ms) ? new Date(ms).toLocaleString() : ev.id;
      return `<tr><td>${esc(when)}</td><td><code>${esc(ev.code || "")}</code><div class="sub">${esc(ev.group || "")}${ev.description ? ": " + esc(ev.description) : ""}</div></td></tr>`;
    }).join("") || `<tr><td class="muted" colspan="2">${t("None since boot.")}</td></tr>`;
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
    notify(CMD_LABEL[cmd] ? t(CMD_LABEL[cmd]) : t("{cmd}: sent", { cmd }));
  } catch (err) { notify(err.message, true); }
  finally { btn.disabled = false; syncCommandAvailability(); }
}

function attachHold(btn, duration) {
  const bar = document.createElement("span"); bar.className = "hold-bar"; bar.setAttribute("aria-hidden", "true");
  const tip = document.createElement("span"); tip.className = "hold-tip"; tip.setAttribute("aria-hidden", "true"); tip.textContent = t("Hold to confirm");
  btn.append(bar, tip);
  btn.setAttribute("aria-description", t("Hold for {n} seconds to confirm", { n: (duration / 1000).toFixed(1).replace(/\.0$/, "") }));
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
    tip.textContent = p >= 1 ? t("Sending") : p >= 0.5 ? t("Keep holding") : t("Hold to confirm");
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
  const stop = () => { if (startedAt !== null) { reset(); tip.textContent = t("Hold to confirm"); } };

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
  $$("#dash-commands [data-cmd]").forEach(b => { if (!b.classList.contains("is-busy")) b.disabled = !live; });

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
  parts.push(t("Idle target: {state}.", { state: settings["pm.default-state"] || "suspend" }));
  if (idle !== null) parts.push(idle > 0 ? t("Inactivity hibernation after {d}.", { d: humanDuration(idle) }) : t("Inactivity hibernation off."));
  parts.push(sched ? t("Scheduled hibernation on ({cron}, wake after {d}).", { cron: settings["pm.scheduled-hibernate-cron"] || "?", d: settings["pm.scheduled-hibernate-duration"] || "?" }) : t("Scheduled hibernation off."));
  if (armed) parts.push(wakeSecs ? t("A wake timer is armed for {d}.", { d: humanDuration(wakeSecs) }) : t("A wake timer is armed."));
  if (has(pm.state) && pm.state !== "running") parts.unshift(`${t("Power manager")}: ${human(pm.state)}.`);
  $("#power-hint").innerHTML = `${esc(parts.join(" "))} <a href="#settings/pm-service">${t("Change in Settings")}</a>`;
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
  } catch (err) { notify(t("Settings unavailable: {error}", { error: err.message }), true); }
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
    container.innerHTML = `<div class="settings-empty">${t("Nothing matches.")}${advanced ? "" : " " + t("Advanced settings are hidden.")}</div>`;
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
  const line = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--sticky-h")) + 12 || 120;
  const mark = (sections, prefix, nav) => {
    if (!sections.length) return;
    let current = sections[0];
    for (const g of sections) if (g.getBoundingClientRect().top <= line) current = g;
    const id = current.id.replace(prefix, "");
    $$("a", nav).forEach(a => a.classList.toggle("is-current", a.dataset.jump === id));
  };
  if (currentView === "settings") mark($$("#settings-groups .group"), /^group-/, $("#settings-jump"));
  if (currentView === "dashboard") mark($$("#view-dashboard [id^=dash-]"), /^dash-/, $("#view-dashboard .jump"));
  if (currentView === "system") mark($$("#view-system [id^=sys-]").filter(el => el.tagName !== "DL"), /^sys-/, $("#view-system .jump"));
}
window.addEventListener("scroll", markCurrentGroup, { passive: true });
// Every .jump strip scrolls to <prefix><id>; the hash keeps the target so
// deep links like #system/services land on the section.
document.addEventListener("click", e => {
  const a = e.target.closest(".jump [data-jump]");
  if (!a) return;
  e.preventDefault();
  const el = document.getElementById(a.closest(".jump").dataset.prefix + a.dataset.jump);
  if (el) el.scrollIntoView({ behavior: "smooth", block: "start" });
});
function scrollToHashSection(view, prefix) {
  const target = decodeURIComponent(location.hash.split("/")[1] || "");
  if (target) document.getElementById(prefix + target)?.scrollIntoView({ block: "start" });
}

function currentValue(key) {
  return dirty.has(key) ? dirty.get(key) : (values[key] ?? "");
}

function settingRowEl(key, m) {
  const row = document.createElement("div");
  row.className = "setting" + (dirty.has(key) ? " is-changed" : "") + (failures[key] ? " is-failed" : "");
  row.dataset.key = key;
  const cur = currentValue(key);
  const def = m.default === null || m.default === undefined ? "" : String(m.default);
  const tags = [m.transient ? `<span class="tag" title="${t("Kept in memory only, reset on reboot")}">${t("until reboot")}</span>` : "",
    m["read-only"] ? `<span class="tag">${t("read only")}</span>` : ""].join("");
  let control;
  const id = "s-" + key.replace(/[^a-z0-9]/gi, "-");
  if (m["read-only"]) {
    control = `<div class="setting-ro">${esc(cur || def || t("(not set)"))}</div>`;
  } else if (m.type === "bool") {
    const on = cur === "" ? def === "true" : cur === "true";
    control = `<div class="row"><label class="switch"><input type="checkbox" id="${id}" data-key="${esc(key)}" ${on ? "checked" : ""}><span class="track"></span></label><label for="${id}" class="muted">${on ? t("On") : t("Off")}</label></div>`;
  } else if (m.type === "enum") {
    const opts = (m.values || []).map(v => `<option value="${esc(v.value)}" ${v.value === cur ? "selected" : ""}>${esc(v.label || v.value)}</option>`).join("");
    const defLabel = (m.values || []).find(v => v.value === def);
    control = `<div class="row"><select id="${id}" data-key="${esc(key)}"><option value="" ${cur === "" ? "selected" : ""}>${esc(def !== "" ? t("Default ({value})", { value: defLabel ? defLabel.label || defLabel.value : def }) : t("Not set"))}</option>${opts}</select></div>`;
  } else if (m.type === "int" || m.type === "float") {
    const attrs = [has(m.min) ? `min="${m.min}"` : "", has(m.max) ? `max="${m.max}"` : "", `step="${m.type === "int" ? 1 : "any"}"`].join(" ");
    control = `<div class="row"><input type="number" id="${id}" data-key="${esc(key)}" value="${esc(cur)}" placeholder="${esc(def)}" ${attrs}>${m.unit ? `<span class="unit">${esc(m.unit)}</span>` : ""}</div>`;
  } else {
    control = `<div class="row"><input type="text" id="${id}" data-key="${esc(key)}" value="${esc(cur)}" placeholder="${esc(def || m.example || "")}" spellcheck="false"></div>`;
  }
  const range = (m.type === "int" || m.type === "float") && (has(m.min) || has(m.max)) ? ` (${has(m.min) ? m.min : t("any")} ${t("to")} ${has(m.max) ? m.max : t("any")})` : "";
  const defLine = m["read-only"] ? "" :
    `<div class="setting-default">${def !== "" ? `${t("Default")} ${esc(def)}${esc(range)}` : `${t("No default")}${esc(range)}`}${(cur !== "" && !m["read-only"]) ? ` <a href="#" data-reset="${esc(key)}">${t("Reset")}</a>` : ""}</div>`;
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
      if (input.type === "checkbox") $(`label[for="${id}"].muted`, row).textContent = input.checked ? t("On") : t("Off");
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
  $("#savebar-text").textContent = n === 1 ? t("1 change") : t("{n} changes", { n });
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
    if (nf) notify(t("{saved} saved, {rejected} rejected", { saved: na, rejected: nf }), true);
    else notify(na === 1 ? t("Saved 1 setting") : t("Saved {n} settings", { n: na }));
    renderSettings();
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});
$("#settings-revert").addEventListener("click", () => { dirty.clear(); failures = {}; renderSettings(); });
$("#settings-filter").addEventListener("input", debounce(renderSettings, 150));
$("#settings-advanced").addEventListener("change", renderSettings);

function debounce(fn, ms) { let timer; return (...a) => { clearTimeout(timer); timer = setTimeout(() => fn(...a), ms); }; }

// ---------- updates ----------

const upd = { data: null, chosen: {} };

Views.updates = async function () {
  try { upd.data = await API.get("/api/updates"); renderUpdates(); } catch (err) { notify(err.message, true); }
};

const OTA_STATUS = { idle: ["Idle", ""], downloading: ["Downloading", "is-info"], preparing: ["Preparing", "is-info"], installing: ["Installing", "is-info"], "pending-reboot": ["Installed, waiting for reboot", "is-warn"], error: ["Error", "is-bad"] };

function channelOf(board) {
  const st = upd.data.settings[`updates.${board}.channel`];
  if (st) return st;
  const v = (upd.data.versions[board] || {}).version_id || "";
  const m = v.match(/^(stable|testing|nightly)/);
  return m ? m[1] : "";
}

function renderUpdates() {
  const d = upd.data; if (!d) return;
  const ota = state.hashes.ota || d.ota || {};
  const NAMES = { mdb: "MDB", dbc: t("Display (DBC)") };
  $("#upd-boards").innerHTML = ["mdb", "dbc"].map(b => {
    const v = d.versions[b] || {};
    const st = ota[`status:${b}`] || "";
    const [label, tone] = OTA_STATUS[st] ? [t(OTA_STATUS[st][0]), OTA_STATUS[st][1]] : [human(st) || t("Unknown"), ""];
    const busy = ["downloading", "preparing", "installing"].includes(st);
    const dl = busy && has(ota[`download-progress:${b}`]) ? num(ota[`download-progress:${b}`]) : null;
    const inst = busy && has(ota[`install-progress:${b}`]) ? num(ota[`install-progress:${b}`]) : null;
    const err = ota[`error:${b}`];
    const preview = ota[`preview-status:${b}`];
    const ch = channelOf(b);
    const lastCheck = d.settings[`updates.${b}.last-check-time`];
    const rows = [
      [t("Installed"), has(v.version) ? esc(v.version) : `<span class="muted">${t("unknown")}</span>`],
      [t("Status"), `<span class="status ${tone}">${esc(label)}</span>`, has(ota[`update-version:${b}`]) && st !== "idle" ? `${ota[`update-version:${b}`]}${has(ota[`update-method:${b}`]) ? `, ${ota[`update-method:${b}`]}` : ""}` : null],
      err ? [t("Error"), `<span class="status is-bad">${esc(human(err))}</span>`, ota[`error-message:${b}`] || null] : null,
      has(ota[`download-abort-reason:${b}`]) ? [t("Download"), `<span class="muted">${t("paused")}: ${esc(human(ota[`download-abort-reason:${b}`]))}</span>`, has(ota[`download-skip-checks:${b}`]) ? t("retries after {n} more checks", { n: ota[`download-skip-checks:${b}`] }) : null] : null,
      [t("Last check"), has(lastCheck) ? esc(ago(lastCheck)) : `<span class="muted">${t("never")}</span>`, d.settings[`updates.${b}.check-interval`] === "0s" ? t("automatic checks off") : has(d.settings[`updates.${b}.check-interval`]) ? t("every {interval}", { interval: d.settings[`updates.${b}.check-interval`] }) : null],
    ];
    const bar = (label, p, warn) => p === null ? "" : `<div class="muted" style="font-size:.85rem">${label} ${p} %</div><div class="progress ${warn ? "is-warn" : ""}"><span style="width:${p}%"></span></div>`;
    const chosen = upd.chosen[b] || ch;
    const pc = ota[`preview-channel:${b}`];
    const previewFor = pc === chosen ? preview : "";
    const previewLine = chosen === ch ? ""
      : previewFor === "checking" ? `<span class="muted">${t("Looking up {channel}", { channel: esc(chosen) })}</span>`
      : previewFor === "ready" ? t("{channel} has {version}{size}", { channel: esc(chosen), version: `<span class="mono">${esc(ota[`preview-version:${b}`])}</span>`, size: has(ota[`preview-size:${b}`]) ? t(", a {size} full download", { size: esc(humanSize(num(ota[`preview-size:${b}`]))) }) : "" })
      : previewFor === "unavailable" ? `<span class="muted">${t("{channel} has nothing for this board", { channel: esc(chosen) })}</span>`
      : previewFor === "error" ? `<span class="status is-bad">${t("Could not read the {channel} channel", { channel: esc(chosen) })}</span>`
      : `<span class="muted">${t("Looking up {channel}", { channel: esc(chosen) })}</span>`;
    return `<section class="block board" data-board="${b}">
      <h2>${NAMES[b]}</h2>
      <dl class="facts">${rows.filter(r => r && has(r[1])).map(([l, val, aside]) => `<dt>${esc(l)}</dt><dd>${val}${has(aside) ? `<span class="aside">${esc(aside)}</span>` : ""}</dd>`).join("")}</dl>
      ${bar(t("Download"), dl, false)}${bar(t("Install"), inst, false)}
      <div class="cmd-row">
        <button type="button" class="btn" data-upd="check" data-board="${b}">${t("Check now")}</button>
        <label class="channel-pick">${t("Channel")}
          <select data-upd-channel="${b}" aria-label="${t("Release channel")}">${["stable", "testing", "nightly"].map(c => `<option value="${c}" ${c === chosen ? "selected" : ""}>${c}${c === ch ? ` (${t("current")})` : ""}</option>`).join("")}</select>
        </label>
        ${chosen !== ch && previewFor === "ready" ? `<button type="button" class="btn btn-primary" data-upd="switch" data-board="${b}">${t("Switch to {channel} and update now", { channel: esc(chosen) })}</button>` : ""}
      </div>
      ${previewLine ? `<p class="cmd-hint">${previewLine}</p>` : ""}
    </section>`;
  }).join("");

  const files = d.files || {};
  $("#upd-files").innerHTML = ["mdb", "dbc"].map(b => {
    const list = files[b] || [];
    if (!list.length) return "";
    return `<div class="upd-group"><h3>${NAMES[b]}</h3>${list.map(f => `<div class="upd-row">
      <span class="fname">${esc(f.name)}</span>
      <span class="row-actions">
        <button type="button" class="btn btn-small" data-upd="install" data-board="${b}" data-file="${esc(f.name)}">${t("Install")}</button>
        <button type="button" class="btn btn-small btn-quiet" data-upd="delete" data-board="${b}" data-file="${esc(f.name)}">${t("Delete")}</button>
      </span>
      <span class="fmeta">${esc(humanSize(f.size))}, ${esc(new Date(f.mtime * 1000).toLocaleString())}</span>
    </div>`).join("")}</div>`;
  }).join("") || `<p class="cmd-hint">${t("No update files on the scooter.")}</p>`;
}

$("#view-updates").addEventListener("change", async e => {
  const sel = e.target.closest("[data-upd-channel]");
  if (!sel) return;
  const board = sel.dataset.updChannel;
  upd.chosen[board] = sel.value;
  renderUpdates();
  if (sel.value !== channelOf(board)) {
    try { await API.post("/api/updates/action", { board, action: "preview", channel: sel.value }); } catch (err) { notify(err.message, true); }
  }
});

$("#view-updates").addEventListener("click", async e => {
  const btn = e.target.closest("[data-upd]");
  if (!btn) return;
  const { upd: action, board, file } = btn.dataset;
  const body = { board, action };
  if (action === "switch") body.channel = upd.chosen[board];
  if (action === "install" || action === "delete") body.file = file;
  if (action === "switch") {
    const ok = await confirmDialog({ title: t("Switch {board} to {channel}", { board: board.toUpperCase(), channel: body.channel }), body: t("The full {channel} image is downloaded and installed now. That takes a while and, for the MDB, ends in a reboot.", { channel: body.channel }), ok: t("Switch and update") });
    if (!ok) return;
    btn.classList.add("is-busy");
    try {
      await API.post("/api/updates/action", { board, action: "channel", channel: body.channel });
      await API.post("/api/updates/action", { board, action: "check" });
      notify(t("{board} switching to {channel}", { board: board.toUpperCase(), channel: body.channel }));
      delete upd.chosen[board];
      Views.updates();
    } catch (err) { notify(err.message, true); }
    finally { btn.classList.remove("is-busy"); }
    return;
  }
  if (action === "install") {
    const ok = await confirmDialog({ title: t("Install on {board}", { board: board.toUpperCase() }), body: board === "dbc"
      ? t("{file} is copied to the display (switched on if needed) and installed. It takes effect the next time the display powers up.", { file })
      : t("{file} is installed now. The MDB reboots when the installation is done and this page comes back after that.", { file }), ok: t("Install"), danger: board === "mdb" });
    if (!ok) return;
  }
  if (action === "delete") {
    const ok = await confirmDialog({ title: t("Delete file"), body: t("Delete {name}?", { name: file }), ok: t("Delete"), danger: true });
    if (!ok) return;
  }
  btn.classList.add("is-busy");
  try {
    const res = await API.post("/api/updates/action", body);
    notify(t({ check: "Checking for updates", install: "Update queued", delete: "File deleted" }[action]));
    if (res && res.status) Views.updates();
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});

$("#upd-upload-form").addEventListener("submit", e => {
  e.preventDefault();
  const file = $("#upd-upload-file").files[0];
  if (!file) return notify(t("Choose a file first"), true);
  if (!/\.(mender|delta)$/.test(file.name)) return notify(t("Only .mender and .delta files"), true);
  const board = $("#upd-upload-board").value;
  const prog = $("#upd-upload-progress"), bar = $("span", prog);
  prog.hidden = false; bar.style.width = "0%";
  const xhr = new XMLHttpRequest();
  xhr.open("PUT", `/api/updates/upload?board=${board}&name=${encodeURIComponent(file.name)}`);
  for (const [k, v] of Object.entries(API.headers(false))) xhr.setRequestHeader(k, v);
  xhr.upload.onprogress = ev => { if (ev.lengthComputable) bar.style.width = `${Math.round(ev.loaded / ev.total * 100)}%`; };
  xhr.onload = () => {
    prog.hidden = true;
    let data = {}; try { data = JSON.parse(xhr.responseText); } catch { /* not json */ }
    if (xhr.status >= 200 && xhr.status < 300) { notify(t("Uploaded {name}", { name: file.name })); $("#upd-upload-file").value = ""; Views.updates(); }
    else notify(data.error || t("Upload failed (HTTP {status})", { status: xhr.status }), true);
  };
  xhr.onerror = () => { prog.hidden = true; notify(t("Upload failed"), true); };
  xhr.send(file);
});

// ---------- system ----------

Views.system = async function () {
  renderSystemFacts();
  try {
    const data = await API.get("/api/system/logs");
    renderBundles(data.bundles || []);
  } catch (err) { notify(err.message, true); }
  await loadServices();
  const sel = $("#journal-unit");
  if (sel.options.length <= 2) {
    for (const u of units) sel.insertAdjacentHTML("beforeend", `<option value="${esc(u.unit)}">${esc(u.unit.replace(/\.service$/, ""))}</option>`);
  }
  scrollToHashSection("system", "sys-");
};

function renderSystemFacts() {
  const m = H("maps"), mo = H("modem"), net = H("internet");
  const vmdb = H("version:mdb"), vdbc = H("version:dbc"), ecu = H("engine-ecu"), sys = H("system");
  const boards = [
    [t("MDB"), vmdb.version || vmdb.version_id, vmdb.serial_number_real],
    [t("Display"), vdbc.version || vdbc.version_id, vdbc.serial_number_real],
    [t("Motor controller"), has(ecu["fw-version"]) ? `${t("firmware")} ${ecu["fw-version"]}` : null, null],
    [t("Bluetooth module"), has(sys["nrf-fw-version"]) ? `${t("firmware")} ${sys["nrf-fw-version"]}` : null, null],
  ].filter(r => has(r[1]));
  $("#boards tbody").innerHTML = boards.map(([n, ver, sn]) =>
    `<tr><td>${esc(n)}</td><td>${esc(ver)}${sn ? `<div class="serial">${esc(sn)}</div>` : ""}</td></tr>`).join("")
    || `<tr><td class="muted">${t("Not reported yet.")}</td></tr>`;


  const art = (p) => has(m[`${p}:size`])
    ? `${esc(humanSize(num(m[`${p}:size`])))}${has(m[`${p}:published-at`]) ? `, ${t("published")} ${esc(m[`${p}:published-at`].slice(0, 10))}` : ""}`
    : `<span class="muted">${t("not installed")}</span>`;
  renderFacts($("#sys-maps-facts"), [
    [t("Region"), has(m["region-name"]) ? esc(m["region-name"]) : has(m.region) ? esc(m.region) : null],
    [t("Map tiles"), art("map"), has(m["map:mtime"]) ? `${t("written")} ${m["map:mtime"].slice(0, 10)}` : null],
    [t("Routing tiles"), art("routing"), has(m["routing:mtime"]) ? `${t("written")} ${m["routing:mtime"].slice(0, 10)}` : null],
    [t("Update"), has(m["update-available"]) ? (m["update-available"] === "true" ? `<span class="status is-info">${t("Available")}</span>` : t("Up to date")) : null, has(m["last-update-check"]) ? `${t("checked")} ${ago(m["last-update-check"])}` : null],
  ]);
  renderFacts($("#sys-modem-facts"), [
    [t("Modem"), status(mo["power-state"], mo["power-state"] === "on" ? t("On") : human(mo["power-state"])), has(mo["error-state"]) && mo["error-state"] !== "ok" ? human(mo["error-state"]) : null],
    [t("Network"), has(mo["operator-name"]) ? esc(mo["operator-name"]) : null, [mo["operator-code"], human(mo.registration), mo["is-roaming"] === "true" ? t("roaming") : null].filter(Boolean).join(", ")],
    [t("Connection"), status(net.status), [net["access-tech"], has(net["signal-quality"]) ? `${t("signal")} ${net["signal-quality"]} %` : null].filter(Boolean).join(", ")],
    [t("IP address"), has(net["ip-address"]) ? `<span class="mono">${esc(net["ip-address"])}</span>` : null],
    [t("SIM"), status(mo["sim-state"], simState(mo["sim-state"])), [mo["sim-lock"] && mo["sim-lock"] !== "disabled" ? `${t("lock")} ${mo["sim-lock"]}` : null, mo["pin-action"] && mo["pin-action"] !== "unconfigured" ? `PIN ${human(mo["pin-action"]).toLowerCase()}` : null].filter(Boolean).join(", ")],
    [t("IMEI"), has(net["sim-imei"]) ? `<span class="mono">${esc(net["sim-imei"])}</span>` : null],
    [t("ICCID"), has(net["sim-iccid"]) ? `<span class="mono">${esc(net["sim-iccid"])}</span>` : null],
    [t("IMSI"), has(net["sim-imsi"]) ? `<span class="mono">${esc(net["sim-imsi"])}</span>` : null],
    [t("Health"), has(net["modem-health"]) ? esc(human(net["modem-health"])) : null, [net.reachability ? `${t("reachability")} ${net.reachability}` : null, net["link-layer"] ? `${t("link")} ${net["link-layer"]}` : null].filter(Boolean).join(", ")],
  ]);
}

function renderBundles(list) {
  $("#log-bundles").innerHTML = list.length ? list.map(b => `<div class="upd-row">
    <a class="fname" href="${esc(API.url(`/files/log-bundles/${encodeURIComponent(b.name)}`, { download: "1" }))}">${esc(b.name)}</a>
    <span class="row-actions"><a class="btn btn-small" href="${esc(API.url(`/files/log-bundles/${encodeURIComponent(b.name)}`, { download: "1" }))}">${t("Download")}</a>
      <button type="button" class="btn btn-small btn-quiet" data-bundle-del="${esc(b.name)}">${t("Delete")}</button></span>
    <span class="fmeta">${esc(humanSize(b.size))}, ${esc(new Date(b.mtime * 1000).toLocaleString())}</span>
  </div>`).join("") : `<p class="cmd-hint">${t("No bundles yet.")}</p>`;
}

$("#log-bundle-form").addEventListener("submit", async e => {
  e.preventDefault();
  const btn = $("button", e.target);
  btn.classList.add("is-busy");
  try {
    const res = await API.post("/api/system/logs", { since: $("#log-since").value });
    renderBundles(res.bundles || []);
    notify(res.bundle ? t("Created {name}", { name: res.bundle }) : t("Bundle created"));
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});
$("#log-bundles").addEventListener("click", async e => {
  const del = e.target.closest("[data-bundle-del]");
  if (!del) return;
  const ok = await confirmDialog({ title: t("Delete bundle"), body: t("Delete {name}?", { name: del.dataset.bundleDel }), ok: t("Delete"), danger: true });
  if (!ok) return;
  try { await API.del(`/api/files?path=${encodeURIComponent("log-bundles/" + del.dataset.bundleDel)}`); Views.system(); }
  catch (err) { notify(err.message, true); }
});
$("#journal-form").addEventListener("submit", async e => {
  e.preventDefault();
  const unit = $("#journal-unit").value, lines = $("#journal-lines").value;
  const out = $("#journal-out");
  const btn = $("button", e.target);
  btn.classList.add("is-busy");
  try {
    const resp = await fetch(API.url("/api/system/journal", { unit, lines }), { headers: API.headers(false) });
    const text = await resp.text();
    if (!resp.ok) throw new Error(text || `HTTP ${resp.status}`);
    out.hidden = false; out.textContent = text || t("(empty)");
    out.scrollTop = out.scrollHeight;
    const dl = $("#journal-download");
    dl.hidden = false; dl.href = API.url("/api/system/journal", { unit, lines }); dl.download = `${unit || "journal"}.log`;
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});

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
  if (!set) { el.className = "nav-current muted"; el.textContent = t("None."); return; }
  el.className = "nav-current";
  el.innerHTML = `${has(d.address) ? `<div class="name">${esc(d.address)}</div>` : ""}<div class="coords">${esc(fmtCoord(d.latitude, d.longitude))}</div>${has(d.timestamp) ? `<div class="when">${t("set")} ${esc(ago(d.timestamp))}</div>` : ""}`;
}

function renderLocations() {
  const el = $("#nav-locations");
  if (!nav.locations.length) { el.innerHTML = `<div class="kc-empty">${t("None saved.")}</div>`; return; }
  el.innerHTML = nav.locations.map(l => {
    const editing = nav.editing === l.id;
    return `<div class="loc" data-id="${l.id}">
      <div><span class="name">${esc(l.label || t("Unnamed"))}</span>${l["last-used-at"] ? `<span class="when">${t("last used")} ${esc(ago(l["last-used-at"]))}</span>` : ""}</div>
      <span class="row-actions">
        <button type="button" class="btn btn-small btn-quiet" data-loc-go="${l.id}">${t("Navigate")}</button>
        <button type="button" class="btn btn-small btn-quiet" data-loc-edit="${l.id}">${editing ? t("Cancel") : t("Edit")}</button>
        <button type="button" class="btn btn-small btn-quiet" data-loc-del="${l.id}">${t("Delete")}</button>
      </span>
      <div class="coords">${esc(fmtCoord(l.latitude, l.longitude))}</div>
      ${editing ? `<form class="loc-edit" data-loc-form="${l.id}">
        <input class="label" value="${esc(l.label)}" placeholder="${t("Name")}" aria-label="${t("Name")}" required>
        <input value="${l.latitude.toFixed(6)}" placeholder="${t("Latitude")}" aria-label="${t("Latitude")}" inputmode="decimal" required>
        <input value="${l.longitude.toFixed(6)}" placeholder="${t("Longitude")}" aria-label="${t("Longitude")}" inputmode="decimal" required>
        <button type="submit" class="btn btn-small btn-primary">${t("Save")}</button>
      </form>` : ""}
    </div>`;
  }).join("");
}

function readCoords(latEl, lonEl) {
  const lat = Number(latEl.value.trim().replace(",", ".")), lon = Number(lonEl.value.trim().replace(",", "."));
  if (!isFinite(lat) || !isFinite(lon) || latEl.value.trim() === "" || lonEl.value.trim() === "") throw new Error(t("Latitude and longitude are needed, as decimal degrees"));
  return { latitude: lat, longitude: lon };
}

async function navigateTo(latitude, longitude, address, locationId) {
  await API.post("/api/navigation", { latitude, longitude, address: address || "", "location-id": locationId ?? null });
  notify(address ? t("Navigating to {name}", { name: address }) : t("Destination set"));
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
    if (!label) return notify(t("Give the location a name first"), true);
    await API.put("/api/navigation/locations", { label, ...c });
    notify(t("Saved {name}", { name: label }));
    $("#nav-form").reset();
    Views.navigation();
  } catch (err) { notify(err.message, true); }
});
$("#nav-use-gps").addEventListener("click", () => {
  const g = H("gps");
  if (!has(g.latitude) || !has(g.longitude) || g.fix === "none") return notify(t("No GPS fix right now"), true);
  $("#nav-lat").value = Number(g.latitude).toFixed(6);
  $("#nav-lon").value = Number(g.longitude).toFixed(6);
});
$("#nav-clear").addEventListener("click", async () => {
  try { await API.post("/api/navigation", { clear: true }); notify(t("Destination cleared")); Views.navigation(); }
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
    const ok = await confirmDialog({ title: t("Delete location"), body: t("Remove {name} from the saved locations?", { name: l.label || t("this location") }), ok: t("Delete"), danger: true });
    if (!ok) return;
    try { await API.del(`/api/navigation/locations?id=${l.id}`); notify(t("Deleted {name}", { name: l.label })); Views.navigation(); }
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
    notify(t("Location saved"));
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
  const row = (uid, kind, extra = "") => `<div class="kc-row ${extra}"><span class="uid">${esc(fmtUID(uid))}</span>${kind ? `<span class="tag">${esc(t(kind))}</span>` : ""}
    ${kind === "master" ? "" : `<button type="button" class="btn btn-small btn-quiet" data-kc-remove="${esc(uid)}" ${kc.authorized.length <= 1 ? `disabled title="${t("The last card stays")}"` : ""}>${t("Remove")}</button>`}</div>`;
  const learned = kc.learned.filter(u => !kc.authorized.includes(u));
  $("#kc-authorized").innerHTML = [
    ...kc.authorized.map(u => row(u, "")),
    ...learned.map(u => row(u, "tapped, unsaved", "is-new")),
  ].join("") || `<div class="kc-empty">${t("No authorized cards yet.")}</div>`;
  $("#kc-master").innerHTML = kc.master.map(u => row(u, "master")).join("") || `<div class="kc-empty">${t("No master card. The next card tapped at the reader becomes one.")}</div>`;

  const learn = $("#kc-learn-start").closest(".kc-learn");
  learn.classList.toggle("is-active", kc.learning === "cards");
  $("#kc-learn-start").hidden = kc.learning === "cards";
  $("#kc-learn-stop").hidden = kc.learning !== "cards";
  $("#kc-learn-hint").textContent = kc.learning === "cards"
    ? (learned.length ? t("Tap cards at the reader, {n} so far.", { n: learned.length }) : t("Tap cards at the reader."))
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
  const verdict = k.authentication === "passed" ? `<span class="status is-good">${t("Accepted")}</span>` : `<span class="status is-bad">${t("Rejected")}</span>`;
  const when = kc.lastSeenAt ? ago(new Date(kc.lastSeenAt).toISOString()) : "";
  el.classList.remove("muted");
  el.innerHTML = `<span class="uid">${esc(fmtUID(k.uid))}</span>${verdict}${has(k.type) ? `<span class="muted">${esc(k.type)} ${t("card")}</span>` : ""}<span class="muted">${esc(when)}</span>
    ${known ? "" : `<button type="button" class="btn btn-small" data-kc-authorize="${esc(k.uid)}">${t("Authorize this card")}</button>`}`;
}

function onKeycardEvent(ev, ts) {
  const [kind, ...rest] = ev.split(":");
  const uid = rest[rest.length - 1];
  switch (kind) {
    case "card-learned": if (!kc.learned.includes(uid)) kc.learned.push(uid); kc.learning = kc.learning || "cards"; notify(t("Tapped {uid}", { uid: fmtUID(uid) })); break;
    case "card-duplicate": notify(t("{uid} is already authorized", { uid: fmtUID(uid) })); break;
    case "mode-entered": if (rest[0] === "master") kc.learning = "master"; break;
    case "mode-exited": if (kc.learning === "master") kc.learning = null; break;
    case "master-learned": notify(t("Master {uid} added", { uid: fmtUID(uid) })); kc.learning = null; Views.keycards(); return;
    case "rejected": notify(t("{uid} is already authorized, so it cannot be a master", { uid: fmtUID(uid) }), true); break;
    case "error": notify(t("Could not save {uid}", { uid: fmtUID(uid) }), true); break;
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
  try { await keycardCommand("add", uid, $("button", e.target)); $("#kc-add-uid").value = ""; notify(t("Card authorized")); }
  catch (err) { notify(err.message, true); }
});
$("#view-keycards").addEventListener("click", async e => {
  const rm = e.target.closest("[data-kc-remove]");
  if (rm) {
    const uid = rm.dataset.kcRemove;
    const ok = await confirmDialog({ title: t("Remove card"), body: t("{uid} will no longer unlock the scooter.", { uid: fmtUID(uid) }), ok: t("Remove"), danger: true });
    if (!ok) return;
    try { await keycardCommand("remove", uid, rm); notify(t("Card removed")); } catch (err) { notify(err.message, true); }
  }
  const au = e.target.closest("[data-kc-authorize]");
  if (au) {
    try { await keycardCommand("add", au.dataset.kcAuthorize, au); notify(t("Card authorized")); renderLastCard(); } catch (err) { notify(err.message, true); }
  }
});
$("#kc-learn-start").addEventListener("click", async e => {
  try { await keycardCommand("learn:start", "", e.target); kc.learning = "cards"; kc.learned = []; renderKeycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-learn-stop").addEventListener("click", async e => {
  try { await keycardCommand("learn:stop", "", e.target); kc.learning = null; kc.learned = []; notify(t("Cards saved")); Views.keycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-master-start").addEventListener("click", async e => {
  try { await keycardCommand("learn:master:start", "", e.target); kc.learning = "master"; renderKeycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-master-stop").addEventListener("click", async e => {
  try { await keycardCommand("learn:master:stop", "", e.target); kc.learning = null; renderKeycards(); } catch (err) { notify(err.message, true); }
});
$("#kc-reset").addEventListener("click", async e => {
  const ok = await confirmDialog({ title: t("Forget all cards"), body: t("Removes all cards. Until a new master is taught in at the reader, no card unlocks the scooter."), ok: t("Forget all cards"), danger: true });
  if (!ok) return;
  try { await keycardCommand("reset", "", e.target); kc.learning = null; kc.learned = []; notify(t("All cards forgotten")); Views.keycards(); } catch (err) { notify(err.message, true); }
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
        <td class="num">${e.dir ? "" : esc(humanSize(e.size))}${e.mtime ? `<span class="m-date">${e.dir ? "" : ", "}${esc(new Date(e.mtime * 1000).toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" }))}</span>` : ""}</td>
        <td>${e.mtime ? esc(new Date(e.mtime * 1000).toLocaleString()) : ""}</td>
        <td class="actions"><span class="row-actions">
          <a class="btn btn-small btn-quiet" href="${esc(dl)}">${e.dir ? t("Download .tar") : t("Download")}</a>
          <button type="button" class="btn btn-small btn-quiet" data-del="${esc(full)}" data-dir="${e.dir ? 1 : 0}">${t("Delete")}</button>
        </span></td>
      </tr>`);
    }
    tb.innerHTML = rows.join("") || `<tr class="files-empty"><td colspan="4">${t("Empty folder.")}</td></tr>`;
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
    title: isDir ? t("Delete folder") : t("Delete file"),
    body: isDir ? t("Delete \"{name}\" and everything inside it? This cannot be undone.", { name }) : t("Delete \"{name}\"? This cannot be undone.", { name }),
    ok: t("Delete"), danger: true,
  });
  if (!ok) return;
  try {
    await API.del(`/api/files?path=${encodeURIComponent(del.dataset.del)}${isDir ? "&recursive=1" : ""}`);
    notify(t("Deleted {name}", { name }));
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
      notify(t("Uploaded {name}", { name: f.name }));
    } catch (err) { notify(err.message, true); }
  }
  renderFiles();
}

const dz = $("#dropzone");
dz.addEventListener("dragover", e => { e.preventDefault(); dz.classList.add("is-drag"); });
dz.addEventListener("dragleave", () => dz.classList.remove("is-drag"));
dz.addEventListener("drop", async e => { e.preventDefault(); dz.classList.remove("is-drag"); await uploadFiles([...e.dataTransfer.files]); });

$("#files-mkdir").addEventListener("click", async () => {
  const name = await promptDialog({ title: t("New folder"), body: `In /data${filesPath ? "/" + filesPath : ""}`, placeholder: t("Folder name"), ok: t("Create") });
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
    [t("Identifier"), has(id.vin) ? `<span class="mono">${esc(id.vin)}</span>` : `<span class="muted">${t("none yet")}</span>`],
    [t("IMEI"), has(id.imei) ? `<span class="mono">${esc(id.imei)}</span>` : `<span class="muted">${t("modem not ready")}</span>`],
    [t("MDB serial"), has(id["mdb-serial"]) ? `<span class="mono">${esc(id["mdb-serial"])}</span>` : null],
    [t("Display serial"), has(id["dbc-serial"]) ? `<span class="mono">${esc(id["dbc-serial"])}</span>` : null],
  ]);

  const services = data.services || {};
  const order = ["radio-gaga", "uplink"];
  const NAMES = { "radio-gaga": "radio-gaga", "uplink": "uplink-service" };
  $("#cloud-services").innerHTML = order.filter(n => services[n]).map(n => {
    const s = services[n];
    const lines = [];
    if (!s.installed) lines.push(`<span class="muted">${t("Not installed")}</span>`);
    else lines.push(status(s.active, s.active === "active" ? t("Running") : human(s.active)));
    if (s.configured) {
      lines.push(`${s.backend === "sunshine" ? t("Connected to Sunshine as") : t("Custom backend,")} <span class="mono">${esc(s.identifier)}</span>`);
      if (s.backend === "custom") lines.push(`<span class="muted">${esc(s["server-url"])}</span>`);
    } else {
      lines.push(`<span class="muted">${t("Not configured")}</span>`);
    }
    lines.push(`<span class="muted mono">${esc(s["config-path"])}</span>`);
    return `<div class="svc"><div class="svc-name">${esc(NAMES[n])}<span class="svc-unit">${esc(s.unit)}</span></div><div class="svc-lines">${lines.join("")}</div></div>`;
  }).join("");

  const connected = order.map(n => services[n]).filter(s => s && s.configured && s.backend === "sunshine");
  const box = $("#cloud-connected");
  if (connected.length) {
    box.hidden = false;
    box.textContent = t("Already connected to Sunshine as {id}. Connecting again moves the scooter to the token owner's account and replaces the config.", { id: connected[0].identifier });
    $("#cloud-bootstrap-form button").textContent = t("Reconnect");
  } else {
    box.hidden = true;
    $("#cloud-bootstrap-form button").textContent = t("Connect");
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
    if (problems.length) notify(t("Config written, but {problems}", { problems: problems.join("; ") }), true);
    else notify(t("Connected to Sunshine as {id}", { id: res.identifier || t("this scooter") }));
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
  if (!yaml.trim()) return notify(t("Paste a config first"), true);
  const btn = $("button[type=submit]", e.target);
  const out = $("#cloud-config-out");
  btn.classList.add("is-busy");
  try {
    const res = await API.post("/api/cloud/config", { service: $("#cloud-service").value, yaml, "config-path": $("#cloud-path").value.trim() });
    const problems = [res.error, res["restart-error"], res["enable-error"]].filter(Boolean);
    showResult(out, res, problems.length > 0);
    notify(problems.length ? t("Config written, but {problems}", { problems: problems.join("; ") }) : t("Config installed, {service} restarted", { service: res.service }), problems.length > 0);
    Views.cloud();
  } catch (err) {
    showResult(out, err.message, true);
    notify(err.message, true);
  } finally { btn.classList.remove("is-busy"); }
});

// ---------- services ----------

let units = [];
let unitFilter = "all";

async function loadServices() {
  try {
    units = (await API.get("/api/services")).units || [];
    renderServices();
  } catch (err) { notify(err.message, true); }
}

function unitTone(u) {
  if (u.load === "masked") return "is-warn";
  if (u.active === "active") return "is-good";
  if (u.active === "failed") return "is-bad";
  return "";
}
function unitStateLabel(u) {
  if (u.load === "masked") return t("Masked");
  if (u.active === "failed") return t("Failed");
  if (u.active === "active") return u.sub === "exited" ? t("Completed") : t("Running");
  if (u.active === "activating") return t("Starting");
  if (u.active === "deactivating") return t("Stopping");
  return t("Stopped");
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
        <button type="button" class="btn btn-small btn-quiet" data-svc="${esc(u.unit)}" data-act="restart">${t("Restart")}</button>
        ${running ? `<button type="button" class="btn btn-small btn-quiet" data-svc="${esc(u.unit)}" data-act="stop">${t("Stop")}</button>`
                  : `<button type="button" class="btn btn-small btn-quiet" data-svc="${esc(u.unit)}" data-act="start">${t("Start")}</button>`}
      </span></td>
    </tr>`;
  }).join("") || `<tr><td colspan="3" class="muted">${t("Nothing matches this filter.")}</td></tr>`;
}

function setUnitFilter(f) {
  unitFilter = f;
  $$(".chip").forEach(b => b.classList.toggle("is-active", b.dataset.filter === f));
  $("#services-filter-select").value = f;
  renderServices();
}
$$(".chip").forEach(btn => btn.addEventListener("click", () => setUnitFilter(btn.dataset.filter)));
$("#services-filter-select").addEventListener("change", e => setUnitFilter(e.target.value));
$("#services-refresh").addEventListener("click", async e => {
  e.currentTarget.classList.add("is-busy");
  try { await loadServices(); } finally { $("#services-refresh").classList.remove("is-busy"); }
});

$("#services-table").addEventListener("click", async e => {
  const btn = e.target.closest("[data-svc]");
  if (!btn) return;
  const { svc, act } = btn.dataset;
  const name = svc.replace(/\.service$/, "");
  const critical = ["valkey.service", "redis.service", "librescoot-vehicle.service", "librescoot-pm.service"].includes(svc);
  if (act !== "start" && critical) {
    const ok = await confirmDialog({ title: `${human(act)} ${name}`, body: t("{name} is a core service. Stopping or restarting it interrupts the whole scooter briefly and may disconnect this page.", { name }), ok: human(act), danger: true });
    if (!ok) return;
  }
  btn.classList.add("is-busy");
  try {
    await API.post("/api/services/action", { unit: svc, action: act });
    notify(`${t({ restart: "Restarted", stop: "Stopped", start: "Started" }[act] || human(act))} ${name}`);
    loadServices();
  } catch (err) { notify(err.message, true); }
  finally { btn.classList.remove("is-busy"); }
});

// ---------- boot ----------

// Static markup is translated now; the scooter's dashboard.language may switch it later.
I18N.apply();
$("#lang-select").value = localStorage.getItem("lsd-lang") || "";
$("#lang-select").addEventListener("change", e => {
  if (e.target.value) localStorage.setItem("lsd-lang", e.target.value); else localStorage.removeItem("lsd-lang");
  setLanguage(I18N.pick(H("settings")["dashboard.language"]));
});
function setLanguage(lang) {
  if (lang === I18N.lang) return;
  I18N.lang = lang;
  I18N.apply();
  document.querySelectorAll(".hold-tip").forEach(el => { el.textContent = t("Hold to confirm"); });
  if (schema) renderSettings();
  Views[currentView]();
}

connectStream();
route();
API.get("/api/info").then(info => { if (info.version) document.title = `Librescoot ${info.version}`; }).catch(() => {});
