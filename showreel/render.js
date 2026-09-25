// Renders index.html frame-by-frame with headless Chromium, then encodes with ffmpeg.
//   node render.js stills out/ 0.5 2.3 ...    -> PNG stills at given times
//   node render.js video out.mp4 [audio.wav]  -> 60fps H.264, sub-frame motion blur, parallel workers
const path = require('path');
const fs = require('fs');
const os = require('os');
const { spawnSync } = require('child_process');
const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');

const FFMPEG = process.env.FFMPEG || 'ffmpeg';
const SUB = +(process.env.SUB || 4);           // sub-frames per frame
const SHUTTER = +(process.env.SHUTTER || 0.5); // 180° shutter
const WORKERS = +(process.env.WORKERS || os.cpus().length);

async function openPage(browser) {
  const page = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
  page.on('pageerror', e => { console.error('[page error]', e); process.exit(1); });
  await page.goto('file://' + path.join(__dirname, 'index.html') + '?render');
  await page.waitForFunction('window.READY === true');
  return page;
}
const png = dataUrl => Buffer.from(dataUrl.split(',')[1], 'base64');

(async () => {
  const [mode, out, ...rest] = process.argv.slice(2);
  const browser = await chromium.launch();
  if (mode === 'stills') {
    const page = await openPage(browser);
    fs.mkdirSync(out, { recursive: true });
    for (const t of rest.map(Number)) {
      const url = await page.evaluate(t => { window.render(t); return document.getElementById('c').toDataURL('image/png'); }, t);
      fs.writeFileSync(path.join(out, `t${t.toFixed(3)}.png`), png(url));
    }
  } else {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'reel-'));
    const probe = await openPage(browser);
    const { FPS, DUR } = await probe.evaluate('window.META');
    await probe.close();
    const frames = Math.round(FPS * DUR);
    let next = 0, done = 0;
    const t0 = Date.now();
    await Promise.all(Array.from({ length: WORKERS }, async () => {
      const page = await openPage(browser);
      while (next < frames) {
        const f = next++;
        const url = await page.evaluate(([f, s, sh]) => window.frame(f, s, sh), [f, SUB, SHUTTER]);
        fs.writeFileSync(path.join(dir, `f${String(f).padStart(4, '0')}.png`), png(url));
        if (++done % 60 === 0) console.log(`${done}/${frames} frames  ${((Date.now() - t0) / 1000).toFixed(0)}s`);
      }
    }));
    const audio = rest[0];
    const args = ['-y', '-loglevel', 'error', '-framerate', String(FPS), '-i', path.join(dir, 'f%04d.png')];
    if (audio) args.push('-i', audio, '-c:a', 'aac', '-b:a', '256k', '-shortest');
    args.push('-c:v', 'libx264', '-preset', 'slow', '-crf', '14', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', out);
    const r = spawnSync(FFMPEG, args, { stdio: 'inherit' });
    if (r.status !== 0) process.exit(r.status || 1);
    fs.rmSync(dir, { recursive: true, force: true });
    console.log('wrote', out);
  }
  await browser.close();
})();
