"""Synthesizes the 15s, 128 BPM soundtrack, hit-synced to the reel. Writes a 48kHz stereo WAV."""
import sys, wave
import numpy as np

SR, DUR, BPM = 48000, 15.0, 128
BEAT = 60 / BPM
BAR = BEAT * 4
N = int(SR * DUR)
L = np.zeros(N); R = np.zeros(N)
rng = np.random.default_rng(7)
duck = np.ones(N)


def add(sig, t0, gain=1.0, pan=0.0):
    i = int(t0 * SR)
    if i >= N: return
    sig = sig[: N - i]
    L[i:i + len(sig)] += sig * gain * np.sqrt(0.5 * (1 - pan)) * 1.414
    R[i:i + len(sig)] += sig * gain * np.sqrt(0.5 * (1 + pan)) * 1.414


def tt(d): return np.arange(int(d * SR)) / SR


def onepole(x, cutoff):
    """One-pole lowpass; cutoff may be an array (per-sample)."""
    cutoff = np.broadcast_to(np.asarray(cutoff, float), x.shape)
    a = 1 - np.exp(-2 * np.pi * cutoff / SR)
    y = np.empty_like(x); s = 0.0
    for i in range(len(x)):
        s += a[i] * (x[i] - s); y[i] = s
    return y


def hp(x, c): return x - onepole(x, c)


def midi(n): return 440 * 2 ** ((n - 69) / 12)


def saw(f, t, detune=0.0):
    ph = (f * (1 + detune) * t + rng.random()) % 1
    return 2 * ph - 1


# ---------------- drums
def kick(g=1.0):
    t = tt(0.55)
    f = 44 + 120 * np.exp(-t * 32)
    ph = 2 * np.pi * np.cumsum(f) / SR
    return (np.sin(ph) * np.exp(-t * 6.5) + 0.25 * np.exp(-t * 400) * rng.standard_normal(len(t))) * g


def clap():
    t = tt(0.35); n = rng.standard_normal(len(t))
    env = np.exp(-t * 16) + 0.6 * np.exp(-((t - 0.012) % 0.011) * 300) * (t < 0.035)
    return hp(n, 900) * env * 0.5 + np.sin(2 * np.pi * 190 * t) * np.exp(-t * 30) * 0.3


def hat(d=0.05):
    t = tt(d); return hp(rng.standard_normal(len(t)), 7000) * np.exp(-t * 70)


def boom(g=1.0, d=2.2):
    t = tt(d); f = 30 + 60 * np.exp(-t * 6)
    s = np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-t * 1.6)
    n = hp(rng.standard_normal(len(t)), 2500) * np.exp(-t * 2.2) * 0.35
    return (s + n) * g


def whoosh(d, g=1.0, up=True):
    t = tt(d); x = rng.standard_normal(len(t))
    p = t / d if up else 1 - t / d
    y = hp(onepole(x, 300 + 9000 * p ** 2), 200) * (p ** 2.2)
    return y * g


def blip(f0, f1, d=0.09):
    t = tt(d); f = f0 * (f1 / f0) ** (t / d)
    return np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-t * 35)


def pluck(f, d=0.28):
    t = tt(d)
    return (np.sin(2 * np.pi * f * t) + 0.35 * np.sin(4 * np.pi * f * t) + 0.12 * saw(f, t)) * np.exp(-t * 14)


beats = [i * BEAT for i in range(32)]
for i, b in enumerate(beats):
    if i >= 28: break
    if i < 4:
        if i < 2: add(kick(0.55), b)
        continue
    add(kick(1.0), b)
    d = np.ones(int(0.3 * SR)); d = 1 - 0.75 * np.exp(-np.arange(len(d)) / SR * 12)
    j = int(b * SR); duck[j:j + len(d)] = np.minimum(duck[j:j + len(d)], d[: N - j])
    if i >= 8 and i % 2 == 1: add(clap(), b, 0.8)
    if i >= 4: add(hat(), b + BEAT / 2, 0.35, 0.3)
    if 20 <= i < 28:
        for k in (1, 3): add(hat(0.03), b + k * BEAT / 4, 0.22, -0.4)

# ---------------- harmony: Am  F  C  G  Am  F  (bars 1..6), resolve on the final hit
CHORDS = {0: [57, 60, 64], 1: [53, 57, 60], 2: [48, 52, 55], 3: [55, 59, 62]}
prog_ = [0, 1, 2, 3, 0, 1]
ROOT = {0: 33, 1: 29, 2: 36, 3: 31}
bass = np.zeros(N); pad = np.zeros((2, N))
for bi, ch in enumerate(prog_):
    t0 = (bi + 1) * BAR
    t = tt(BAR)
    # pad
    for n in CHORDS[ch] + [CHORDS[ch][0] + 12]:
        for s, det in ((0, -0.004), (1, 0.004)):
            x = saw(midi(n), t, det) + saw(midi(n), t, -det * 0.5)
            i = int(t0 * SR); pad[s, i:i + len(t)] += x[: N - i] * 0.05
    # bass on 8ths
    for k in range(8):
        tb = tt(BEAT / 2)
        f = midi(ROOT[ch] + (12 if k % 4 == 3 else 0))
        x = (saw(f, tb) + np.sin(2 * np.pi * f * tb)) * np.exp(-tb * 5)
        i = int((t0 + k * BEAT / 2) * SR); bass[i:i + len(x)] += x[: N - i] * 0.3
# filter sweeps
cut = 400 + 3800 * np.clip((np.arange(N) / SR - BAR) / (6 * BAR), 0, 1) ** 1.5
pad[0] = onepole(onepole(pad[0], cut), cut); pad[1] = onepole(onepole(pad[1], cut), cut)
bass = onepole(bass, 900)
L += (pad[0] + bass) * duck; R += (pad[1] + bass) * duck

# arps across the grid montage
for k in range(32):
    t0 = SCENE = 5 * BAR + k * BEAT / 4
    ch = CHORDS[prog_[4 if k < 16 else 5]]
    n = (ch + [c + 12 for c in ch])[[0, 1, 2, 3, 4, 5, 4, 2][k % 8]] + 12
    add(pluck(midi(n)), t0, 0.12, (-0.5 if k % 2 else 0.5))

# ---------------- sound design, synced to picture
add(boom(0.55, 1.2), 0.0)                                    # cold open
for k in range(28): add(blip(1800, 2400, 0.012), 0.12 + k * 0.021, 0.08)   # typewriter
add(whoosh(0.5, 0.6), 0.94 - 0.2)                             # the smear
add(whoosh(0.45, 0.5), BAR - 0.45)                             # stripes
for k in range(16): add(blip(900 + 120 * k, 700 + 120 * k, 0.02), 2.72 + k * 0.03, 0.12)  # slot roll
add(whoosh(0.4, 0.7), 3.35)                                    # iris
for tb in (2 * BAR + BEAT, 2 * BAR + 2 * BEAT, 2 * BAR + 3 * BEAT): add(blip(500, 1400, 0.12), tb, 0.3)
add(whoosh(0.3, 0.6), 5.33)                                    # collapse
for k in range(90):                                            # particle shimmer
    tk = 3 * BAR + rng.random() ** 1.6 * 1.2
    add(np.sin(2 * np.pi * rng.uniform(2500, 6500) * tt(0.12)) * np.exp(-tt(0.12) * 40), tk, 0.05, rng.uniform(-.8, .8))
add(whoosh(0.9, 0.9), 6.6)                                     # riser into 3D
add(whoosh(0.5, 0.9), 8.9)                                     # fly-through
for k in range(24): add(blip(1200, 1300, 0.015), 6 * BAR + k * 0.04, 0.06)   # counters
add(whoosh(0.5, 0.7), 12.65)                                   # lime wipe
riser_t = tt(1.4); add(np.sin(2 * np.pi * np.cumsum(200 + 800 * (riser_t / 1.4) ** 3) / SR) * (riser_t / 1.4) ** 2, 13.125 - 1.4, 0.12)
for tb, g in [(BAR, .35), (2 * BAR, .5), (3 * BAR, .7), (4 * BAR, .6), (5 * BAR, .6), (6 * BAR, .4)]: add(boom(g, 1.2), tb)

# the signature hit: boom + big A minor chord ringing out
T8 = 7 * BAR
add(boom(1.1, 2.0), T8); add(kick(1.2), T8)
tc = tt(DUR - T8)
chord = sum(saw(midi(n), tc, d) for n in (45, 57, 60, 64, 67, 71) for d in (-.005, .005)) * 0.03
chord = onepole(chord, 2600 * np.exp(-tc * 0.9) + 300) * np.exp(-tc * 0.9)
add(chord, T8, 1.0)
for k, tb in enumerate([13.95 + 0.6 * p for p in (0.3636, 0.7273, 0.9091, 1.0)]):  # bouncing dot
    add(blip(700 + k * 90, 500 + k * 90, 0.1), tb, 0.35 * (1 - k * 0.2))

# ---------------- master
mix = np.stack([L, R])
mix = hp(mix[0], 25), hp(mix[1], 25)
mix = np.stack(mix)
mix /= np.max(np.abs(mix)) + 1e-9
mix = np.tanh(mix * 1.6) / np.tanh(1.6)
fade = np.ones(N); fl = int(0.2 * SR); fade[-fl:] = np.linspace(1, 0, fl)
mix *= fade * 0.9
out = (np.clip(mix.T, -1, 1) * 32767).astype('<i2')
with wave.open(sys.argv[1] if len(sys.argv) > 1 else 'audio.wav', 'wb') as w:
    w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR); w.writeframes(out.tobytes())
print('ok')
