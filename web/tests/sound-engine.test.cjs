const test = require("node:test");
const assert = require("node:assert/strict");
const { SoundEngine, headroomFor } = require("../node_modules/.cache/sound-test/sound-engine.js");
const { defaultSettings, hydrateSettings } = require("../node_modules/.cache/sound-test/preferences.js");

class Param {
  constructor() { this.value = 0; this.targets = []; }
  cancelScheduledValues() {}
  setValueAtTime(value) { this.value = value; }
  setTargetAtTime(value) { this.targets.push(value); }
}
class Node {
  constructor() { this.connections = []; this.disconnected = false; }
  connect(node) { this.connections.push(node); return node; }
  disconnect() { this.disconnected = true; }
}
class Context {
  constructor() { this.state = "running"; this.currentTime = 1; this.destination = new Node(); this.bands = []; this.gains = []; }
  createMediaElementSource() { this.source = new Node(); return this.source; }
  createBiquadFilter() {
    const node = new Node();
    node.frequency = new Param(); node.gain = new Param(); node.Q = new Param();
    this.bands.push(node); return node;
  }
  createGain() { const node = new Node(); node.gain = new Param(); this.gains.push(node); return node; }
  createDynamicsCompressor() {
    const node = new Node();
    for (const name of ["threshold", "knee", "ratio", "attack", "release"]) node[name] = new Param();
    return node;
  }
  close() { this.closed = true; return Promise.resolve(); }
}

test("old settings retain flat EQ and new band defaults, invalid inputs are bounded", () => {
  const old = hydrateSettings({ eq: "warm" });
  assert.equal(old.eq, "warm");
  assert.deepEqual([old.eqBass, old.eqMid, old.eqTreble], [0, 0, 0]);
  const custom = hydrateSettings({ eq: "custom", eqBass: 25, eqMid: -30, eqTreble: "bad" });
  assert.deepEqual([custom.eqBass, custom.eqMid, custom.eqTreble], [12, -12, 0]);
  assert.equal(hydrateSettings({ eq: "toString" }).eq, "flat");
});

test("custom EQ constructs three filters and updates them without rebuilding the graph", () => {
  const ctx = new Context();
  const engine = new SoundEngine(() => ctx);
  const settings = { ...defaultSettings(), eq: "custom", eqBass: 3, eqMid: -2, eqTreble: 4 };
  engine.configure(settings);
  engine.unlock({});
  assert.deepEqual(ctx.bands.map((node) => [node.type, node.frequency.value, node.gain.value]), [
    ["lowshelf", 200, 3], ["peaking", 1000, -2], ["highshelf", 6000, 4],
  ]);
  assert.ok(Math.abs(ctx.gains[0].gain.value - headroomFor([3, -2, 4])) < 1e-12);
  const source = ctx.source;
  engine.configure({ ...settings, eqBass: 100, eqMid: Number.NaN, eqTreble: -50 });
  assert.equal(ctx.bands.length, 3, "slider updates must not rebuild the chain");
  assert.equal(ctx.source, source);
  assert.deepEqual(ctx.bands.map((node) => node.gain.targets.at(-1)), [12, 0, -12]);
  assert.ok(Math.abs(ctx.gains[0].gain.targets.at(-1) - headroomFor([12, 0, -12])) < 1e-12);
  engine.dispose();
  assert.equal(source.disconnected, true);
  assert.ok(ctx.bands.every((node) => node.disconnected));
  assert.equal(ctx.closed, true);
});

test("headroom accounts for combined boosts; cuts do not add attenuation", () => {
  assert.equal(headroomFor([0, -3]), 1);
  assert.ok(Math.abs(headroomFor([3, 4, -2]) - 10 ** (-7 / 20)) < 1e-12);
});
