# mypack site

The official MyPack website: a static [Astro](https://astro.build) site with Tailwind CSS v4, GSAP, WebGPU
backdrops from the [`shaders`](https://shaders.com) library, and seven languages through
[Paraglide](https://inlang.com/m/gerre34r/library-inlang-paraglideJs). It follows the layout of the Triwarden site.

```sh
pnpm install
pnpm dev        # http://localhost:4321
pnpm build      # static site in dist/
pnpm preview
```

## Where things live

| Path | What |
|---|---|
| `src/components/Home.astro` | The page: sections in order, plus the SoftwareApplication JSON-LD |
| `src/layouts/Base.astro` | `<head>`: title, description, canonical, hreflang, Open Graph, Twitter card |
| `src/config.ts` | Repo links and the install commands (copied from the README; keep them in sync) |
| `src/i18n.ts`, `project.inlang/`, `messages/*.json` | Languages. English is at `/`, the others at `/zh-hant/`, `/zh-hans/`, `/yue/`, `/es/`, `/fr/`, `/ja/` |
| `src/scripts/shader.ts` | The three shader backdrops (hero, gallery, closing) |
| `src/scripts/hero.ts` | The hero demo: an address typed, built and packed into the Dock |
| `src/styles/global.css` | Tokens and all styles |
| `public/assets/` | Screenshots (WebP), icon, favicon, link-preview card |

Copy comes from the repo's READMEs; features must match README.md › Features. The app's own wording (Library,
Discover, New App, Install & Open…) follows `internal/i18n/catalogs/`.

### Languages

Add or change text in `messages/en.json` first, then every other file in `messages/`. Each file must have the
same keys. To add a language, add it to `project.inlang/settings.json`, `LOCALES` in `src/i18n.ts` and `locales`
in `astro.config.mjs`.

### Shaders

`astro.config.mjs` swaps the `shaders` registry for one that lists only the effects in `SHADERS_USED`, so the
other ~200 never ship. The library loads lazily, only on WebGPU browsers; everywhere else, and with reduced
motion, each section's CSS gradient is the finished backdrop.

## Screenshots

The screenshots are real captures of the current app in demo mode (`MYPACK_FAKE=1`, in-memory data, no network),
taken with MyGo's automation plugin so they need no screen-recording permission:

```sh
make app                                              # dist/MyPack.app
go build -tags e2e -o dist/MyPack.app/Contents/MacOS/mypack ./cmd/mypack && codesign -f -s - dist/MyPack.app
go build -o /tmp/mygo-automation github.com/egoist/mygo/plugins/automation/cmd/mygo-automation
open -n --env MYPACK_FAKE=1 --env MYGO_AUTOMATION=/tmp/mypack.sock dist/MyPack.app
/tmp/mygo-automation open --socket /tmp/mypack.sock
/tmp/mygo-automation screenshot captures/dark-library.png   # navigate with `press key=…`, `fill`, `key`
```

Capture each screen in light and dark (Settings › Appearance) into one folder as `light-<name>.png` and
`dark-<name>.png`, where `<name>` is one of `create-empty`, `check`, `building`, `success`, `library`,
`discover`, `builds`, `palette` (see `SHOTS` in `scripts/shots.mjs`). Avoid screens that show your home folder
path, such as an app's details. Then:

```sh
pnpm shots path/to/captures   # resizes to WebP in public/assets/shots and updates src/shots.json
```

The menu bar shot comes from `docs/screenshots/readme-menubar-*.png`.

## Deploy

Cloudflare, configured in `wrangler.toml` the same way as Triwarden's site (`pnpm build`, static assets from
`./dist`). The first time:

```sh
npx wrangler login
SITE_URL=https://mypack.dev npx wrangler deploy
```

Then attach the custom domain in the Cloudflare dashboard (Workers & Pages › mypack › Settings › Domains), once
the domain is on the same Cloudflare account. `SITE_URL` must be the final origin: it sets the canonical URLs,
hreflang links, Open Graph URLs, `robots.txt` and the sitemap. Without it the site still builds, minus the
sitemap and with relative canonical links.
