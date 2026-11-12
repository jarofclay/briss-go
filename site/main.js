// briss.app: the scan-to-pages stage, the terminal tabs, and the download hint.
(() => {
  const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;

  /* ---------- stage ---------- */
  const stage = document.querySelector('.stage');
  const caption = document.getElementById('caption');
  const chips = [...document.querySelectorAll('.steps [data-go]')];
  const play = document.querySelector('.play');
  const CAPTIONS = [
    'A book laid open on the scanner: two pages per sheet, a dark border, a shadow in the gutter, and everything leaning a degree and a half.',
    'Briss looks at the sheet itself and finds the printed area of each page. Border, shadow, page edges and dust are left outside the boxes.',
    'The sheet is cut at the gutter. One scan, two pages, in reading order.',
    'Each page is turned level by its own angle: 1.6° for the left one, 1.5° for the right.',
    'The margins go. Nothing is re-encoded: the scan keeps its quality, only the visible page box changes.',
    'On the same screen the text is now about three times larger. That is the whole point.',
  ];
  const HOLD = [2800, 3000, 2200, 2400, 2800, 4800];
  let step = 0, timer = 0, playing = !reduce, visible = false;

  function show(n) {
    step = (n + CAPTIONS.length) % CAPTIONS.length;
    stage.dataset.step = step;
    caption.textContent = CAPTIONS[step];
    chips.forEach((c, i) => i === step ? c.setAttribute('aria-current', 'step') : c.removeAttribute('aria-current'));
    const li = chips[step].parentElement, ol = li.parentElement;
    ol.scrollTo({ left: li.offsetLeft - ol.offsetLeft - (ol.clientWidth - li.offsetWidth) / 2, behavior: reduce ? 'auto' : 'smooth' });
  }
  function schedule() {
    clearTimeout(timer);
    if (playing && visible) timer = setTimeout(() => { show(step + 1); schedule(); }, HOLD[step]);
  }
  function setPlaying(on) {
    playing = on;
    play.setAttribute('aria-pressed', String(!on));
    play.setAttribute('aria-label', on ? 'Pause the animation' : 'Play the animation');
    schedule();
  }
  chips.forEach((c) => c.addEventListener('click', () => { show(+c.dataset.go); setPlaying(false); }));
  play.addEventListener('click', () => { if (!playing) show(step + 1); setPlaying(!playing); });
  new IntersectionObserver(([e]) => { visible = e.isIntersecting; schedule(); }, { threshold: 0.35 }).observe(stage);
  document.addEventListener('visibilitychange', () => { visible = !document.hidden && visible; schedule(); });
  if (reduce) { setPlaying(false); show(5); }

  /* ---------- terminal ---------- */
  const term = document.querySelector('[data-term]');
  const tabs = [...term.querySelectorAll('[role=tab]')];
  const typed = new WeakSet();

  function type(pane) {
    if (reduce || typed.has(pane)) return;
    typed.add(pane);
    const cmd = pane.querySelector('.cmd'), out = pane.querySelector('.out');
    if (!cmd || !out || pane.id === 'pane-more') return;
    const text = cmd.textContent;
    out.classList.add('wait');
    cmd.textContent = '';
    cmd.classList.add('typing');
    let i = 0;
    (function tick() {
      cmd.textContent = text.slice(0, ++i);
      if (i < text.length) setTimeout(tick, 14 + Math.random() * 26);
      else setTimeout(() => { cmd.classList.remove('typing'); out.classList.remove('wait'); }, 350);
    })();
  }
  function select(tab, focus) {
    tabs.forEach((t) => {
      const on = t === tab;
      t.setAttribute('aria-selected', String(on));
      t.tabIndex = on ? 0 : -1;
      document.getElementById(t.getAttribute('aria-controls')).hidden = !on;
    });
    if (focus) tab.focus();
    type(document.getElementById(tab.getAttribute('aria-controls')));
  }
  tabs.forEach((t, i) => {
    t.addEventListener('click', () => select(t));
    t.addEventListener('keydown', (e) => {
      const d = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
      if (d) { e.preventDefault(); select(tabs[(i + d + tabs.length) % tabs.length], true); }
    });
  });
  new IntersectionObserver(([e], io) => {
    if (e.isIntersecting) { type(document.getElementById('pane-crop')); io.disconnect(); }
  }, { threshold: 0.4 }).observe(term);

  /* ---------- download: mark the visitor's system ---------- */
  const p = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || '';
  const os = /mac/i.test(p) ? 'mac' : /win/i.test(p) ? 'win' : /linux|x11/i.test(p) && !/android/i.test(navigator.userAgent) ? 'linux' : '';
  const card = os && document.querySelector(`.oscard[data-os=${os}]`);
  if (card) card.classList.add('you');
})();
