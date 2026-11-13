/* Page behaviour: sticky nav, platform-aware download labels, the step frames, tabs and copy buttons. */
import { ScrollTrigger, $, $$ } from "./motion";
import "./shader";
import "./scene";

// Nav turns solid once the page leaves the hero.
const nav = $("[data-nav]")!;
const onScroll = () => nav.classList.toggle("is-stuck", scrollY > innerHeight * 0.6);
addEventListener("scroll", onScroll, { passive: true });
onScroll();

// Close the language menu on outside click or Escape.
const lang = $<HTMLDetailsElement>("[data-lang]");
document.addEventListener("click", (e) => { if (lang?.open && !lang.contains(e.target as Node)) lang.open = false; });
document.addEventListener("keydown", (e) => { if (e.key === "Escape" && lang?.open) { lang.open = false; $("summary", lang)?.focus(); } });

// Which platform is this? Labels say "Download for macOS" and the install tabs open on it.
const ua = navigator.userAgent;
const os = /Windows/i.test(ua) ? "win" : /Mac/i.test(ua) && !/iPhone|iPad/i.test(ua) ? "mac" : /Linux|X11/i.test(ua) && !/Android/i.test(ua) ? "linux" : null;
const OS_NAME = { mac: "macOS", win: "Windows", linux: "Linux" } as const;
if (os) $$("[data-dl-label]").forEach((el) => (el.textContent = el.dataset.template!.replace("{os}", OS_NAME[os])));

// Tabs (gallery and install): click or arrow keys move the selection.
function tabs(list: HTMLElement, onSelect?: (i: number) => void) {
  const btns = $$<HTMLButtonElement>("[role=tab]", list);
  const select = (i: number, focus = false) => {
    btns.forEach((b, k) => {
      const on = k === i;
      b.setAttribute("aria-selected", String(on));
      b.tabIndex = on ? 0 : -1;
      document.getElementById(b.getAttribute("aria-controls")!)!.hidden = !on;
    });
    if (focus) btns[i].focus();
    onSelect?.(i);
  };
  btns.forEach((b, i) => {
    b.addEventListener("click", () => select(i));
    b.addEventListener("keydown", (e) => {
      const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
      if (d) { e.preventDefault(); select((i + d + btns.length) % btns.length, true); }
    });
  });
  return select;
}
const gallery = $("[data-tabs]");
if (gallery) tabs(gallery);
const osList = $("[data-os-root] [role=tablist]");
if (osList) {
  const select = tabs(osList);
  const i = os ? ["mac", "win", "linux"].indexOf(os) : 0;
  if (i > 0) select(i);
}

// Steps: on wide screens the frame on the right follows the step being read.
const root = $("[data-steps-root]");
if (root) {
  const steps = $$("[data-step]", root);
  const frames = $$(".steps__frame .frame", root);
  const activate = (i: number) => {
    steps.forEach((s, k) => s.classList.toggle("is-active", k === i));
    frames.forEach((f, k) => f.classList.toggle("is-active", k === i));
  };
  ScrollTrigger.matchMedia({
    "(min-width: 960px)": () => {
      steps.forEach((s, i) => ScrollTrigger.create({ trigger: s, start: "top 55%", end: "bottom 55%", onToggle: (st) => st.isActive && activate(i) }));
    },
  });
}

// GitHub stars: refresh the build-time count, once a session per visitor (the API allows 60 calls an hour).
(async () => {
  const chips = $$("[data-stars]");
  if (!chips.length) return;
  let n: number | undefined;
  try { const c = sessionStorage.getItem("mp-stars"); if (c) n = Number(c); } catch {}
  if (n === undefined || Number.isNaN(n)) {
    n = undefined;
    try {
      const r = await fetch("https://api.github.com/repos/sinhong2011/mypack", { headers: { accept: "application/vnd.github+json" } });
      if (r.ok) n = (await r.json()).stargazers_count;
      try { if (typeof n === "number") sessionStorage.setItem("mp-stars", String(n)); } catch {}
    } catch {}
  }
  if (typeof n !== "number") return;
  for (const chip of chips) {
    const v = new Intl.NumberFormat(chip.dataset.lang, { notation: "compact", maximumFractionDigits: 1 }).format(n);
    $("[data-stars-n]", chip)!.textContent = v;
    chip.setAttribute("aria-label", chip.dataset.label!.replace("{count}", v));
    chip.hidden = false;
  }
})();

// Copy buttons: [data-copy-text] holds the text; [data-copy-label] shows the result for a moment.
$$("[data-copy-text]").forEach((btn) => btn.addEventListener("click", async () => {
  const label = $("[data-copy-label]", btn) ?? btn;
  const idle = label.textContent;
  try { await navigator.clipboard.writeText(btn.dataset.copyText!); label.textContent = btn.dataset.copyDone ?? ""; }
  catch { label.textContent = btn.dataset.copyFail ?? ""; }
  setTimeout(() => (label.textContent = idle), 1400);
}));

