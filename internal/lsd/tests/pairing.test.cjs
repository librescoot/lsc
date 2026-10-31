const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { test } = require("node:test");
const vm = require("node:vm");
const source = readFileSync(join(__dirname, "../static/app.js"), "utf8");
const markup = readFileSync(join(__dirname, "../static/index.html"), "utf8");
function fixture(get = async () => ({ state: "not_registered", running: false })) {
  const nodes = new Map(); const timers = new Map(); const posts = []; let nextTimer = 0;
  const context = vm.createContext({
    currentView: "cloud", Date,
    $: selector => {
      if (!nodes.has(selector)) nodes.set(selector, { hidden: true, textContent: "", disabled: false, addEventListener() {} });
      return nodes.get(selector);
    },
    t: (text, values = {}) => text.replace(/\{(\w+)\}/g, (_, key) => values[key]),
    API: { get, post: async (path, body) => { posts.push({ path, body }); return { state: "starting", running: true }; } },
    clearTimeout: id => timers.delete(id), setTimeout: fn => { timers.set(++nextTimer, fn); return nextTimer; },
  });
  const code = source.slice(source.indexOf("let cloudPairingTimer;"), source.indexOf("function renderCloud(data)"));
  vm.runInContext(code, context);
  return { nodes, timers, posts, context, run: code => vm.runInContext(code, context) };
}
test("Cloud entry only reads status and safely displays the short code", async () => {
  const f = fixture(async () => ({ state: "waiting_for_web_setup", running: true, code: "01234-56789", expires_at: Date.now()/1000+900 }));
  await f.run("loadCloudPairing()");
  assert.equal(f.posts.length, 0);
  assert.equal(f.nodes.get("#cloud-pairing-code").textContent, "01234-56789");
  assert.equal(f.nodes.get("#cloud-pairing-code-box").hidden, false);
  assert.equal(f.nodes.get("#cloud-pairing-start").disabled, true);
  assert.equal(f.timers.size, 1);
  assert.match(markup, /href="https:\/\/connect.librescoot.org\/pair"[^>]*rel="noopener noreferrer"/);
});
test("Start and stop are explicit POSTs and use the same polling view", async () => {
  const f = fixture();
  await f.run('cloudPairingAction("start")');
  await f.run('cloudPairingAction("stop")');
  assert.deepEqual(f.posts.map(p => p.body.action), ["start", "stop"]);
  assert.ok(f.posts.every(p => p.path === "/api/cloud/pairing"));
  assert.equal(f.timers.size, 1);
});
test("Connected/paused states hide codes and stop periodic polling", async () => {
  const f = fixture(async () => ({ state: "connected", running: false }));
  await f.run("loadCloudPairing()");
  assert.equal(f.nodes.get("#cloud-pairing-start").disabled, true);
  assert.equal(f.nodes.get("#cloud-pairing-code-box").hidden, true);
  assert.equal(f.nodes.get("#cloud-pairing-stop").hidden, true);
  assert.equal(f.timers.size, 0);
});
test("Leaving Cloud suppresses late responses and further polling", async () => {
  let done; const f = fixture(() => new Promise(resolve => { done = resolve; }));
  const loading = f.run("loadCloudPairing()"); f.context.currentView = "system";
  done({ state: "waiting_for_web_setup", running: true, code: "01234-56789" }); await loading;
  assert.equal(f.timers.size, 0);
  assert.equal(f.nodes.size, 2); // Only the click handlers were bound.
});
test("Unavailable backend hides old code, retries and never echoes raw errors", async () => {
  const f = fixture(async () => { throw new Error("private-token"); });
  await f.run("loadCloudPairing()");
  assert.equal(f.nodes.get("#cloud-pairing-code-box").hidden, true);
  assert.equal(f.nodes.get("#cloud-pairing-code").textContent, "");
  assert.doesNotMatch(f.nodes.get("#cloud-pairing-error").textContent, /private-token/);
  assert.equal(f.timers.size, 1);
});
