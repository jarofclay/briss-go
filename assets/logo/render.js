// Render the logo SVGs (from svg.py) to the PNG files the builds use, with
// Chromium through Playwright: node render.js (run it from this folder).
//
//   png/briss-<size>.png  for Windows, Linux, the window icon and the README
//   Briss.iconset/        for the macOS app (iconutil turns it into Briss.icns)
//
// 16 to 32 pixels use the simplified drawings.
const { chromium } = require('playwright');
const fs = require('fs');

const jobs = [];
for (const size of [16, 24, 32, 48, 64, 128, 256, 512, 1024]) {
  jobs.push([size <= 32 ? 'briss-small.svg' : 'briss.svg', `png/briss-${size}.png`, size]);
}
for (const [pt, scale] of [[16, 1], [16, 2], [32, 1], [32, 2], [128, 1], [128, 2], [256, 1], [256, 2], [512, 1], [512, 2]]) {
  const px = pt * scale;
  const name = `icon_${pt}x${pt}${scale === 2 ? '@2x' : ''}.png`;
  jobs.push([px <= 32 ? 'briss-macos-small.svg' : 'briss-macos.svg', `Briss.iconset/${name}`, px]);
}

(async () => {
  fs.mkdirSync('png', { recursive: true });
  fs.mkdirSync('Briss.iconset', { recursive: true });
  const browser = await chromium.launch();
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  for (const [svg, out, size] of jobs) {
    const data = fs.readFileSync(svg).toString('base64');
    await page.setViewportSize({ width: size, height: size });
    await page.setContent(`<body style="margin:0;background:transparent"><img id="i" src="data:image/svg+xml;base64,${data}" width="${size}" height="${size}" style="display:block"></body>`);
    await page.waitForFunction(() => document.getElementById('i').complete);
    await page.screenshot({ path: out, omitBackground: true, clip: { x: 0, y: 0, width: size, height: size } });
  }
  await browser.close();
})();
