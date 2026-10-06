const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { test } = require("node:test");
const vm = require("node:vm");

const source = readFileSync(join(__dirname, "../static/app.js"), "utf8");
const markup = readFileSync(join(__dirname, "../static/index.html"), "utf8");
function between(start, end) {
  const from = source.indexOf(start);
  const to = source.indexOf(end, from);
  assert.ok(from >= 0 && to > from, `Missing source boundaries: ${start}, ${end}`);
  return source.slice(from, to);
}

// Execute the production router and view loaders with a minimal DOM/API fixture.
function fixture(hash = "#dashboard", get = async () => ({})) {
  const nodes = new Map();
  const scrolls = [];
  const events = {};
  let context;
  const run = code => vm.runInContext(code, context);
  function node(id, tagName = "SECTION", hidden = false) {
    const el = {
      id, tagName, hidden, open: false, options: [0, 1, 2],
      classList: { toggle() {} },
      getClientRects() {
        const view = id.startsWith("dash-") ? "dashboard" : id.startsWith("sys-") ? "system" : id.startsWith("group-") ? "settings" : null;
        return el.hidden || (view && nodes.get("view-" + view)?.hidden) || (el.advanced && !context.advancedMode) ? [] : [{}];
      },
      scrollIntoView: options => scrolls.push({ id, options }),
    };
    nodes.set(id, el);
    return el;
  }
  for (const match of markup.matchAll(/<(\w+)\b[^>]*\bid="([^"]+)"[^>]*>/g)) {
    node(match[2], match[1].toUpperCase(), /\bhidden\b/.test(match[0])).advanced = /class="[^"]*\badv-only\b/.test(match[0]);
  }
  const views = [...nodes.values()].filter(el => el.id.startsWith("view-"));
  const document = {
    getElementById: id => nodes.get(id),
    addEventListener: (event, fn) => { events[event] = fn; },
  };
  const location = { hash, replace: hash => { location.hash = hash; } };
  context = vm.createContext({
    document, location,
    window: { addEventListener: (event, fn) => { events[event] = fn; } },
    $: selector => nodes.get(selector.slice(1)),
    $$: selector => selector === ".view" ? views : [],
    API: { get }, state: { live: true }, units: [], advancedMode: false,
    updateSavebar() {}, measureSticky() {}, renderDashboard() {}, loadEvents() {},
    renderSystemFacts() {}, renderBundles() {}, loadServices: async () => {},
    notify() {}, t: text => text,
    renderSettings() {
      for (const meta of Object.values(run("schema"))) {
        if (meta["user-visible"] || run("advancedMode")) node("group-" + meta.service);
      }
    },
    setAdvanced(on) { context.advancedMode = on; run("renderSettings(); Views[currentView]();"); },
  });
  run(between("const Views = {};", "// ---------- live state ----------"));
  run(between("Views.dashboard =", "// ---------- settings ----------"));
  run(between("let schema = null;", "// Live settings changes"));
  run(between("function markCurrentGroup()", "function currentValue"));
  run(between("Views.system =", "function renderSystemFacts"));
  const route = () => run("route()");
  const navigate = async hash => {
    location.hash = hash;
    await events.hashchange();
  };
  async function click(hash, options = {}) {
    let prevented = false;
    const a = {
      target: options.target || "",
      getAttribute: () => hash,
      hasAttribute: name => name === "download" && options.download,
      dataset: { jump: hash.split("/")[1] },
      closest: () => ({ dataset: { prefix: { dashboard: "dash-", settings: "group-", system: "sys-" }[hash.slice(1).split("/")[0]] } }),
    };
    const event = {
      button: 0, ...options,
      target: { closest: selector => selector === ".jump [data-jump]" && !options.jump ? null : a },
      preventDefault() { prevented = true; },
    };
    events.click(event);
    if (!prevented && !options.ctrlKey && !options.metaKey && !options.shiftKey && !options.altKey && !options.target && !options.download && hash !== location.hash) {
      await navigate(hash);
    }
    await new Promise(resolve => setImmediate(resolve));
    return prevented;
  }
  return { nodes, scrolls, events, location, route, navigate, click, run, node };
}

function lastTarget(f) { return f.scrolls.at(-1)?.id; }

for (const hash of new Set([...markup.matchAll(/href="(#(?:dashboard|system)\/[^"]+)"/g)].map(m => m[1]))) {
  test(`direct section route ${hash}`, async () => {
    const f = fixture(hash);
    // Advanced mode and shell availability are visibility prerequisites.
    f.run("advancedMode = true");
    f.nodes.get("sys-shell").hidden = false;
    await f.route();
    const [view, section] = hash.slice(1).split("/");
    assert.equal(lastTarget(f), (view === "dashboard" ? "dash-" : "sys-") + section);
    assert.equal(f.nodes.get("view-" + view).hidden, false);
    if (hash === "#system/shell") assert.equal(f.nodes.get("sys-shell").open, true);
  });
}

test("fault badge navigates and repeated taps scroll again", async () => {
  const f = fixture();
  await f.route();
  await f.click("#dashboard/faults");
  assert.equal(f.location.hash, "#dashboard/faults");
  assert.equal(lastTarget(f), "dash-faults");
  f.scrolls.length = 0;
  assert.equal(await f.click("#dashboard/faults"), true);
  assert.equal(lastTarget(f), "dash-faults");
  assert.equal(f.scrolls[0].options.behavior, "smooth");
});

test("section links preserve the hash for reload and back navigation", async () => {
  const f = fixture();
  await f.route();
  await f.click("#dashboard/vehicle", { jump: true });
  assert.equal(f.location.hash, "#dashboard/vehicle");
  await f.click("#dashboard/faults", { jump: true });
  await f.navigate("#dashboard/vehicle");
  assert.equal(lastTarget(f), "dash-vehicle");
  f.scrolls.length = 0;
  await f.route();
  assert.equal(lastTarget(f), "dash-vehicle");
});

test("settings sections scroll after async rendering and repeat taps", async () => {
  const f = fixture("#settings/pm-service", async path => path.endsWith("/schema")
    ? { timeout: { service: "pm-service", "user-visible": true } }
    : {});
  await f.route();
  assert.equal(lastTarget(f), "group-pm-service");
  f.scrolls.length = 0;
  await f.click("#settings/pm-service");
  assert.equal(lastTarget(f), "group-pm-service");
});

test("advanced-only settings deep links reveal the target", async () => {
  const f = fixture("#settings/pm-service", async path => path.endsWith("/schema")
    ? { timeout: { service: "pm-service", "user-visible": false } }
    : {});
  await f.route();
  assert.equal(f.run("advancedMode"), true);
  assert.equal(lastTarget(f), "group-pm-service");
});

test("static system sections scroll without waiting for API responses", async () => {
  let resolve;
  const response = new Promise(done => { resolve = done; });
  const f = fixture("#system/services", () => response);
  const routing = f.route();
  assert.equal(lastTarget(f), "sys-services");
  await f.navigate("#dashboard/faults");
  f.scrolls.length = 0;
  resolve({});
  await routing;
  assert.equal(f.scrolls.length, 0);
});

test("pending settings navigation cannot scroll another view", async () => {
  let resolve;
  const response = new Promise(done => { resolve = done; });
  const f = fixture("#settings/pm-service", () => response);
  const routing = f.route();
  await f.navigate("#dashboard/faults");
  f.scrolls.length = 0;
  resolve({ timeout: { service: "pm-service", "user-visible": true } });
  await routing;
  assert.equal(f.scrolls.length, 0);
  assert.equal(f.nodes.has("group-pm-service"), false);
});

test("hidden, unknown and malformed sections do not scroll or throw", async () => {
  for (const hash of ["#system/shell", "#dashboard/missing", "#dashboard/%", "#missing/faults", "#dashboard"]) {
    const f = fixture(hash);
    await f.route();
    assert.equal(f.scrolls.length, 0, hash);
  }
});

test("modified clicks and download links retain browser behavior", async () => {
  const f = fixture("#dashboard/faults");
  await f.route();
  f.scrolls.length = 0;
  for (const options of [{ ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { altKey: true }, { button: 1 }, { target: "_blank" }, { download: true }, { defaultPrevented: true }]) {
    assert.equal(await f.click("#dashboard/faults", options), false);
  }
  assert.equal(f.scrolls.length, 0);
});
