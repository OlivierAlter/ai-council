'use strict';
// CLAUDE — MOTION REEL '26
// 15 seconds, 1920x1080, 128 BPM. Every frame is a pure function of time: render(t, frame).

const W = 1920, H = 1080, FPS = 60, DUR = 15, BPM = 128, BEAT = 60 / BPM, BAR = BEAT * 4;
const TAU = Math.PI * 2, CX = W / 2, CY = H / 2;
const C = {
  ink: '#0B0B10', cream: '#F4EFE4', coral: '#FF5B3A', violet: '#7A5CFF',
  lime: '#D6FF3E', cyan: '#3BD6FF', pink: '#FF4FA3',
};
const RGB = {
  ink: [11, 11, 16], cream: [244, 239, 228], coral: [255, 91, 58], violet: [122, 92, 255],
  lime: [214, 255, 62], cyan: [59, 214, 255], pink: [255, 79, 163],
};

// ---------------------------------------------------------------- canvases
function mk(w, h) { const c = document.createElement('canvas'); c.width = w; c.height = h; return [c, c.getContext('2d')]; }
const cv = document.getElementById('c'); cv.width = W; cv.height = H;
const X = cv.getContext('2d');
const [SC, S] = mk(W, H);            // scene buffer
const [BL, B] = mk(W / 4, H / 4);    // bloom downsample
const [BL2, B2] = mk(W / 4, H / 4);  // bloom blurred
const [CH, CHX] = mk(W, H);          // chromatic aberration channel

// ---------------------------------------------------------------- math
const clamp = (x, a = 0, b = 1) => Math.min(b, Math.max(a, x));
const lerp = (a, b, t) => a + (b - a) * t;
const prog = (t, a, b) => clamp((t - a) / (b - a));
const E = {
  outExpo: x => x >= 1 ? 1 : 1 - Math.pow(2, -10 * x),
  inExpo: x => x <= 0 ? 0 : Math.pow(2, 10 * x - 10),
  inOutExpo: x => x <= 0 ? 0 : x >= 1 ? 1 : x < .5 ? Math.pow(2, 20 * x - 10) / 2 : (2 - Math.pow(2, -20 * x + 10)) / 2,
  outCubic: x => 1 - Math.pow(1 - x, 3),
  inCubic: x => x * x * x,
  inOutCubic: x => x < .5 ? 4 * x * x * x : 1 - Math.pow(-2 * x + 2, 3) / 2,
  inOutQuart: x => x < .5 ? 8 * x ** 4 : 1 - Math.pow(-2 * x + 2, 4) / 2,
  outQuart: x => 1 - Math.pow(1 - x, 4),
  outBack: (x, s = 1.70158) => x <= 0 ? 0 : 1 + (s + 1) * Math.pow(x - 1, 3) + s * Math.pow(x - 1, 2),
  inBack: (x, s = 1.70158) => (s + 1) * x * x * x - s * x * x,
  outElastic: x => x <= 0 ? 0 : x >= 1 ? 1 : Math.pow(2, -10 * x) * Math.sin((x * 10 - .75) * (TAU / 3)) + 1,
  outBounce: x => {
    const n = 7.5625, d = 2.75;
    if (x < 1 / d) return n * x * x;
    if (x < 2 / d) return n * (x -= 1.5 / d) * x + .75;
    if (x < 2.5 / d) return n * (x -= 2.25 / d) * x + .9375;
    return n * (x -= 2.625 / d) * x + .984375;
  },
};
function mulberry(a) { return () => { a |= 0; a = a + 0x6D2B79F5 | 0; let t = Math.imul(a ^ a >>> 15, 1 | a); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }; }
function hash(n) { const s = Math.sin(n * 127.1 + 311.7) * 43758.5453; return s - Math.floor(s); }
function noise1(x) { const i = Math.floor(x), f = x - i, u = f * f * (3 - 2 * f); return lerp(hash(i), hash(i + 1), u) * 2 - 1; }
const mixRGB = (a, b, t) => [lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)];
const rgba = (c, a = 1) => `rgba(${c[0] | 0},${c[1] | 0},${c[2] | 0},${a})`;

// decaying pulse on every beat
function pulse(t, k = 7) { if (t < 0) return 0; const ph = (t / BEAT) % 1; return Math.exp(-ph * BEAT * k); }
const SCENE_T = i => i * BAR;
const IMPACTS = [[SCENE_T(1), .45], [SCENE_T(2), .7], [SCENE_T(3), 1], [SCENE_T(4), .8], [SCENE_T(5), .9], [SCENE_T(6), .6], [SCENE_T(7), 1.25]];
function impact(t) { let s = 0; for (const [a, m] of IMPACTS) if (t >= a) s += m * Math.exp(-(t - a) * 9); return s; }

// text helpers
function setFont(c, f, ls = '0px') { c.font = f; c.letterSpacing = ls; }
function charXs(c, str) { const xs = []; for (let i = 0; i <= str.length; i++) xs.push(c.measureText(str.slice(0, i)).width); return xs; }
function typed(str, p) { return str.slice(0, Math.floor(str.length * clamp(p))); }

// =================================================================== SCENE 1 — GENESIS
const STRIPES = [C.coral, C.violet, C.cream, C.lime, C.cyan];
function drawStripes(c, t, mode) {
  for (let i = 0; i < 5; i++) {
    const k = i - 2;
    let y = CY + k * (H / 5), hh, x = 0;
    if (mode === 'in') {
      const fan = E.outExpo(prog(t, 3 * BEAT, 3 * BEAT + .35));
      y = lerp(CY, y, fan);
      hh = lerp(4, H / 5 + 2, E.inOutExpo(prog(t, 1.52 + Math.abs(k) * .04, BAR)));
    } else {
      hh = H / 5 + 2;
      const p = E.inOutQuart(prog(t, BAR + i * .035, BAR + i * .035 + .38));
      x = (i % 2 ? 1 : -1) * p * (W + 40);
    }
    c.fillStyle = STRIPES[i];
    c.fillRect(x, y - hh / 2, W, hh);
  }
}
function s1(t) {
  const c = S;
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  // ambient glow
  const g = c.createRadialGradient(CX, CY, 0, CX, CY, 700);
  g.addColorStop(0, `rgba(122,92,255,${.14 * prog(t, 0, .8)})`); g.addColorStop(1, 'rgba(0,0,0,0)');
  c.fillStyle = g; c.fillRect(0, 0, W, H);
  // ripples on the first beats
  for (let b = 0; b < 2; b++) {
    const dt = t - b * BEAT - .05; if (dt < 0 || dt > 1.3) continue;
    const e = E.outExpo(dt / 1.3);
    c.strokeStyle = `rgba(244,239,228,${(1 - e) * .55})`; c.lineWidth = 2;
    c.beginPath(); c.arc(CX, CY, 20 + e * 760, 0, TAU); c.stroke();
    c.strokeStyle = `rgba(255,91,58,${(1 - e) * .4})`;
    c.beginPath(); c.arc(CX, CY, 20 + E.outExpo(clamp((dt - .06) / 1.3)) * 620, 0, TAU); c.stroke();
  }
  if (t < 3 * BEAT) {
    const appear = E.outBack(prog(t, 0, .42), 2.6);
    const bp = t < .9 ? pulse(t) : 0;
    const antic = Math.sin(Math.PI * prog(t, .74, .96));             // anticipation squash
    const stretch = E.inOutExpo(prog(t, .94, 1.32));                 // the smear
    const r = 18 * appear;
    let w = r * 2 * (1 + .35 * bp) * (1 - .4 * antic), h = r * 2 * (1 - .25 * bp) * (1 + .35 * antic);
    w = lerp(w, W * 1.25, stretch);
    h = lerp(h, 4, E.outExpo(prog(t, .94, 1.12)));
    c.fillStyle = C.cream;
    c.beginPath(); c.roundRect(CX - w / 2, CY - h / 2, w, h, Math.min(w, h) / 2); c.fill();
  } else {
    drawStripes(c, t, 'in');
  }
  // boot caption
  const ca = 1 - prog(t, .86, 1.0);
  if (ca > 0) {
    setFont(c, '500 22px Mono', '3px'); c.textAlign = 'center'; c.textBaseline = 'alphabetic';
    c.fillStyle = `rgba(244,239,228,${.7 * ca})`;
    const s = typed('// init motion_reel --fps 60', prog(t, .12, .7));
    const cur = (Math.floor(t * 8) % 2) ? '▍' : ' ';
    c.fillText(s + cur, CX, CY + 96);
  }
}

// =================================================================== SCENE 2 — TYPE
const ROLL_GLYPHS = 'AXKQ7RZ#BWVH&';
function s2(t) {
  const c = S, T0 = BAR;
  c.fillStyle = C.cream; c.fillRect(0, 0, W, H);

  // --- layout
  const bigF = '900 290px Inter', serF = 'italic 400 140px Serif';
  setFont(c, serF); const serTxt = 'is my obsession', serW = c.measureText(serTxt + '.').width;
  const serX = CX - serW / 2, serY = 800;
  const px = serX + c.measureText(serTxt).width + c.measureText('.').width / 2, py = serY - 14;
  const push = E.inExpo(prog(t, 3.3, 3.75));
  c.save();
  c.translate(px, py); c.scale(1 + push * 7, 1 + push * 7); c.rotate(push * .12); c.translate(-px, -py);

  // --- grid lines & marks
  c.strokeStyle = 'rgba(11,11,16,.12)'; c.lineWidth = 1;
  const gl = E.outExpo(prog(t, T0, T0 + .8));
  for (let i = 1; i < 12; i++) { const x = i * W / 12; c.beginPath(); c.moveTo(x, 0); c.lineTo(x, H * gl); c.stroke(); }
  c.beginPath(); c.moveTo(0, 590); c.lineTo(W * gl, 590); c.stroke();

  // --- big word: MOTION rolls into DESIGN
  setFont(c, bigF, '-8px');
  const w1 = 'MOTION', w2 = 'DESIGN';
  const xs1 = charXs(c, w1), xs2 = charXs(c, w2);
  const tot1 = xs1[6], tot2 = xs2[6];
  const baseY = 560, LH = 300;
  const rnd = mulberry(7);
  c.save();
  c.beginPath(); c.rect(0, baseY - 238, W, 262); c.clip();
  c.textAlign = 'center'; c.fillStyle = C.ink;
  for (let i = 0; i < 6; i++) {
    const rev = E.outExpo(prog(t, T0 + .08 + i * .05, T0 + .08 + i * .05 + .75));
    const roll = E.inOutCubic(prog(t, 2.72 + i * .045, 2.72 + i * .045 + .42));
    const c1 = CX - tot1 / 2 + (xs1[i] + xs1[i + 1]) / 2, c2 = CX - tot2 / 2 + (xs2[i] + xs2[i + 1]) / 2;
    const x = lerp(c1, c2, roll);
    const strip = [w1[i]]; for (let k = 0; k < 4; k++) strip.push(ROLL_GLYPHS[Math.floor(rnd() * ROLL_GLYPHS.length)]); strip.push(w2[i]);
    const yOff = (1 - rev) * 300;
    for (let j = 0; j < strip.length; j++) {
      const y = baseY + yOff + (j - roll * 5) * LH;
      if (y < baseY - 300 || y > baseY + 300) continue;
      c.save(); c.translate(x, y); c.rotate((1 - rev) * .25);
      c.fillText(strip[j], 0, 0); c.restore();
    }
  }
  c.restore();

  // coral marker bar under the word
  const bar = E.inOutExpo(prog(t, 2.32, 2.7)), barOut = E.inOutExpo(prog(t, 2.72, 3.05));
  const bx0 = CX - tot1 / 2, bx1 = CX + tot1 / 2;
  c.fillStyle = C.coral;
  c.fillRect(lerp(bx0, bx1, barOut), baseY + 30, (bx1 - bx0) * (bar - barOut), 16);

  // micro labels
  setFont(c, '500 22px Mono', '3px'); c.textAlign = 'left'; c.fillStyle = C.ink;
  c.fillText(typed('(01) KINETIC TYPE', prog(t, 2.05, 2.4)), CX - tot1 / 2, baseY - 270);
  c.textAlign = 'right';
  c.fillText(typed('ROLL → 6 GLYPHS × 45ms', prog(t, 2.72, 3.1)), CX + tot1 / 2, baseY - 270);

  // --- serif line
  setFont(c, serF); c.textAlign = 'left';
  const words = ['is ', 'my ', 'obsession'];
  let wx = serX;
  for (let i = 0; i < words.length; i++) {
    const e = E.outExpo(prog(t, 3.0 + i * .07, 3.0 + i * .07 + .6));
    c.save(); c.beginPath(); c.rect(wx - 20, serY - 150, c.measureText(words[i]).width + 40, 190); c.clip();
    c.fillStyle = C.coral; c.globalAlpha = e;
    c.fillText(words[i], wx, serY + (1 - e) * 120);
    c.restore();
    wx += c.measureText(words[i]).width;
  }
  c.restore();
  // the period: it becomes the iris
  const dotIn = E.outBack(prog(t, 3.2, 3.42), 3);
  const r = Math.max(11 * dotIn * (1 + push * 7), E.inExpo(prog(t, 3.42, 3.75)) * 2400);
  c.fillStyle = C.ink; c.beginPath(); c.arc(px, py, r, 0, TAU); c.fill();

  // stripes leaving
  drawStripes(c, t, 'out');
}

// =================================================================== SCENE 3 — FORM
const NA = 256;
function polyVerts(n, R, rot, inner) {
  const v = [];
  const m = inner ? n * 2 : n;
  for (let i = 0; i < m; i++) { const a = rot + i * TAU / m, r = inner && i % 2 ? inner : R; v.push([Math.cos(a) * r, Math.sin(a) * r]); }
  return v;
}
function rayPoly(v, th) {
  const dx = Math.cos(th), dy = Math.sin(th); let best = 1e9;
  for (let i = 0; i < v.length; i++) {
    const [ax, ay] = v[i], [bx, by] = v[(i + 1) % v.length];
    const ex = bx - ax, ey = by - ay, den = dx * ey - dy * ex;
    if (Math.abs(den) < 1e-9) continue;
    const s = (ax * ey - ay * ex) / den, u = (ax * dy - ay * dx) / den;
    if (s > 0 && u >= -1e-6 && u <= 1 + 1e-6) best = Math.min(best, s);
  }
  return best;
}
const shapeTable = f => Float32Array.from({ length: NA }, (_, i) => f(i / NA * TAU));
const SHAPES = [
  shapeTable(() => 1),
  shapeTable(th => rayPoly(polyVerts(4, 1.22, Math.PI / 4), th)),
  shapeTable(th => rayPoly(polyVerts(3, 1.38, -Math.PI / 2), th)),
  shapeTable(th => rayPoly(polyVerts(5, 1.35, -Math.PI / 2, .56), th)),
];
const MORPHS = [SCENE_T(2) + BEAT, SCENE_T(2) + 2 * BEAT, SCENE_T(2) + 3 * BEAT];
function shapeAt(t) {
  const r = Float32Array.from(SHAPES[0]);
  let rot = t * .35;
  MORPHS.forEach((tb, k) => {
    const e = E.outExpo(prog(t, tb, tb + .38));
    if (e > 0) for (let i = 0; i < NA; i++) r[i] = lerp(r[i], SHAPES[k + 1][i], e);
    rot += E.outBack(prog(t, tb, tb + .55), 2.2) * Math.PI / 2;
  });
  const T0 = SCENE_T(2);
  let sc = 250 * E.outBack(prog(t, T0, T0 + .42), 2.4) * (1 + .07 * pulse(t - T0));
  const col = prog(t, 5.34, 5.6);
  sc *= 1 - clamp(E.inBack(col, 2.4));
  rot += E.inCubic(col) * TAU;
  return { r, rot, sc };
}
function tracePath(c, sh, x, y, scMul = 1) {
  c.beginPath();
  for (let i = 0; i <= NA; i++) {
    const k = i % NA, a = k / NA * TAU + sh.rot, rr = sh.r[k] * sh.sc * scMul;
    const px = x + Math.cos(a) * rr, py = y + Math.sin(a) * rr;
    i ? c.lineTo(px, py) : c.moveTo(px, py);
  }
  c.closePath();
}
function s3(t) {
  const c = S, T0 = SCENE_T(2);
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  // ripple dot-grid
  const sp = 60;
  c.fillStyle = C.cream;
  const beats = [T0, ...MORPHS];
  for (let gx = sp / 2; gx < W; gx += sp) for (let gy = sp / 2; gy < H; gy += sp) {
    const dx = gx - CX, dy = gy - CY, d = Math.hypot(dx, dy) || 1;
    let disp = 0;
    for (const bt of beats) { const dt = t - bt; if (dt <= 0) continue; disp += 34 * Math.exp(-(((d - dt * 1500) / 90) ** 2)) * Math.exp(-dt * 1.4); }
    const a = .16 + disp * .02;
    c.globalAlpha = Math.min(.9, a) * prog(t, T0, T0 + .3);
    const s = 2.2 + disp * .12;
    c.fillRect(gx + dx / d * disp - s / 2, gy + dy / d * disp - s / 2, s, s);
  }
  c.globalAlpha = 1;
  // orbits
  const oa = E.outExpo(prog(t, T0 + .15, T0 + .7)) * (1 - prog(t, 5.3, 5.55));
  for (let k = 0; k < 3; k++) {
    c.save(); c.translate(CX, CY); c.rotate(k * TAU / 6 - .3);
    c.strokeStyle = `rgba(244,239,228,${.22 * oa})`; c.lineWidth = 1.5; c.setLineDash([4, 10]);
    c.beginPath(); c.ellipse(0, 0, 440 * oa, 150 * oa, 0, 0, TAU); c.stroke(); c.setLineDash([]);
    const a = t * (2.1 + k * .4) + k * 2;
    c.fillStyle = [C.lime, C.cyan, C.pink][k];
    c.beginPath(); c.arc(Math.cos(a) * 440 * oa, Math.sin(a) * 150 * oa, 9 * oa, 0, TAU); c.fill();
    c.restore();
  }
  // echo trails
  const trail = [RGB.coral, RGB.pink, RGB.violet, RGB.cyan];
  for (let j = 12; j >= 1; j--) {
    const sh = shapeAt(t - j * .03);
    const q = j / 12, ci = q * (trail.length - 1), col = mixRGB(trail[Math.floor(ci)], trail[Math.min(trail.length - 1, Math.floor(ci) + 1)], ci % 1);
    c.strokeStyle = rgba(col, (1 - q) * .85); c.lineWidth = 2.5;
    tracePath(c, sh, CX, CY, 1 + j * .045); c.stroke();
  }
  const sh = shapeAt(t);
  c.fillStyle = C.coral; tracePath(c, sh, CX, CY); c.fill();
  // inner counter-rotating form
  const inner = { r: sh.r, rot: -sh.rot * 1.5, sc: sh.sc };
  c.strokeStyle = C.ink; c.lineWidth = 3; tracePath(c, inner, CX, CY, .52); c.stroke();
  c.fillStyle = C.ink; tracePath(c, inner, CX, CY, .16); c.fill();
  // singularity flash
  const fl = E.inExpo(prog(t, 5.42, SCENE_T(3)));
  if (fl > 0) {
    const g = c.createRadialGradient(CX, CY, 0, CX, CY, 60 + fl * 500);
    g.addColorStop(0, `rgba(255,250,240,${fl})`); g.addColorStop(1, 'rgba(255,250,240,0)');
    c.fillStyle = g; c.fillRect(0, 0, W, H);
  }
  // labels
  const names = ['CIRCLE', 'SQUARE', 'TRIANGLE', 'STAR'];
  let idx = 0; MORPHS.forEach((m, k) => { if (t >= m) idx = k + 1; });
  setFont(c, '500 22px Mono', '3px'); c.fillStyle = 'rgba(244,239,228,.75)'; c.textAlign = 'center';
  c.fillText(`(02) MORPH → ${names[idx]}  ·  ${NA} VERTS`, CX, H - 170);
}

// =================================================================== SCENE 4 — PARTICLES
let PARTS = null;
function initParticles() {
  const [, tx] = mk(W, H);
  tx.fillStyle = '#fff'; setFont(tx, '900 330px Inter', '-6px'); tx.textAlign = 'center'; tx.textBaseline = 'middle';
  tx.fillText('CLAUDE', CX, CY + 6);
  const d = tx.getImageData(0, 0, W, H).data, pts = [];
  for (let y = 0; y < H; y += 4) for (let x = 0; x < W; x += 4) if (d[(y * W + x) * 4 + 3] > 140) pts.push([x, y]);
  const rnd = mulberry(42);
  for (let i = pts.length - 1; i > 0; i--) { const j = Math.floor(rnd() * (i + 1)); [pts[i], pts[j]] = [pts[j], pts[i]]; }
  let minX = 1e9, maxX = 0; pts.forEach(p => { minX = Math.min(minX, p[0]); maxX = Math.max(maxX, p[0]); });
  const N = Math.min(5200, pts.length);
  PARTS = [];
  for (let i = 0; i < N; i++) {
    const r = rnd();
    PARTS.push({
      tx: pts[i][0], ty: pts[i][1], ang: rnd() * TAU, dist: 180 + Math.sqrt(rnd()) * 1000,
      delay: (pts[i][0] - minX) / (maxX - minX) * .32 + rnd() * .08, exitD: rnd() * .12,
      col: r < .62 ? 0 : r < .8 ? 1 : r < .9 ? 2 : 3, size: 2.2 + rnd() * 2.6, ph: rnd() * TAU,
    });
  }
}
const PCOL = [C.cream, C.coral, C.violet, C.cyan];
function s4(t) {
  const c = S, T0 = SCENE_T(3);
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  const g = c.createRadialGradient(CX, CY, 0, CX, CY, 900);
  g.addColorStop(0, 'rgba(122,92,255,.16)'); g.addColorStop(1, 'rgba(0,0,0,0)');
  c.fillStyle = g; c.fillRect(0, 0, W, H);

  const b = E.outExpo(prog(t, T0, T0 + .9)), sw = 1.3 * E.outCubic(prog(t, T0, T0 + 1));
  const scanX = lerp(-200, W + 200, E.inOutCubic(prog(t, 6.62, 7.02)));
  const groups = [[], [], [], [], []];
  for (const p of PARTS) {
    const a = p.ang + sw;
    let x = CX + Math.cos(a) * p.dist * b, y = CY + Math.sin(a) * p.dist * b * .72;
    const f = E.inOutCubic(prog(t, 5.98 + p.delay, 6.55 + p.delay));
    x = lerp(x, p.tx, f); y = lerp(y, p.ty, f);
    x += Math.sin(t * 5 + p.ph) * 1.4 * f; y += Math.cos(t * 4 + p.ph) * 1.4 * f;
    const v = E.inCubic(prog(t, 7.0 + p.exitD, SCENE_T(4)));
    if (v > 0) {
      const dx = x - CX, dy = y - CY, ang = Math.atan2(dy, dx) + v * 5, rr = Math.hypot(dx, dy) * (1 - v);
      x = CX + Math.cos(ang) * rr; y = CY + Math.sin(ang) * rr;
    }
    const lit = Math.abs(x - scanX) < 26;
    groups[lit ? 4 : p.col].push(x, y, p.size * (lit ? 1.6 : 1));
  }
  c.globalCompositeOperation = 'lighter';
  const gc = [...PCOL, C.lime];
  groups.forEach((gr, k) => {
    c.fillStyle = gc[k];
    for (let i = 0; i < gr.length; i += 3) { const s = gr[i + 2]; c.fillRect(gr[i] - s / 2, gr[i + 1] - s / 2, s, s); }
  });
  c.globalCompositeOperation = 'source-over';
  if (scanX > -100 && scanX < W + 100) { c.fillStyle = 'rgba(214,255,62,.9)'; c.fillRect(scanX - 1, CY - 230, 2, 460); }
  // opening flash from the singularity
  const fl = 1 - E.outCubic(prog(t, T0, T0 + .35));
  if (fl > 0) { const gg = c.createRadialGradient(CX, CY, 0, CX, CY, 900); gg.addColorStop(0, `rgba(255,250,240,${fl})`); gg.addColorStop(1, 'rgba(255,250,240,0)'); c.fillStyle = gg; c.fillRect(0, 0, W, H); }
  setFont(c, '500 22px Mono', '3px'); c.fillStyle = 'rgba(244,239,228,.75)'; c.textAlign = 'center';
  const n = Math.round(PARTS.length * E.outExpo(prog(t, T0, T0 + 1)));
  c.globalAlpha = 1 - prog(t, 7.0, 7.3);
  c.fillText(`(03) PARTICLES × ${n.toLocaleString('en-US')}  ·  SPRING → TYPE`, CX, H - 170);
  c.globalAlpha = 1;
}

// =================================================================== SCENE 5 — DIMENSION
const NS = 1700, SPH = [], KNOT = [], SPK = [], RINGS = [], STARS = [];
(function init3D() {
  const rnd = mulberry(9), ga = Math.PI * (3 - Math.sqrt(5));
  for (let i = 0; i < NS; i++) {
    const y = 1 - (i / (NS - 1)) * 2, r = Math.sqrt(1 - y * y), th = ga * i;
    SPH.push([Math.cos(th) * r * 330, y * 330, Math.sin(th) * r * 330]);
    const u = i / NS * TAU, p = 2, q = 3, rr = Math.cos(q * u) + 2.2;
    KNOT.push([rr * Math.cos(p * u) * 118, rr * Math.sin(p * u) * 118, -Math.sin(q * u) * 170]);
    SPK.push(rnd() ** 3);
  }
  const conf = [[560, 1.25, .2, RGB.lime], [650, 1.4, -.5, RGB.cyan], [740, 1.1, .9, RGB.coral]];
  conf.forEach(([R, tx, tz, col], k) => { for (let i = 0; i < 160; i++) RINGS.push({ k, a: i / 160 * TAU, R, tx, tz, col }); });
  for (let i = 0; i < 380; i++) STARS.push([rnd() * W, rnd() * H, rnd()]);
})();
function rot3(p, rx, ry, rz = 0) {
  let [x, y, z] = p, c, s;
  c = Math.cos(rz); s = Math.sin(rz); [x, y] = [x * c - y * s, x * s + y * c];
  c = Math.cos(ry); s = Math.sin(ry); [x, z] = [x * c + z * s, -x * s + z * c];
  c = Math.cos(rx); s = Math.sin(rx); [y, z] = [y * c - z * s, y * s + z * c];
  return [x, y, z];
}
function s5(t) {
  const c = S, T0 = SCENE_T(4), FOC = 1650;
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  const fly = E.inExpo(prog(t, 8.95, SCENE_T(5)));
  // warp stars
  c.fillStyle = C.cream;
  for (const [sx, sy, d] of STARS) {
    const k = 1 + fly * 2.5 * (0.5 + d) + (t - T0) * .04 * d;
    const x = CX + (sx - CX) * k, y = CY + (sy - CY) * k;
    const x2 = CX + (sx - CX) * (k - fly * .5), y2 = CY + (sy - CY) * (k - fly * .5);
    c.globalAlpha = .25 + d * .5;
    if (fly > .02) { c.strokeStyle = C.cream; c.lineWidth = 1 + d; c.beginPath(); c.moveTo(x2, y2); c.lineTo(x, y); c.stroke(); }
    else c.fillRect(x, y, 1 + d * 1.5, 1 + d * 1.5);
  }
  c.globalAlpha = 1;
  const camZ = lerp(1500, -250, fly);
  const ry = t * .7 + E.outExpo(prog(t, T0, T0 + .9)) * 2.4, rx = .35 + Math.sin(t * 1.3) * .2;
  const sc = E.outExpo(prog(t, T0, T0 + .6));
  const m = E.inOutExpo(prog(t, 8.44, 8.98));
  const bp = pulse(t - T0, 6) * (1 - m);
  const pal = [RGB.coral, RGB.pink, RGB.violet, RGB.cyan, RGB.coral];
  c.globalCompositeOperation = 'lighter';
  const proj = [];
  for (let i = 0; i < NS; i++) {
    const s = SPH[i], k = KNOT[i], spike = 1 + .55 * bp * SPK[i];
    const p = [lerp(s[0] * spike, k[0], m) * sc, lerp(s[1] * spike, k[1], m) * sc, lerp(s[2] * spike, k[2], m) * sc];
    const [x, y, z] = rot3(p, rx, ry, m * .4);
    const zz = z + camZ;
    if (zz < 30) { proj.push(null); continue; }
    const f = FOC / zz, X2 = CX + x * f, Y2 = CY + y * f;
    proj.push([X2, Y2]);
    const near = clamp((1800 - zz) / 700);
    const ci = (i / NS) * 4, base = mixRGB(pal[Math.floor(ci)], pal[Math.floor(ci) + 1], ci % 1);
    const col = mixRGB(mixRGB(RGB.violet, base, m), RGB.cream, near * .55);
    const sz = Math.min(16, 3.4 * f);
    c.fillStyle = rgba(col, .5 + near * .5);
    c.fillRect(X2 - sz / 2, Y2 - sz / 2, sz, sz);
  }
  // knot filament
  if (m > .4) {
    c.strokeStyle = `rgba(244,239,228,${(m - .4) * .7})`; c.lineWidth = 2.2; c.beginPath();
    let pen = false;
    for (let i = 0; i <= NS; i++) { const p = proj[i % NS]; if (!p) { pen = false; continue; } pen ? c.lineTo(p[0], p[1]) : c.moveTo(p[0], p[1]); pen = true; }
    c.stroke();
  }
  // orbit rings
  const ra = E.outExpo(prog(t, T0 + .25, T0 + 1.1));
  for (const q of RINGS) {
    const a = q.a + t * (q.k % 2 ? -.6 : .45);
    const p = rot3(rot3([Math.cos(a) * q.R * ra, 0, Math.sin(a) * q.R * ra], q.tx, 0, q.tz), rx * .5, ry * .4);
    const zz = p[2] + camZ; if (zz < 30) continue;
    const f = FOC / zz, sz = Math.min(10, 2.2 * f);
    c.fillStyle = rgba(q.col, .75 * ra);
    c.fillRect(CX + p[0] * f - sz / 2, CY + p[1] * f - sz / 2, sz, sz);
  }
  c.globalCompositeOperation = 'source-over';
  setFont(c, '500 22px Mono', '3px'); c.fillStyle = 'rgba(244,239,228,.75)'; c.textAlign = 'center';
  c.globalAlpha = 1 - fly;
  c.fillText(m < .5 ? '(04) FIBONACCI SPHERE · 1,700 PTS · NO ENGINE' : '(04) TORUS KNOT (2,3) · PURE PROJECTION MATH', CX, H - 170);
  c.globalAlpha = 1;
  const wf = E.inExpo(prog(t, 9.2, SCENE_T(5)));
  if (wf > 0) { c.fillStyle = `rgba(255,252,246,${wf})`; c.fillRect(0, 0, W, H); }
}

// =================================================================== SCENE 6 — SYSTEMS (grid montage)
const PANELS = {
  optical(c, w, h, t, p) {
    c.fillStyle = p[0]; c.fillRect(0, 0, w, h);
    c.strokeStyle = p[1]; c.lineWidth = 11;
    const R = Math.hypot(w, h);
    const centers = [[w / 2 + Math.sin(t * 2.2) * 70, h / 2], [w / 2 - Math.sin(t * 2.2) * 70, h / 2 + Math.cos(t * 1.7) * 50]];
    for (const [x, y] of centers) for (let r = (t * 70) % 36; r < R; r += 36) { c.beginPath(); c.arc(x, y, r, 0, TAU); c.stroke(); }
  },
  rhythm(c, w, h, t, p) {
    c.fillStyle = p[0]; c.fillRect(0, 0, w, h);
    const n = 22, bw = w / n;
    c.fillStyle = p[1];
    for (let i = 0; i < n; i++) {
      const v = (.18 + .82 * Math.abs(noise1(t * 3.2 + i * .61))) * (.55 + .45 * pulse(t - i * .012)) * Math.exp(-(((i - n / 2) / (n * .45)) ** 2));
      const bh = Math.max(8, v * h * .7);
      c.beginPath(); c.roundRect(i * bw + bw * .2, h / 2 - bh / 2, bw * .6, bh, bw * .3); c.fill();
    }
  },
  physics(c, w, h, t, p) {
    c.fillStyle = p[0]; c.fillRect(0, 0, w, h);
    const r = Math.min(w, h) * .065, ground = h * .8;
    c.fillStyle = 'rgba(0,0,0,.18)'; c.fillRect(0, ground + r * .15, w, 2);
    for (let k = 0; k < 5; k++) {
      const ph = ((t + k * .09) / BEAT) % 1, hgt = Math.sin(ph * Math.PI);
      const v = Math.cos(ph * Math.PI);
      const contact = Math.max(0, 1 - hgt * 8);
      let sy = 1 + .35 * Math.abs(v) * (1 - contact) - .4 * contact, sx = 1 / sy;
      const x = w * (.18 + k * .16), y = ground - hgt * h * .52 - r * sy;
      c.fillStyle = 'rgba(0,0,0,.2)'; c.beginPath(); c.ellipse(x, ground + r * .2, r * (1.2 - hgt * .6), r * .22 * (1 - hgt * .5), 0, 0, TAU); c.fill();
      c.fillStyle = p[1]; c.beginPath(); c.ellipse(x, y + r * sy - r * sy, r * sx, r * sy, 0, 0, TAU); c.fill();
    }
  },
  flow(c, w, h, t, p) {
    c.fillStyle = p[0]; c.fillRect(0, 0, w, h);
    const n = 17;
    c.lineWidth = 3; c.strokeStyle = p[1];
    for (let k = 0; k < n; k++) {
      const y0 = h * .22 + k * (h * .64 / (n - 1));
      const cx1 = w * (.5 + Math.sin(t * 1.4 + k * .35) * .18), amp = h * .13 * (.6 + .4 * noise1(k * 1.3 + t * 1.5));
      c.beginPath();
      for (let x = 0; x <= w; x += 8) {
        const y = y0 - amp * Math.exp(-(((x - cx1) / (w * .09)) ** 2)) - amp * .45 * Math.exp(-(((x - cx1 - w * .14) / (w * .05)) ** 2));
        x ? c.lineTo(x, y) : c.moveTo(x, y);
      }
      c.lineTo(w, h); c.lineTo(0, h); c.closePath();
      c.fillStyle = p[0]; c.fill(); c.stroke();
    }
  },
};
const CELL_DEFS = [
  ['optical', [C.coral, C.ink], 'A / OPTICS'], ['rhythm', [C.ink, C.lime], 'B / RHYTHM'],
  ['physics', [C.violet, C.cream], 'C / PHYSICS'], ['flow', [C.cream, C.ink], 'D / FLOW'],
];
const ALT = [
  ['rhythm', [C.coral, C.ink]], ['flow', [C.ink, C.lime]], ['optical', [C.lime, C.ink]], ['physics', [C.cyan, C.ink]],
  ['flow', [C.violet, C.cream]], ['optical', [C.ink, C.violet]], ['physics', [C.cream, C.coral]], ['rhythm', [C.pink, C.ink]],
  ['optical', [C.cyan, C.ink]], ['physics', [C.ink, C.coral]], ['rhythm', [C.cream, C.violet]], ['flow', [C.lime, C.ink]],
];
function s6(t) {
  const c = S, T0 = SCENE_T(5), b1 = T0 + BEAT, b2 = b1 + BEAT, b3 = b2 + BEAT;
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  const e1 = E.outExpo(prog(t, b1, b1 + .42)), e2 = E.outExpo(prog(t, b2, b2 + .42)), e3 = E.inOutExpo(prog(t, b3, b3 + .5));
  const sx = lerp(W, W / 2, e1), sy = lerp(H, H / 2, e2);
  const z = lerp(1, .5, e3) * (1 - .05 * prog(t, b3, SCENE_T(6)));
  c.save();
  c.translate(CX, CY); c.rotate(-.07 * e3); c.scale(z, z); c.translate(-CX, -CY);
  const g = 22 * e3 / z, rad = 26 * e3 / z;
  const cells = [
    [0, 0, sx, sy, 0], [sx, 0, W - sx, sy, 1], [0, sy, sx, H - sy, 2], [sx, sy, W - sx, H - sy, 3],
  ];
  if (t >= b3) {
    let a = 0;
    for (let i = -1; i <= 2; i++) for (let j = -1; j <= 2; j++) {
      if ((i === 0 || i === 1) && (j === 0 || j === 1)) continue;
      cells.push([i * W / 2, j * H / 2, W / 2, H / 2, 4 + a++]);
    }
  }
  for (const [x, y, w, h, id] of cells) {
    if (w < 1 || h < 1) continue;
    const gx = (x + w / 2 - CX) / (W / 2), gy = (y + h / 2 - CY) / (H / 2);
    const d = Math.hypot(gx, gy);
    const flip = 1 - E.inCubic(prog(t, 10.93 + d * .06, 10.93 + d * .06 + .2));
    if (flip <= 0) continue;
    c.save();
    c.translate(x + w / 2, y + h / 2); c.scale(1, flip); c.translate(-w / 2, -h / 2);
    c.beginPath(); c.roundRect(g / 2, g / 2, w - g, h - g, rad); c.clip();
    c.translate(g / 2, g / 2);
    const iw = w - g, ih = h - g;
    const def = id < 4 ? CELL_DEFS[id] : ALT[(id - 4) % ALT.length];
    PANELS[def[0]](c, iw, ih, t + id * .37, def[1]);
    if (id < 4) {
      setFont(c, '500 20px Mono', '3px'); c.textAlign = 'left'; c.fillStyle = def[1][1];
      c.globalAlpha = .85; c.fillText(def[2], 34, ih - 34); c.globalAlpha = 1;
    }
    c.restore();
  }
  c.restore();
  const wf = 1 - E.outCubic(prog(t, T0, T0 + .3));
  if (wf > 0) { c.fillStyle = `rgba(255,252,246,${wf})`; c.fillRect(0, 0, W, H); }
}

// =================================================================== SCENE 7 — DATA
function s7(t) {
  const c = S, T0 = SCENE_T(6);
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  const cols = [
    { x: W * .2, t0: T0, from: 0, to: 900, fmt: n => String(Math.round(n)).padStart(3, '0'), label: 'FRAMES RENDERED', g: 'bars', col: C.cream },
    { x: W * .5, t0: T0 + BEAT, from: 999, to: 0, fmt: n => String(Math.round(n)).padStart(3, '0'), label: 'KEYFRAMES USED', g: 'line', col: C.coral },
    { x: W * .8, t0: T0 + 2 * BEAT, from: 0, to: 100, fmt: n => Math.round(n) + '%', label: 'WRITTEN IN CODE', g: 'ring', col: C.lime },
  ];
  // title
  const ti = E.outExpo(prog(t, T0, T0 + .6)), to = E.inExpo(prog(t, 12.62, 12.9));
  setFont(c, '500 22px Mono', '4px'); c.textAlign = 'left'; c.fillStyle = C.lime;
  c.globalAlpha = 1 - to;
  c.fillText(typed('(05) BY THE NUMBERS', ti), 150, 170);
  c.fillStyle = 'rgba(244,239,228,.25)'; c.fillRect(150, 196, (W - 300) * ti * (1 - to), 1);
  c.globalAlpha = 1;
  cols.forEach((o, i) => {
    const rev = E.outExpo(prog(t, o.t0, o.t0 + .7)), cnt = E.outExpo(prog(t, o.t0, o.t0 + 1.0));
    const ex = E.inExpo(prog(t, 12.62 + i * .05, 12.92 + i * .05));
    if (rev <= 0) return;
    c.save(); c.translate(0, -ex * 500); c.globalAlpha = 1 - ex;
    // divider
    if (i > 0) { c.fillStyle = 'rgba(244,239,228,.18)'; c.fillRect(o.x - W * .15, 260, 1, 560 * rev); }
    // graphic
    const gy = 400;
    if (o.g === 'bars') {
      const rnd = mulberry(3);
      for (let k = 0; k < 10; k++) {
        const hh = (40 + rnd() * 120) * E.outBack(prog(t, o.t0 + k * .04, o.t0 + k * .04 + .5));
        c.fillStyle = k % 3 === 2 ? C.coral : C.cream; c.fillRect(o.x - 160 + k * 32, gy + 60 - hh, 20, hh);
      }
    } else if (o.g === 'line') {
      const rnd = mulberry(11), pts = []; let v = 0;
      for (let k = 0; k < 28; k++) { v += (rnd() - .45) * 40; pts.push([o.x - 180 + k * (360 / 27), gy - v * .6]); }
      const L = E.inOutCubic(prog(t, o.t0 + .05, o.t0 + .8)) * (pts.length - 1);
      c.strokeStyle = C.coral; c.lineWidth = 4; c.lineJoin = 'round'; c.beginPath();
      for (let k = 0; k <= Math.floor(L); k++) k ? c.lineTo(...pts[k]) : c.moveTo(...pts[k]);
      const k0 = Math.floor(L), fr = L - k0, p1 = pts[Math.min(k0 + 1, pts.length - 1)];
      const hx = lerp(pts[k0][0], p1[0], fr), hy = lerp(pts[k0][1], p1[1], fr);
      c.lineTo(hx, hy); c.stroke();
      c.fillStyle = C.cream; c.beginPath(); c.arc(hx, hy, 9 + 4 * pulse(t), 0, TAU); c.fill();
    } else {
      c.lineWidth = 16; c.strokeStyle = 'rgba(244,239,228,.12)';
      c.beginPath(); c.arc(o.x, gy, 90, 0, TAU); c.stroke();
      c.strokeStyle = C.lime; c.lineCap = 'round';
      c.beginPath(); c.arc(o.x, gy, 90, -Math.PI / 2, -Math.PI / 2 + TAU * cnt * .999); c.stroke(); c.lineCap = 'butt';
      c.save(); c.translate(o.x, gy); c.rotate(t * 2);
      c.fillStyle = C.lime; for (let k = 0; k < 3; k++) { c.rotate(TAU / 3); c.fillRect(-4, -40, 8, 18); }
      c.restore();
    }
    // number
    c.save(); c.beginPath(); c.rect(o.x - 300, 500, 600, 190); c.clip();
    setFont(c, '800 170px Mono', '-4px'); c.textAlign = 'center'; c.fillStyle = o.col;
    c.fillText(o.fmt(lerp(o.from, o.to, cnt)), o.x, 660 + (1 - rev) * 190);
    c.restore();
    setFont(c, '500 22px Mono', '4px'); c.fillStyle = 'rgba(244,239,228,.6)'; c.textAlign = 'center';
    c.fillText(typed(o.label, prog(t, o.t0 + .15, o.t0 + .55)), o.x, 750);
    c.restore();
  });
  // lime wipe
  const x1 = W * E.inOutExpo(prog(t, 12.72, 13.0)), x0 = W * E.inOutExpo(prog(t, 12.9, SCENE_T(7)));
  if (x1 > x0) { c.fillStyle = C.lime; c.fillRect(x0, 0, x1 - x0, H); }
}

// =================================================================== SCENE 8 — SIGNATURE
const DOT_T = 13.95, DOT_D = .6;
function s8(t) {
  const c = S, T0 = SCENE_T(7);
  c.fillStyle = C.ink; c.fillRect(0, 0, W, H);
  // gradient blobs
  const ba = E.outCubic(prog(t, T0, T0 + .8));
  c.globalCompositeOperation = 'lighter';
  [[RGB.coral, .3, .38, .7], [RGB.violet, .72, .62, 1.1], [RGB.cyan, .55, .2, .9]].forEach(([col, fx, fy, sp], k) => {
    const x = W * fx + Math.sin(t * sp + k) * 180, y = H * fy + Math.cos(t * sp * .8 + k * 2) * 120;
    const g = c.createRadialGradient(x, y, 0, x, y, 760);
    g.addColorStop(0, rgba(col, .26 * ba)); g.addColorStop(1, rgba(col, 0));
    c.fillStyle = g; c.fillRect(0, 0, W, H);
  });
  c.globalCompositeOperation = 'source-over';
  // CLAUDE
  setFont(c, '900 250px Inter', '-6px');
  const name = 'CLAUDE', xs = charXs(c, name), tot = xs[6];
  c.textAlign = 'left'; c.fillStyle = C.cream;
  for (let i = 0; i < 6; i++) {
    const p = prog(t, T0 + .02 + i * .05, T0 + .02 + i * .05 + 1.0);
    if (p <= 0) continue;
    const e = E.outElastic(p);
    c.save();
    const lx = CX - tot / 2 + xs[i], lw = xs[i + 1] - xs[i];
    c.translate(lx + lw / 2, 560 - (1 - e) * 420);
    c.rotate((1 - e) * .5 * (i % 2 ? 1 : -1));
    c.globalAlpha = Math.min(1, p * 5);
    c.fillText(name[i], -lw / 2, 0);
    c.restore();
  }
  // serif subtitle, blur-in per letter
  setFont(c, 'italic 400 128px Serif');
  const sub = 'motion designer', sx = charXs(c, sub), sw = sx[sub.length];
  const subX = CX - sw / 2 - 18, subY = 720;
  c.fillStyle = C.coral;
  for (let i = 0; i < sub.length; i++) {
    const e = E.outCubic(prog(t, 13.5 + i * .022, 13.5 + i * .022 + .5));
    if (e <= 0) continue;
    c.save(); c.globalAlpha = e; c.filter = `blur(${((1 - e) * 14).toFixed(1)}px)`;
    c.fillText(sub[i], subX + sx[i], subY + (1 - e) * 40);
    c.restore();
  }
  // underline
  const ul = E.inOutExpo(prog(t, 13.95, 14.4));
  c.fillStyle = 'rgba(244,239,228,.5)'; c.fillRect(CX - tot / 2, 770, tot * ul, 2);
  // the dot from frame one comes home: bounces in as the full stop
  if (t > DOT_T) {
    const p = prog(t, DOT_T, DOT_T + DOT_D), yb = E.outBounce(p);
    const gy = subY - 12, y = lerp(-60, gy, yb);
    const contact = Math.exp(-Math.abs(1 - yb) * 60) * (1 - p * .7);
    const r = 13, sxq = 1 + .45 * contact, syq = 1 - .4 * contact;
    c.fillStyle = C.cream; c.beginPath(); c.ellipse(subX + sw + 26, y + r * (1 - syq), r * sxq, r * syq, 0, 0, TAU); c.fill();
  }
  // tagline
  setFont(c, '500 22px Mono', '4px'); c.textAlign = 'center'; c.fillStyle = 'rgba(244,239,228,.62)';
  c.fillText(typed('SHOWREEL 2026  ·  EVERY FRAME GENERATED IN CODE  ·  15s @ 60FPS', prog(t, 14.15, 14.7)), CX, 850);
  // impact flash
  const fl = Math.exp(-(t - T0) * 9);
  c.fillStyle = `rgba(255,252,246,${.85 * fl})`; c.fillRect(0, 0, W, H);
  const fo = E.inCubic(prog(t, 14.8, 15));
  if (fo > 0) { c.fillStyle = `rgba(11,11,16,${fo})`; c.fillRect(0, 0, W, H); }
}

// =================================================================== POST + HUD
const SCENES = [s1, s2, s3, s4, s5, s6, s7, s8];
const NAMES = ['GENESIS', 'TYPE', 'FORM', 'PARTICLES', 'DIMENSION', 'SYSTEMS', 'DATA', 'SIGNATURE'];
const BLOOM = [.5, 0, .55, .9, .9, .15, .35, .6];
const GRAIN = [];
for (let k = 0; k < 6; k++) {
  const [g, gx] = mk(256, 256), id = gx.createImageData(256, 256), rnd = mulberry(100 + k);
  for (let i = 0; i < id.data.length; i += 4) { const v = rnd() * 255; id.data[i] = id.data[i + 1] = id.data[i + 2] = v; id.data[i + 3] = 255; }
  gx.putImageData(id, 0, 0); GRAIN.push(X.createPattern(g, 'repeat'));
}
const VIG = X.createRadialGradient(CX, CY, H * .35, CX, CY, H * 1.05);
VIG.addColorStop(0, 'rgba(0,0,0,0)'); VIG.addColorStop(1, 'rgba(0,0,0,.42)');

function hud(t, f) {
  const c = X;
  const a = prog(t, .25, .7) * (1 - .75 * prog(t, SCENE_T(7), SCENE_T(7) + .3)) * (1 - prog(t, 14.8, 15));
  if (a <= 0) return;
  c.save();
  c.globalCompositeOperation = 'difference';
  c.globalAlpha = a; c.fillStyle = c.strokeStyle = C.cream; c.lineWidth = 2;
  const m = 40, L = 28;
  [[m, m, 1, 1], [W - m, m, -1, 1], [m, H - m, 1, -1], [W - m, H - m, -1, -1]].forEach(([x, y, dx, dy]) => {
    c.beginPath(); c.moveTo(x + dx * L, y); c.lineTo(x, y); c.lineTo(x, y + dy * L); c.stroke();
  });
  setFont(c, '500 18px Mono', '3px'); c.textBaseline = 'middle';
  c.textAlign = 'left'; c.fillText('CLAUDE — MOTION REEL ’26', 84, 68);
  const secs = Math.floor(f / FPS), ff = f % FPS;
  c.textAlign = 'right'; c.fillText(`TC 00:00:${String(secs).padStart(2, '0')}:${String(ff).padStart(2, '0')}`, W - 84, 68);
  const si = Math.min(7, Math.floor(t / BAR));
  c.textAlign = 'left'; c.fillText(`${String(si + 1).padStart(2, '0')}/08 — ${NAMES[si]}`, 84, H - 68);
  c.textAlign = 'right'; c.fillText('128 BPM', W - 84 - 4 * 22 - 14, H - 68);
  const bi = Math.floor(t / BEAT) % 4;
  for (let k = 0; k < 4; k++) {
    const x = W - 84 - (3 - k) * 22 - 12;
    k === bi ? c.fillRect(x, H - 74, 12, 12) : c.strokeRect(x + 1, H - 73, 10, 10);
  }
  c.globalAlpha = a * .5; c.fillRect(84, H - 44, (W - 168) * (t / DUR), 2);
  c.restore();
}

function render(t, f) {
  t = clamp(t, 0, DUR - 1e-6);
  if (f === undefined) f = Math.floor(t * FPS);
  const si = Math.min(7, Math.floor(t / BAR));
  S.setTransform(1, 0, 0, 1, 0, 0); S.globalAlpha = 1; S.globalCompositeOperation = 'source-over'; S.filter = 'none';
  S.save(); SCENES[si](t); S.restore();

  const imp = impact(t);
  const sh = imp * 14, ox = noise1(t * 38) * sh, oy = noise1(t * 38 + 77) * sh;
  const ca = imp * 18, mg = sh + ca + 2;
  X.setTransform(1, 0, 0, 1, 0, 0); X.globalAlpha = 1; X.filter = 'none';
  X.globalCompositeOperation = 'source-over';
  if (ca > .6) {
    X.fillStyle = '#000'; X.fillRect(0, 0, W, H);
    [['#ff0000', -ca], ['#00ff00', 0], ['#0000ff', ca]].forEach(([col, dx]) => {
      CHX.globalCompositeOperation = 'copy'; CHX.drawImage(SC, 0, 0);
      CHX.globalCompositeOperation = 'multiply'; CHX.fillStyle = col; CHX.fillRect(0, 0, W, H);
      X.globalCompositeOperation = 'lighter';
      X.drawImage(CH, ox - mg + dx, oy - mg, W + 2 * mg, H + 2 * mg);
    });
  } else {
    X.drawImage(SC, ox - mg, oy - mg, W + 2 * mg, H + 2 * mg);
  }
  // bloom
  const bl = BLOOM[si] * (si === 1 ? 0 : 1) + imp * .25;
  if (bl > .02) {
    B.globalCompositeOperation = 'copy'; B.drawImage(SC, 0, 0, W / 4, H / 4);
    B2.globalCompositeOperation = 'copy'; B2.filter = 'blur(9px) brightness(1.1)'; B2.drawImage(BL, 0, 0); B2.filter = 'none';
    X.globalCompositeOperation = 'lighter'; X.globalAlpha = Math.min(1, bl);
    X.drawImage(BL2, 0, 0, W, H);
    X.globalAlpha = 1;
  }
  // grade: vignette + grain
  X.globalCompositeOperation = 'source-over'; X.fillStyle = VIG; X.fillRect(0, 0, W, H);
  X.globalCompositeOperation = 'overlay'; X.globalAlpha = .075;
  X.fillStyle = GRAIN[f % GRAIN.length];
  X.save(); X.translate((f * 37) % 256, (f * 91) % 256); X.fillRect(-256, -256, W + 512, H + 512); X.restore();
  X.globalAlpha = 1; X.globalCompositeOperation = 'source-over';
  hud(t, f);
}

// motion blur: average `sub` renders spread across `shutter` of the frame interval
const [OUT, OX] = mk(W, H);
let ACC = null, OUTID = null;
function frame(f, sub = 4, shutter = .5) {
  if (!ACC) { ACC = new Float32Array(W * H * 4); OUTID = OX.createImageData(W, H); }
  ACC.fill(0);
  for (let k = 0; k < sub; k++) {
    render((f + (k / sub) * shutter) / FPS, f);
    const d = X.getImageData(0, 0, W, H).data;
    for (let i = 0; i < d.length; i++) ACC[i] += d[i];
  }
  const o = OUTID.data, inv = 1 / sub;
  for (let i = 0; i < o.length; i++) o[i] = ACC[i] * inv + .5;
  OX.putImageData(OUTID, 0, 0);
  return OUT.toDataURL('image/png');
}

// =================================================================== boot
window.render = render;
window.frame = frame;
window.META = { W, H, FPS, DUR, BPM };
document.fonts.load('900 100px Inter').then(() => Promise.all([
  document.fonts.load('italic 400 100px Serif'), document.fonts.load('500 20px Mono'), document.fonts.load('800 20px Mono'),
])).then(() => {
  initParticles();
  window.READY = true;
  const q = new URLSearchParams(location.search);
  if (q.has('t')) { render(parseFloat(q.get('t'))); return; }
  if (q.has('render')) return;
  document.body.classList.add('preview');
  const start = performance.now();
  const loop = now => { render(((now - start) / 1000) % DUR); requestAnimationFrame(loop); };
  requestAnimationFrame(loop);
});
