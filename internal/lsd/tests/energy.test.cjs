const assert = require("node:assert/strict");
const { readFileSync } = require("node:fs");
const { join } = require("node:path");
const { test } = require("node:test");
const vm = require("node:vm");

const source = readFileSync(join(__dirname, "../static/app.js"), "utf8");
const from = source.indexOf('[t("Energy"),');
const to = source.indexOf("],", from);
assert.ok(from >= 0 && to > from, "Missing dashboard energy row");
const row = source.slice(from, to + 1);
const helpers = source.split("\n").filter(line => /^const (has|num) =/.test(line)).join("\n");

function energy(ecu) {
  return Array.from(vm.runInNewContext(`${helpers}\n(${row})`, { ecu, t: text => text }));
}

test("dashboard converts both ECU mWh counters to kWh", () => {
  assert.deepEqual(energy({ "energy:consumed": "1000000", "energy:recovered": "500000" }),
    ["Energy", "1.0 kWh used", "0.5 kWh recovered"]);
});

test("dashboard preserves zero energy counters", () => {
  assert.deepEqual(energy({ "energy:consumed": "0", "energy:recovered": "0" }),
    ["Energy", "0.0 kWh used", "0.0 kWh recovered"]);
});

test("dashboard omits missing counters independently", () => {
  assert.deepEqual(energy({}), ["Energy", null, null]);
  assert.deepEqual(energy({ "energy:consumed": "1000000", "energy:recovered": "" }),
    ["Energy", "1.0 kWh used", null]);
  assert.deepEqual(energy({ "energy:consumed": null, "energy:recovered": "500000" }),
    ["Energy", null, "0.5 kWh recovered"]);
});

test("dashboard supports counters above the uint32 range", () => {
  assert.deepEqual(energy({ "energy:consumed": "4294967296", "energy:recovered": "2345678" }),
    ["Energy", "4295.0 kWh used", "2.3 kWh recovered"]);
});
