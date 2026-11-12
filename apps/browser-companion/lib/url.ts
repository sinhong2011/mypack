// Which tab addresses may become an app. The native host validates again; this only decides what the popup offers.

const STORE_HOSTS = new Set(['chromewebstore.google.com', 'microsoftedge.microsoft.com']);

export type TabCheck = { ok: true; host: string; url: string } | { ok: false };

/** An http(s) address without credentials, not a browser store page, with the fragment removed. */
export function checkTabUrl(raw: string | undefined | null): TabCheck {
  if (!raw || raw.length > 2048) return { ok: false };
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return { ok: false };
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return { ok: false };
  if (u.username || u.password) return { ok: false };
  const host = u.hostname.toLowerCase();
  if (!host) return { ok: false };
  if (STORE_HOSTS.has(host)) return { ok: false };
  if (host === 'chrome.google.com' && u.pathname.startsWith('/webstore')) return { ok: false };
  u.hash = '';
  return { ok: true, host, url: u.href };
}

/** A favicon address safe to show in the popup (http(s) or an inline image), else ''. */
export function faviconSrc(u: unknown): string {
  return typeof u === 'string' && /^(https?:|data:image\/)/i.test(u) ? u : '';
}
