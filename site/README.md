# briss.app

The public site for Briss: one static page, no build step and no external requests.

- `index.html`, `styles.css`, `main.js`: the page.
- `assets/demo/`: the scan, the crop boxes and the output pages shown in the demo. They are
  real Briss output from a synthetic scan; rebuild them with `tools/site-demo/build.sh WORKDIR`
  (see the comments at the top of that script) after the detector changes, and copy the new
  rectangles from `demo.crops.json` into the `--x1 ... --a2` variables on `.stage` in `styles.css`.
- `assets/img/`: the app screenshots from `docs/img/`, as WebP.
- `assets/fonts/`: Fraunces and JetBrains Mono (SIL Open Font License 1.1).
- `favicon.svg`, `favicon-32.png`, `apple-touch-icon.png`: the Briss logo, from `assets/logo/`
  (`briss-small.svg`, `png/briss-32.png`, and `png/briss-256.png` scaled to 180 on the app's
  background colour).

Look at it locally with `python3 -m http.server -d site 4173`. It is published to GitHub Pages
by `.github/workflows/site.yml` on every change to `master`; `CNAME` holds the domain.
