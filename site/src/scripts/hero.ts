// The hero's one orchestrated moment: an address is typed, the build runs through its real steps
// (Validate, Assemble, Sign, Verify), and the window packs itself into a Dock tile. Then the next site.
// Reduced motion shows the finished state: every tile in the Dock, no loop.
import { gsap, reduce, $, $$ } from "./motion";

type Site = { url: string; name: string; mark: string; tone: string };

const demo = $("[data-demo]");
if (demo) run(demo);

function run(demo: HTMLElement) {
  const sites: Site[] = JSON.parse(demo.dataset.sites!);
  const win = $("[data-win]", demo)!;
  const typed = $("[data-typed]", demo)!;
  const icon = $("[data-icon]", demo)!;
  const name = $("[data-name]", demo)!;
  const bar = $("[data-bar]", demo)!;
  const steps = $$("[data-steps] li", demo);
  const tiles = $$("[data-tile]", demo);

  if (reduce) {
    steps.forEach((s) => s.classList.add("on"));
    gsap.set(bar, { scaleX: 1 });
    return;
  }

  // Until the first frame runs (or if frames never run, as in a background tab) the page shows the
  // window as rendered: GitHub, ready to build. Tweens only take over once they start.
  gsap.set(tiles, { autoAlpha: 0, scale: 0.4 });
  const show = (s: Site) => {
    icon.className = `demo__icon ${s.tone}`;
    icon.textContent = s.mark;
    name.textContent = s.name;
  };

  const tl = gsap.timeline({ repeat: -1, repeatDelay: 0.6, delay: 0.5 });
  sites.forEach((s, i) => {
    const text = { n: 0 };
    tl.call(() => { show(s); typed.textContent = ""; steps.forEach((x) => x.classList.remove("on")); })
      .set(bar, { scaleX: 0 })
      .fromTo(win, { autoAlpha: 0, y: 24, scale: 0.94 }, { autoAlpha: 1, y: 0, scale: 1, duration: 0.6, ease: "expo.out", immediateRender: false })
      .fromTo([icon, name], { autoAlpha: 0, y: 8 }, { autoAlpha: 1, y: 0, duration: 0.4, stagger: 0.06, ease: "power2.out", immediateRender: false }, "<0.1")
      .to(text, { n: s.url.length, duration: s.url.length * 0.055, ease: "none", onUpdate: () => (typed.textContent = s.url.slice(0, Math.round(text.n))) }, "<");
    steps.forEach((step, k) => {
      tl.to(bar, { scaleX: (k + 1) / steps.length, duration: 0.42, ease: "power1.inOut" }).call(() => step.classList.add("on"));
    });
    // Pack: the window shrinks toward its slot in the Dock while the tile pops in.
    // Distances are measured when the tween starts, so they hold at any demo size.
    const delta = () => {
      const a = win.getBoundingClientRect(), b = tiles[i].parentElement!.getBoundingClientRect();
      return { x: b.left + b.width / 2 - (a.left + a.width / 2), y: b.top + b.height / 2 - (a.top + a.height / 2) };
    };
    tl.to(win, { x: () => delta().x, y: () => delta().y, scale: 0.14, autoAlpha: 0, borderRadius: 90, transformOrigin: "50% 50%", duration: 0.55, ease: "power3.in" }, "+=0.35")
      .to(tiles[i], { autoAlpha: 1, scale: 1, duration: 0.6, ease: "back.out(2.2)" }, "-=0.12")
      .set(win, { x: 0, y: 0, borderRadius: 22 });
  });
  // Clear the Dock before the loop starts over.
  tl.to(tiles, { autoAlpha: 0, scale: 0.4, duration: 0.4, stagger: 0.05, ease: "power2.in" }, "+=1.6");

  // Save work while the hero is off-screen.
  new IntersectionObserver(([e]) => (e.isIntersecting ? tl.resume() : tl.pause())).observe(demo);
}
