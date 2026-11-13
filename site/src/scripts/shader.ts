// Live WebGPU backdrops from the open-source `shaders` library (MIT, shaders.com). Each `[data-shader="name"]`
// element gets a canvas; the element's own CSS gradient is the fallback, so a browser without WebGPU, a failed
// device or a slow load still shows a finished page. The library loads only when a backdrop is near the
// viewport and the main thread is idle, telemetry is off, and reduced motion freezes every layer.
// astro.config.mjs keeps the bundle to the effects used here (SHADERS_USED).
import type { ShaderInstance, PresetConfig } from "shaders/js";

const still = matchMedia("(prefers-reduced-motion: reduce)").matches;

// The icon's palette: cobalt field, ultraviolet corner, a cyan glint, midnight depth.
const C = { night: "#06103F", deep: "#0E2A9E", cobalt: "#2360FF", sky: "#5FA0FF", violet: "#7357FF", glint: "#9FE3FF" };
const stops = (cs: string[]) => cs.map((color, i) => ({ color, position: i / (cs.length - 1) }));
const grain = (strength: number, child: object) => ({ type: "FilmGrain", id: "grain", props: { strength, bias: 1 }, children: [child] });

const PRESETS: Record<string, () => PresetConfig> = {
  // Hero: the cobalt field seen through a grid of rounded glass tiles (app icons, many of them), rippling
  // where the cursor passes.
  hero: () => {
    const mesh = {
      type: "MeshGradient", id: "mesh",
      props: { stops: stops([C.night, C.cobalt, C.deep, C.violet, C.sky, C.night]), count: 7, smoothness: 2.4, variation: 0.55, swirl: 0.3, drift: 0.6, speed: still ? 0 : 0.3, seed: 7 },
    };
    const tiles = { type: "GlassTiles", id: "tiles", props: { intensity: 1.4, tileCount: 11, rotation: 0, roundness: 0.55 }, children: [mesh] };
    const top = still ? tiles : { type: "CursorRipples", id: "ripple", props: { intensity: 5, decay: 5, radius: 0.35, chromaticSplit: 0.5 }, children: [tiles] };
    return { components: [grain(0.13, top)] };
  },
  // Gallery stage: a calmer, deeper field for the screenshots to sit on.
  stage: () => ({
    components: [grain(0.12, {
      type: "MeshGradient", id: "mesh",
      props: { stops: stops([C.night, C.deep, C.cobalt, C.night, C.violet]), count: 5, smoothness: 2.6, variation: 0.4, swirl: -0.2, drift: 0.4, speed: still ? 0 : 0.18, seed: 3 },
    })],
  }),
  // Closing: bigger, softer tiles, as if the page settles into a single Dock.
  close: () => ({
    components: [grain(0.12, {
      type: "GlassTiles", id: "tiles", props: { intensity: 1.1, tileCount: 7, rotation: 0, roundness: 0.6 },
      children: [{
        type: "MeshGradient", id: "mesh",
        props: { stops: stops([C.deep, C.cobalt, C.violet, C.sky, C.deep]), count: 6, smoothness: 2.2, variation: 0.5, swirl: 0.35, drift: 0.7, speed: still ? 0 : 0.35, seed: 19 },
      }],
    })],
  }),
};

let lib: Promise<typeof import("shaders/js")> | undefined;
const mounted: ShaderInstance[] = [];

async function build(el: HTMLElement, canvas: HTMLCanvasElement, name: string) {
  if (!("gpu" in navigator)) return; // no WebGPU: keep the CSS backdrop and skip the download
  lib ??= import("shaders/js");
  const { createShader, isWebGPUSupported } = await lib;
  if (!isWebGPUSupported()) return;
  const shader = await createShader(canvas, PRESETS[name](), {
    disableTelemetry: true,
    onReady: () => el.classList.add("shader-ready"),
    onError: () => el.classList.remove("shader-ready"),
  });
  mounted.push(shader);
}

function mount(el: HTMLElement) {
  const name = el.dataset.shader!;
  if (!PRESETS[name]) return;
  const canvas = document.createElement("canvas");
  canvas.className = "shader-canvas";
  // createShader pins an unsized canvas to its first pixel size; percentages keep it following the element.
  canvas.style.width = canvas.style.height = "100%";
  canvas.setAttribute("aria-hidden", "true");
  el.prepend(canvas);
  const io = new IntersectionObserver((entries) => {
    if (!entries.some((e) => e.isIntersecting)) return;
    io.disconnect();
    const go = () => build(el, canvas, name).catch(() => {});
    "requestIdleCallback" in window ? requestIdleCallback(go, { timeout: 1500 }) : setTimeout(go, 300);
  }, { rootMargin: "300px" });
  io.observe(el);
}

document.querySelectorAll<HTMLElement>("[data-shader]").forEach(mount);
