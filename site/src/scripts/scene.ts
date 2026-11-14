// The hero's build scene: the app's own animation, ported from internal/desktop/assets/scene/scene.html
// (which kit/scene.go also follows frame for frame), with the site's real icon as the app draws it.
// A website window is checked, the app engine downloads into its address bar, the window morphs into the app's icon, a verified badge springs on, one light sweep crosses it,
// and a Dock slides up for the icon to drop into, bounce twice and show its running dot.
// Same springs and constants as the app; colours suit the hero's cobalt glass. Reduced motion shows the finished
// app in the Dock and does not loop. Frames stop while the hero is off-screen or the tab is hidden.

type Site = { color: string; name: string; host: string; icon?: string };
type Stage = "validating" | "downloading" | "assembling" | "signing" | "verifying" | "done";
type Spring = { v: number; x: number; vel: number; k: number; d: number };

const NS = "http://www.w3.org/2000/svg";
const ORDER: Stage[] = ["validating", "downloading", "assembling", "signing", "verifying", "done"];
// The app's demo timing (ms per stage).
const BEATS: [Stage, number][] = [["validating", 1700], ["downloading", 2000], ["assembling", 1400], ["signing", 900], ["verifying", 1000], ["done", 3200]];

const root = document.querySelector<HTMLElement>("[data-scene]");
if (root) run(root);

function run(root: HTMLElement) {
  const svg = root.querySelector<SVGSVGElement>("svg")!;
  const all = svg.querySelector<SVGGElement>("[data-all]")!;
  const clipRect = svg.querySelector<SVGRectElement>("[data-clip]")!;
  const caption = root.querySelector<HTMLElement>("[data-caption]")!;
  const titles = JSON.parse(root.dataset.titles!) as Record<Stage, string>;
  // Sites from Discover, with the icons MyPack fetched for them (public/assets/sites/).
  const SITES = JSON.parse(root.dataset.sites!) as Site[];
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // ---- springs (the app's constants) ----
  const REST = { morph: 0, load: 0, chrome: 1, badge: 0, dock: 0, drop: 0, run: 0, shine: 0, shake: 0, fade: 1 };
  type Name = keyof typeof REST;
  const sp = Object.fromEntries(Object.entries(REST).map(([k, v]) => [k, { v, x: v, vel: 0, k: 170, d: 16 }])) as Record<Name, Spring>;
  const to = (n: Name, v: number, k = 170, d = 16) => { const s = sp[n]; s.v = v; s.k = k; s.d = d; kick(); };
  function step(dt: number) {
    let moving = false;
    for (const s of Object.values(sp)) {
      if (reduce) { s.x = s.v; s.vel = 0; continue; }
      s.vel += (-s.k * (s.x - s.v) - s.d * s.vel) * dt; s.x += s.vel * dt;
      if (Math.abs(s.vel) > 0.002 || Math.abs(s.x - s.v) > 0.002) moving = true; else { s.x = s.v; s.vel = 0; }
    }
    return moving;
  }

  // ---- state ----
  const st = { stage: "validating" as Stage, progress: 0, site: SITES[0] };
  // Fetch the icons up front so none pops in mid-morph.
  SITES.forEach((x) => x.icon && (new Image().src = x.icon));
  const t0 = performance.now();
  let ring = 0;
  const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
  const clamp = (x: number) => Math.max(0, Math.min(1, x));
  const el = (tag: string, attrs: Record<string, string | number>, parent: Element = all) => {
    const e = document.createElementNS(NS, tag);
    for (const k in attrs) e.setAttribute(k, String(attrs[k]));
    parent.appendChild(e);
    return e;
  };
  const mix = (a: string, b: string, t: number) => {
    const p = (h: string) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
    const A = p(a), B = p(b);
    return "#" + A.map((v, i) => Math.round(lerp(v, B[i], t)).toString(16).padStart(2, "0")).join("");
  };

  // ---- draw (the reference's geometry; light window and glass Dock on the cobalt hero) ----
  function draw(now: number) {
    all.replaceChildren();
    const t = (now - t0) / 1000;
    const color = st.site.color;
    const m = clamp(sp.morph.x);
    const chrome = clamp(sp.chrome.x);
    const dockY = 236, slotY = dockY - 4;

    const d1 = clamp(sp.dock.x);
    if (d1 > 0.01) {
      const dg = el("g", { transform: `translate(0 ${lerp(70, 0, d1).toFixed(2)})`, opacity: d1.toFixed(3) });
      el("rect", { x: 92, y: dockY - 34, width: 216, height: 60, rx: 22, fill: "rgba(255,255,255,.16)", stroke: "rgba(255,255,255,.34)", filter: "url(#mp-dock-shadow)" }, dg);
      [118, 162, 282].forEach((x) => el("rect", { x: x - 18, y: dockY - 22, width: 36, height: 36, rx: 10, fill: "rgba(255,255,255,.24)" }, dg));
      const r = clamp(sp.run.x);
      el("circle", { cx: 222, cy: dockY + 20, r: 2.4 * r, fill: "#ffffff", opacity: (0.85 * r).toFixed(2) }, dg);
    }

    // window 224×150 → icon 92×92 → Dock slot 36×36
    const bob = reduce ? 0 : Math.sin(t * 2) * 2.5 * (1 - m);
    let w = lerp(224, 92, m), h = lerp(150, 92, m), r = lerp(16, 22, m), cx = 200, cy = lerp(132, 128, m) + bob;
    const drop = clamp(sp.drop.x);
    if (drop > 0) { w = lerp(92, 36, drop); h = w; r = lerp(22, 10, drop); cx = lerp(200, 222, drop); cy = lerp(128, slotY - 4, drop); }
    cy += sp.shake.x;
    const x = cx - w / 2, y = cy - h / 2;

    const g = el("g", { opacity: clamp(sp.fade.x).toFixed(3), filter: "url(#mp-lift)" });
    const paper = "#ffffff";
    el("rect", { x, y, width: w, height: h, rx: r, fill: m > 0 ? mix(paper, color, Math.pow(m, 0.8)) : paper }, g);

    clipRect.setAttribute("x", String(x)); clipRect.setAttribute("y", String(y));
    clipRect.setAttribute("width", String(w)); clipRect.setAttribute("height", String(h)); clipRect.setAttribute("rx", String(r));
    const inner = el("g", { "clip-path": "url(#mp-win-clip)" }, g);

    if (chrome > 0.01) {
      const cg = el("g", { opacity: chrome.toFixed(3) }, inner);
      ["#ff5f57", "#febc2e", "#28c840"].forEach((c, i) => el("circle", { cx: x + 15 + i * 11, cy: y + 14, r: 3.6, fill: c }, cg));
      const bx = x + 52, bw = w - 64, by = y + 6, bh = 16;
      el("rect", { x: bx, y: by, width: bw, height: bh, rx: 8, fill: "#eef2f8" }, cg);
      const load = clamp(sp.load.x);
      if (load > 0.005) {
        el("rect", { x: bx, y: by, width: bw * load, height: bh, rx: 8, fill: color, opacity: 0.2 }, cg);
        el("rect", { x: bx + 6, y: by + bh - 2.5, width: Math.max(0, (bw - 12) * load), height: 2, rx: 1, fill: color }, cg);
      }
      const tx = el("text", { x: bx + 24, y: by + 11.5, fill: "rgba(14,23,38,.55)", "font-size": 9, "font-family": "Onest, -apple-system, system-ui, sans-serif" }, cg);
      tx.textContent = st.site.host;
      el("line", { x1: x, y1: y + 28, x2: x + w, y2: y + 28, stroke: "rgba(14,40,90,.06)" }, cg);
      el("rect", { x: x + 14, y: y + 40, width: w - 28, height: 34, rx: 10, fill: color, opacity: 0.9 }, cg);
      ([[0.82, 86], [0.6, 99], [0.7, 112]] as const).forEach(([f, yy]) => el("rect", { x: x + 14, y: y + yy, width: (w - 28) * f, height: 7, rx: 3.5, fill: "rgba(14,40,90,.07)" }, cg));
      if (st.stage === "validating" && !reduce) {
        const p = (t * 0.75) % 1, sy = y + 30 + p * (h - 36);
        el("rect", { x, y: sy - 18, width: w, height: 18, fill: color, opacity: 0.08 }, cg);
        el("rect", { x: x + 8, y: sy, width: w - 16, height: 2, rx: 1, fill: color }, cg);
      }
    }

    // favicon: small in the address bar → large in the icon's centre (a monogram, as the app draws without an icon)
    const fs = lerp(12, 54, m) * (drop > 0 ? lerp(1, 36 / 92, drop) : 1);
    const fx = lerp(x + 58, cx - fs / 2, m), fy = lerp(y + 8, cy - fs / 2, m);
    if (st.site.icon) {
      el("image", { href: st.site.icon, x: fx, y: fy, width: fs, height: fs, preserveAspectRatio: "xMidYMid meet" }, inner);
    } else {
      el("rect", { x: fx, y: fy, width: fs, height: fs, rx: fs * 0.26, fill: m > 0.5 ? "#ffffff" : color, opacity: m > 0.5 ? 0.22 : 1 }, inner);
      const tt = el("text", { x: fx + fs / 2, y: fy + fs * 0.72, "text-anchor": "middle", fill: "#fff", "font-size": (fs * 0.66).toFixed(1), "font-weight": 700, "font-family": "Onest, -apple-system, system-ui, sans-serif" }, inner);
      tt.textContent = st.site.name.charAt(0).toUpperCase();
    }

    const sh = sp.shine.x;
    if (sh > 0 && sh < 1) {
      const sx0 = lerp(x - 40, x + w + 40, sh);
      el("polygon", { points: `${sx0},${y} ${sx0 + 22},${y} ${sx0 - 8},${y + h} ${sx0 - 30},${y + h}`, fill: "#fff", opacity: 0.35 }, inner);
    }

    const b = sp.badge.x;
    if (b > 0.01 && drop < 0.5) {
      const bg = el("g", { transform: `translate(${(x + w - 6).toFixed(2)} ${(y + 6).toFixed(2)}) scale(${b.toFixed(3)})` });
      el("circle", { r: 15, fill: "#1578FA", stroke: "#ffffff", "stroke-width": 3 }, bg);
      el("path", { d: "M-6 .5 L-1.8 4.6 L6.5 -4", fill: "none", stroke: "#fff", "stroke-width": 3, "stroke-linecap": "round", "stroke-linejoin": "round" }, bg);
      if (ring > 0) el("circle", { r: 15 + ring * 22, fill: "none", stroke: "#ffffff", "stroke-width": 2, opacity: ((1 - ring) * 0.6).toFixed(2) }, bg);
    }
  }

  // ---- frame loop: idle when nothing moves, stopped when hidden or off-screen ----
  let raf = 0, last = 0, onScreen = true;
  function frame(now: number) {
    const dt = Math.min(0.05, (now - (last || now)) / 1000); last = now;
    const moving = step(dt);
    if (ring > 0) { ring += dt * 1.6; if (ring >= 1) ring = 0; }
    draw(now);
    const idle = !reduce && (st.stage === "validating" || sp.morph.x < 0.5);
    raf = (moving || ring > 0 || idle) && onScreen ? requestAnimationFrame(frame) : 0;
    if (!raf) last = 0;
  }
  function kick() { if (!raf && !document.hidden && onScreen) raf = requestAnimationFrame(frame); }
  document.addEventListener("visibilitychange", () => { if (document.hidden) { cancelAnimationFrame(raf); raf = 0; last = 0; } else kick(); });

  // ---- stages → motion (the app's choreography) ----
  let reached = -1, timers: number[] = [];
  const later = (ms: number, fn: () => void) => timers.push(window.setTimeout(fn, reduce ? 0 : ms));
  const bounce = () => {
    if (reduce) return;
    let n = 0;
    const hop = () => { if (n++ >= 2) return; to("shake", 0, 140, 9); sp.shake.vel = -170; later(520, hop); };
    hop();
  };

  function set(stage: Stage, restart = false) {
    if (restart) {
      timers.forEach(clearTimeout); timers = []; reached = -1; ring = 0;
      for (const [k, s] of Object.entries(sp)) { s.x = s.v = REST[k as Name]; s.vel = 0; }
    }
    st.stage = stage;
    const i = ORDER.indexOf(stage);
    if (i === 1) to("load", st.progress, 220, 24);
    if (i >= 2 && reached < 2) { to("load", 1, 220, 24); to("chrome", 0, 200, 22); later(120, () => to("morph", 1, 120, 13)); }
    if (i >= 3 && reached < 3) later(200, () => { to("badge", 1, 320, 13); ring = 0.001; kick(); });
    if (i >= 4 && reached < 4) later(150, () => { sp.shine.x = 0; sp.shine.v = 0; to("shine", 1, 30, 9); });
    if (i === 5 && reached < 5) {
      to("dock", 1, 160, 18);
      later(520, () => { to("badge", 0, 240, 20); to("drop", 1, 110, 12); });
      later(1050, () => { bounce(); to("run", 1, 160, 14); });
    }
    reached = Math.max(reached, i);
    caption.textContent = titles[stage] + (stage === "downloading" ? ` ${Math.round(st.progress * 100)}%` : "");
    kick();
  }

  draw(performance.now()); // paint the window now, before any frame runs
  if (reduce) { set("done", true); return; }

  // Demo loop, as the app's reference page runs it. Paused while the hero is off-screen.
  let n = 0, paused: (() => void) | null = null;
  const wait = (ms: number) => new Promise<void>((res) => {
    const end = () => (onScreen ? res() : (paused = res));
    window.setTimeout(end, ms);
  });
  new IntersectionObserver(([e]) => {
    onScreen = e.isIntersecting;
    if (onScreen) { kick(); const p = paused; paused = null; p?.(); }
  }).observe(root);

  (async function loop() {
    for (;;) {
      st.site = SITES[n++ % SITES.length];
      st.progress = 0;
      set("validating", true);
      for (const [stage, ms] of BEATS) {
        set(stage);
        if (stage === "downloading") {
          const s = performance.now();
          while (performance.now() - s < ms) { st.progress = (performance.now() - s) / ms; set("downloading"); await wait(80); }
        } else await wait(ms);
      }
    }
  })();
}
