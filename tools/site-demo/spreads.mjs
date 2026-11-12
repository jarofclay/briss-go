// Renders the synthetic "scanned book" used by the demo on briss.app.
// Usage: node spreads.mjs OUTDIR   (needs playwright and the EB Garamond font files)
// The text is original, written for this demo, so it can be shipped freely.
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';

const out = process.argv[2] || 'out';
const require = createRequire(path.resolve(process.cwd(), 'x.js'));
const { chromium } = require('playwright'); // from the work directory or NODE_PATH
const font = (f) => readFileSync(require.resolve(`@fontsource/eb-garamond/files/${f}`)).toString('base64');

const P = [
  `It is a curious fact, and one seldom remarked upon by those who make books, that the greater part of every page is given over to nothing at all. The printer sets his lines in a narrow field and surrounds them with a broad white border, as a farmer leaves a headland round his wheat; and the reader, who has paid for the whole sheet, receives perhaps one half of it in words.`,
  `Nor is this waste, as the hasty suppose. The margin is where the thumb rests and where the eye recovers; it keeps the text from the binder's knife and from the damp that creeps inward from the edges of old volumes. A page without margins is a room without a door: one may admire the furniture, but one cannot comfortably go in.`,
  `Yet what serves the hand that holds a folio serves very ill the hand that holds a slate. When a book has been laid open upon the glass and copied, two pages to the sheet, the copy carries with it everything the glass could see: the dark of the lid, the shadow in the gutter where the leaves dive toward the spine, the grey fore-edge of all the pages lying beneath, and now and then a thumb.`,
  `The reader of such a copy is in the position of a man who looks at a miniature through the wrong end of a telescope. The words are all there, and all correct, but they have withdrawn to the middle of the field, and no squinting will bring them nearer. He turns the device sideways; he pinches and drags; he reads a line, loses his place, and begins the paragraph again.`,
  `The remedy is as old as the paper-knife. One has only to find where the print leaves off and the emptiness begins, and to cut there, a little outside the last letter, on all four sides. Do this for the left page and again for the right, set the two in their proper order, and the book that was a photograph of a book becomes once more a thing that can be read.`,
  `It will be objected that no two sheets lie alike upon the glass. That is true, and it is the whole difficulty. One leans a degree to the left, the next a degree to the right; a third was laid down sideways by a tired assistant at the end of the day. The careful workman therefore measures every sheet for itself, and trusts no measurement taken from its neighbour.`,
  `There remain the small furnitures of the page: the running title, the folio in its corner, the signature mark at the foot. Some readers hold these to be part of the text and will not part with them; others count them among the margins and are glad to see them go. On this question I offer no opinion, except that the choice ought to be the reader's, and ought to be made once for the whole book.`,
  `Of straightening I shall say only that a leaning line tires the eye more than a small one. A page turned level by a single degree is read with noticeably less effort, though few readers could tell you afterwards what had been done to it. The best of these operations, like the best of the binder's, are those that nobody notices.`,
  `When all is finished the pages should be of one size, so that the text neither swells nor shrinks as the leaves are turned. This is a small courtesy, but small courtesies are the greater part of typography, and the reader who has been spared a hundred little adjustments will finish the chapter without knowing why it seemed so short.`,
];
const TITLE = 'A Reader’s Companion';
const CHAPS = ['Of Margins', 'Of Margins', 'Of the Gutter', 'Of the Gutter', 'Of Leaning Pages', 'Of Leaning Pages'];

function page(n, folio, side) {
  const start = (n * 3) % P.length;
  const paras = (n === 0 ? [0, 1, 2] : [0, 1, 2, 3]).slice(0, 3).map((i) => P[(start + i) % P.length]);
  const head = side === 'l'
    ? `<span>${folio}</span><span class="rt">${TITLE}</span><span></span>`
    : `<span></span><span class="rt">${CHAPS[n]}</span><span>${folio}</span>`;
  const chapter = n === 0 ? `<h2>CHAPTER III</h2><h3>Of Margins, and What Becomes of Them</h3>` : '';
  return `<div class="page ${side}"><div class="head">${head}</div>${chapter}
    ${paras.map((p, i) => `<p class="${n === 0 && i === 0 ? 'first' : ''}">${p}</p>`).join('')}
    <div class="sig">${side === 'r' ? 'C' + (n + 1) : ''}</div></div>`;
}

function html(s) {
  const { tilt, dx, dy, n } = s;
  return `<!doctype html><meta charset="utf-8"><style>
@font-face{font-family:G;src:url(data:font/woff2;base64,${font('eb-garamond-latin-400-normal.woff2')});font-weight:400}
@font-face{font-family:G;src:url(data:font/woff2;base64,${font('eb-garamond-latin-400-italic.woff2')});font-weight:400;font-style:italic}
@font-face{font-family:G;src:url(data:font/woff2;base64,${font('eb-garamond-latin-600-normal.woff2')});font-weight:600}
*{box-sizing:border-box;margin:0}
html,body{width:2200px;height:1560px;overflow:hidden}
body{background:#17161a;position:relative;font-family:G,serif}
.bed{position:absolute;inset:0;background:radial-gradient(120% 90% at 50% 40%,#2a282d 0,#151417 70%,#0c0c0e 100%)}
.book{position:absolute;left:${150 + dx}px;top:${95 + dy}px;width:1900px;height:1370px;transform:rotate(${tilt}deg);
  filter:drop-shadow(0 6px 18px rgba(0,0,0,.7))}
/* the block of pages lying beneath: grey fore-edges */
.block{position:absolute;inset:-9px -16px -13px -16px;border-radius:3px;
  background:repeating-linear-gradient(90deg,#8d8779 0 1px,#b9b2a2 1px 3px)}
.sheet{position:absolute;inset:0;display:flex;background:#e9e2d0}
.page{flex:1;position:relative;padding:118px 132px 110px 150px;color:#1d1a16;
  background:linear-gradient(90deg,#e4dcc8,#efe8d7 30%,#ede6d4)}
.page.r{padding:118px 150px 110px 132px;background:linear-gradient(270deg,#e2dac6,#eee7d6 30%,#ece5d3)}
/* gutter shadow: the leaves dive toward the spine */
.page.l::after,.page.r::after{content:"";position:absolute;top:0;bottom:0;width:150px;pointer-events:none}
.page.l::after{right:0;background:linear-gradient(90deg,transparent,rgba(40,30,20,.16) 55%,rgba(20,14,8,.62))}
.page.r::after{left:0;background:linear-gradient(270deg,transparent,rgba(40,30,20,.14) 55%,rgba(20,14,8,.58))}
.head{display:grid;grid-template-columns:80px 1fr 80px;font-size:23px;letter-spacing:.04em;margin-bottom:46px;color:#2a2620}
.head .rt{text-align:center;font-style:italic}
.head span:last-child{text-align:right}
h2{text-align:center;font-weight:400;font-size:27px;letter-spacing:.22em;margin:34px 0 14px}
h3{text-align:center;font-weight:400;font-style:italic;font-size:34px;margin-bottom:44px}
p{font-size:30px;line-height:1.5;text-align:justify;hyphens:auto;text-indent:1.4em}
p.first{text-indent:0}
p.first::first-letter{font-size:92px;float:left;line-height:.82;padding:8px 8px 0 0}
.sig{position:absolute;bottom:62px;left:0;right:0;text-align:center;font-size:20px;color:#3a352c}
.noise{position:absolute;inset:0;mix-blend-mode:multiply;opacity:.5}
.dust{position:absolute;border-radius:50%;background:#2b2722}
.vig{position:absolute;inset:0;background:linear-gradient(180deg,rgba(0,0,0,.16),transparent 12%,transparent 90%,rgba(0,0,0,.2))}
</style><body lang="en"><div class="bed"></div>
<div class="book"><div class="block"></div><div class="sheet">${page(n, 34 + n, 'l')}${page(n + 1, 35 + n, 'r')}</div></div>
<svg class="noise" width="2200" height="1560"><filter id="n"><feTurbulence type="fractalNoise" baseFrequency=".8" numOctaves="2" seed="${7 + n}"/>
<feColorMatrix values="0 0 0 0 .5  0 0 0 0 .5  0 0 0 0 .5  0 0 0 -1.1 1.05"/></filter><rect width="100%" height="100%" filter="url(#n)"/></svg>
${s.dust.map(([x, y, r]) => `<i class="dust" style="left:${x}px;top:${y}px;width:${r}px;height:${r}px;opacity:.55"></i>`).join('')}
<div class="vig"></div></body>`;
}

const spreads = [
  { n: 0, tilt: -1.6, dx: 10, dy: 0, dust: [[90, 300, 5], [2120, 1210, 4], [1300, 40, 3]] },
  { n: 2, tilt: 1.1, dx: -20, dy: 12, dust: [[60, 900, 4], [2140, 500, 6]] },
  { n: 4, tilt: -0.7, dx: 16, dy: -8, dust: [[110, 1400, 5], [1900, 30, 3]] },
];
const b = await chromium.launch();
const pg = await b.newPage({ viewport: { width: 2200, height: 1560 }, deviceScaleFactor: 1 });
for (const [i, s] of spreads.entries()) {
  await pg.setContent(html(s));
  await pg.evaluate(() => document.fonts.ready);
  await pg.screenshot({ path: path.join(out, `sheet-${i + 1}.png`) });
}
await b.close();
