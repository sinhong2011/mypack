// @ts-check
import { defineConfig } from "astro/config";
import tailwindcss from "@tailwindcss/vite";
import sitemap from "@astrojs/sitemap";
import { paraglideVitePlugin } from "@inlang/paraglide-js";

// Keep in sync with project.inlang/settings.json and src/i18n.ts. These are the app's and the READMEs' languages.
const locales = ["en", "zh-hant", "zh-hans", "yue", "es", "fr", "ja"];
// BCP 47 tags for hreflang, as `lang` in src/i18n.ts.
const LANGS = { en: "en", "zh-hant": "zh-Hant-TW", "zh-hans": "zh-Hans", yue: "yue-Hant-HK", es: "es", fr: "fr", ja: "ja" };

// The production origin. Canonical URLs, Open Graph and the sitemap need an absolute origin, so set SITE_URL
// on the Cloudflare Pages project (or locally) once the domain is live; without it the build still works.
const siteUrl = process.env.SITE_URL;
const integrations = [];
if (siteUrl) {
  integrations.push(sitemap({ i18n: { defaultLocale: "en", locales: LANGS } }));
}

// The `shaders` library registers all ~200 of its effects up front (2.5 MB). The site uses a handful, so its
// registry module is swapped for one that lists only these; the rest are never bundled. Add a name here when
// a preset in src/scripts/shader.ts starts using a new component.
const SHADERS_USED = ["MeshGradient", "GlassTiles", "CursorRipples", "FilmGrain"];

/** @returns {import("vite").Plugin} */
function slimShaderRegistry() {
  const id = "\0mp-shader-registry";
  return {
    name: "mp-slim-shader-registry",
    enforce: "pre",
    resolveId(source, importer) {
      if (importer?.includes("/shaders/dist/") && /(^|\/)shaderRegistry-[\w-]+\.js$/.test(source)) return id;
    },
    load(loaded) {
      if (loaded !== id) return;
      const imports = SHADERS_USED.map((n, i) => `import { componentDefinition as d${i} } from "shaders/core/${n}";`).join("\n");
      return `${imports}
const shaderRegistry = {};
for (const def of [${SHADERS_USED.map((_, i) => `d${i}`).join(", ")}]) shaderRegistry[def.name] = { name: def.name, fileName: def.name, category: def.category || "Uncategorized", definition: def, propsMetadata: {} };
const getAllShaders = () => Object.values(shaderRegistry);
const getShaderByName = (name) => shaderRegistry[name];
const getShadersByCategory = (c) => getAllShaders().filter((s) => s.category === c);
const getShaderCategories = () => [...new Set(getAllShaders().map((s) => s.category))].sort();
export { shaderRegistry as a, getShadersByCategory as i, getShaderByName as n, getShaderCategories as r, getAllShaders as t };`;
    },
  };
}

export default defineConfig({
  ...(siteUrl ? { site: siteUrl } : {}),
  output: "static",
  trailingSlash: "ignore",
  devToolbar: { enabled: false },
  i18n: { defaultLocale: "en", locales, routing: { prefixDefaultLocale: false } },
  integrations,
  vite: {
    // The one large chunk is the shaders runtime (~300 kB gzipped). It loads lazily, only where WebGPU exists.
    build: { chunkSizeWarningLimit: 1500 },
    optimizeDeps: { include: ["gsap", "gsap/ScrollTrigger", "shaders/js"] },
    plugins: [
      tailwindcss(),
      slimShaderRegistry(),
      // Static pages: the middleware sets the locale per page, so no URL or cookie strategy is needed at runtime.
      paraglideVitePlugin({ project: "./project.inlang", outdir: "./src/paraglide", strategy: ["globalVariable", "baseLocale"] }),
    ],
  },
});
