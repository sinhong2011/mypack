// Build time only: the latest release's version for the JSON-LD. Any failure (offline, rate limit) returns undefined.
import { SITE } from "./config";

let cached: Promise<string | undefined> | undefined;
export function latestVersion(): Promise<string | undefined> {
  cached ??= fetch(`${SITE.api}/releases/latest`, { headers: { accept: "application/vnd.github+json" }, signal: AbortSignal.timeout(5000) })
    .then((r) => (r.ok ? r.json() : undefined))
    .then((j) => (j?.tag_name ? String(j.tag_name).replace(/^v/, "") : undefined))
    .catch(() => undefined);
  return cached;
}

// Build time only: the repo's star count, shown next to GitHub links. The page refreshes it in the browser
// (scripts/page.ts); undefined hides the count until then.
let stars: Promise<number | undefined> | undefined;
export function starCount(): Promise<number | undefined> {
  stars ??= fetch(SITE.api, { headers: { accept: "application/vnd.github+json" }, signal: AbortSignal.timeout(5000) })
    .then((r) => (r.ok ? r.json() : undefined))
    .then((j) => (typeof j?.stargazers_count === "number" ? j.stargazers_count : undefined))
    .catch(() => undefined);
  return stars;
}
