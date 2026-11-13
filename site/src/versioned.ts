// Build-time only (uses node:fs): a public file's URL with a short hash of its contents, so a re-captured
// screenshot gets a new URL and browsers don't keep showing the cached old one.
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { asset } from "./config";

const cache = new Map<string, string>();
export function versioned(path: string): string {
  let url = cache.get(path);
  if (!url) {
    let v = "";
    try { v = createHash("sha1").update(readFileSync(join(process.cwd(), "public", path))).digest("hex").slice(0, 8); } catch {}
    url = v ? `${asset(path)}?v=${v}` : asset(path);
    if (!import.meta.env.DEV) cache.set(path, url);
  }
  return url;
}
