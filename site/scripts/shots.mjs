// Regenerates public/assets/ from real app captures and the app icon: `pnpm shots <capture-dir>`.
//
// <capture-dir> holds light-*.png and dark-*.png window captures of the current app running in demo mode
// (MYPACK_FAKE=1), taken with mygo-automation as site/README.md describes. Each is resized to WebP so pages
// stay light. Without a capture dir, only the icon, favicon and link-preview card are rebuilt.
import sharp from "sharp";
import { copyFileSync, existsSync, mkdirSync, writeFileSync, readFileSync } from "node:fs";
import { join } from "node:path";

const repo = join(import.meta.dirname, "..", "..");
const out = join(import.meta.dirname, "..", "public", "assets");
const manifest = join(import.meta.dirname, "..", "src", "shots.json");
mkdirSync(join(out, "shots"), { recursive: true });

// Site name → capture file name (without the light-/dark- prefix).
const SHOTS = {
  "create-empty": "create-empty",
  "create-check": "check",
  "create-building": "building",
  "create-done": "success",
  library: "library",
  discover: "discover",
  builds: "builds",
  palette: "palette",
};

const sizes = existsSync(manifest) ? JSON.parse(readFileSync(manifest, "utf8")) : {};
const save = async (srcFor, name, width) => {
  for (const t of ["light", "dark"]) {
    const info = await sharp(srcFor(t)).resize({ width, withoutEnlargement: true }).webp({ quality: 84 }).toFile(join(out, "shots", `${t}-${name}.webp`));
    sizes[name] = [info.width, info.height];
  }
};

const caps = process.argv[2];
if (caps) {
  for (const [name, file] of Object.entries(SHOTS)) await save((t) => join(caps, `${t}-${file}.png`), name, 1600);
  // The menu bar flyout lives in the status bar, outside the automation's windows: the README's capture.
  await save((t) => join(repo, "docs", "screenshots", `readme-menubar-${t}.png`), "menubar", 640);
  writeFileSync(manifest, JSON.stringify(sizes, null, 2) + "\n");

  // The hero scene's site icons: the icons MyPack itself fetched, cut from the light Discover capture
  // (152×152 tiles at these origins) and masked to the macOS squircle so they sit on any background.
  const SITE_ICONS = { linear: [814, 308], todoist: [542, 659], trello: [2174, 308], github: [542, 1382] };
  mkdirSync(join(out, "sites"), { recursive: true });
  const mask = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="152" height="152"><rect width="152" height="152" rx="34" fill="#fff"/></svg>`);
  for (const [name, [left, top]] of Object.entries(SITE_ICONS)) {
    await sharp(join(caps, "light-discover.png")).extract({ left, top, width: 152, height: 152 })
      .composite([{ input: mask, blend: "dest-in" }]).png().toFile(join(out, "sites", `${name}.png`));
  }
}

// The official icon, already cut to its squircle with transparent corners (what the app itself shows).
// Only the apple-touch-icon uses the full-bleed art: iOS applies its own mask.
const brand = join(repo, "internal", "desktop", "assets");
copyFileSync(join(brand, "brand-256.png"), join(out, "icon-256.png"));
copyFileSync(join(brand, "brand-64.png"), join(out, "favicon-64.png"));
await sharp(join(repo, "assets", "AppIcon-macOS-1024.png")).resize(180).png().toFile(join(out, "apple-touch-icon.png"));

// Link-preview card, 1200×630: the icon on the hero's cobalt field.
const W = 1200, H = 630;
const bg = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}">
  <defs>
    <radialGradient id="a" cx="18%" cy="20%" r="80%"><stop offset="0" stop-color="#3D7BFF"/><stop offset=".55" stop-color="#1537B8"/><stop offset="1" stop-color="#0A1352"/></radialGradient>
    <radialGradient id="b" cx="92%" cy="96%" r="55%"><stop offset="0" stop-color="#8A6BFF" stop-opacity=".85"/><stop offset="1" stop-color="#8A6BFF" stop-opacity="0"/></radialGradient>
  </defs>
  <rect width="100%" height="100%" fill="url(#a)"/><rect width="100%" height="100%" fill="url(#b)"/>
  <text x="80" y="300" font-family="Helvetica Neue, Arial, sans-serif" font-size="92" font-weight="700" fill="#fff" letter-spacing="-3">Any website.</text>
  <text x="80" y="400" font-family="Helvetica Neue, Arial, sans-serif" font-size="92" font-weight="700" fill="#fff" letter-spacing="-3">Any desktop.</text>
  <text x="84" y="480" font-family="Helvetica Neue, Arial, sans-serif" font-size="32" fill="#DCE6FF">MyPack. Free and open source, for macOS, Windows and Linux.</text>
</svg>`);
const tile = await sharp(join(brand, "brand-256.png")).resize(300).png().toBuffer();
await sharp(bg).composite([{ input: tile, left: 820, top: 70 }]).jpeg({ quality: 86 }).toFile(join(out, "og.jpg"));
console.log("assets written");
