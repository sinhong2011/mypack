// Shared helpers: native-messaging client, URL eligibility, locale resolution.
// Plain ES modules; no build step. Protocol keys and error codes are never
// translated (spec 09G.4); only the strings in i18n/*.json are.

export const HOST_NAME = "dev.mypack.companion";
export const PROTOCOL_VERSION = 1;
export const DOCS_URL = "https://mypack.dev/docs/companion";
export const LOCALES = ["en", "zh-Hant", "zh-Hans", "yue-Hant-HK", "es", "fr", "ja"];
export const LOCALE_NAMES = {
  "en": "English",
  "zh-Hant": "繁體中文",
  "zh-Hans": "简体中文",
  "yue-Hant-HK": "粵語（香港）",
  "es": "Español",
  "fr": "Français",
  "ja": "日本語"
};

// ---- locale resolution ----------------------------------------------------------

// Map a BCP-47 tag (browser UI language or Desktop preference) to one of the
// seven catalogs. Order: exact -> language family -> en. zh-HK/zh-MO map to
// zh-Hant (Chrome's zh-HK UI is written Traditional Chinese); the Cantonese
// catalog is only chosen explicitly (options) or when a tag says "yue".
export function normalizeLocale(tag) {
  if (!tag || typeof tag !== "string") return null;
  const t = tag.replace(/_/g, "-").toLowerCase();
  const exact = LOCALES.find((l) => l.toLowerCase() === t);
  if (exact) return exact;
  if (t === "yue" || t.startsWith("yue-")) return "yue-Hant-HK";
  if (t.startsWith("zh")) {
    if (/(^|-)hant(-|$)|-(tw|hk|mo)(-|$)/.test(t)) return "zh-Hant";
    return "zh-Hans"; // zh, zh-CN, zh-SG, zh-Hans-*
  }
  const lang = t.split("-")[0];
  return LOCALES.find((l) => l === lang) || null;
}

export function browserLocale() {
  let tag = "en";
  try { tag = chrome.i18n.getUILanguage() || navigator.language || "en"; } catch (_) { tag = navigator.language || "en"; }
  return normalizeLocale(tag) || "en";
}

// language setting: "browser" (default) | "desktop" | one of LOCALES.
export async function getSettings() {
  try {
    const s = await chrome.storage.local.get({ language: "browser", desktopLocale: null });
    return s;
  } catch (_) {
    return { language: "browser", desktopLocale: null };
  }
}

export async function setSettings(patch) {
  try { await chrome.storage.local.set(patch); } catch (_) { /* ignore */ }
}

export function resolveLocale(settings) {
  const lang = settings.language;
  if (LOCALES.includes(lang)) return lang;
  if (lang === "desktop") {
    return normalizeLocale(settings.desktopLocale) || browserLocale();
  }
  return browserLocale();
}

// ---- catalogs --------------------------------------------------------------------

const cache = new Map();
async function loadCatalog(locale) {
  if (!cache.has(locale)) {
    const res = await fetch(chrome.runtime.getURL(`i18n/${locale}.json`));
    cache.set(locale, await res.json());
  }
  return cache.get(locale);
}

export async function createTranslator(locale) {
  const en = await loadCatalog("en");
  const cat = locale === "en" ? en : await loadCatalog(locale).catch(() => en);
  const t = (key, vars) => {
    let s = cat[key] ?? en[key] ?? key;
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v));
    return s;
  };
  t.locale = locale;
  return t;
}

export function applyDocumentLocale(locale) {
  document.documentElement.lang = locale;
  document.documentElement.dir = "ltr";
}

// ---- URL eligibility ---------------------------------------------------------------

const STORE_HOSTS = new Set(["chromewebstore.google.com", "microsoftedge.microsoft.com"]);

// Returns {ok, host, url} for a tab URL that may become an app.
export function checkTabUrl(raw) {
  if (!raw || raw.length > 2048) return { ok: false };
  let u;
  try { u = new URL(raw); } catch (_) { return { ok: false }; }
  if (u.protocol !== "http:" && u.protocol !== "https:") return { ok: false };
  if (u.username || u.password) return { ok: false };
  const host = u.hostname.toLowerCase();
  if (!host) return { ok: false };
  if (STORE_HOSTS.has(host)) return { ok: false };
  if (host === "chrome.google.com" && u.pathname.startsWith("/webstore")) return { ok: false };
  u.hash = "";
  return { ok: true, host, url: u.href };
}

// ---- native messaging client ----------------------------------------------------------

export class HostError extends Error {
  constructor(code, details) {
    super(code);
    this.code = code; // protocol code, or "HOST_MISSING" when the browser cannot start the host
    this.details = details;
  }
}

let seq = 0;

export class HostConnection {
  constructor(timeoutMs = 10000) {
    this.timeoutMs = timeoutMs;
    this.pending = new Map();
    this.port = null;
    this.dead = null;
  }

  connect() {
    if (this.port || this.dead) return;
    try {
      this.port = chrome.runtime.connectNative(HOST_NAME);
    } catch (e) {
      this.dead = new HostError("HOST_MISSING", String(e && e.message));
      return;
    }
    this.port.onMessage.addListener((msg) => {
      const p = msg && this.pending.get(msg.requestId);
      if (!p) return;
      this.pending.delete(msg.requestId);
      clearTimeout(p.timer);
      if (msg.ok) p.resolve(msg.result);
      else p.reject(new HostError((msg.error && msg.error.code) || "HOST_UNAVAILABLE", msg.error && msg.error.details));
    });
    this.port.onDisconnect.addListener(() => {
      const msg = (chrome.runtime.lastError && chrome.runtime.lastError.message) || "";
      // "Specified native messaging host not found." / "Access to the specified native messaging host is forbidden."
      const missing = /not found|forbidden|not exist/i.test(msg);
      this.dead = new HostError(missing ? "HOST_MISSING" : "HOST_UNAVAILABLE", msg);
      this.port = null;
      for (const [, p] of this.pending) { clearTimeout(p.timer); p.reject(this.dead); }
      this.pending.clear();
    });
  }

  request(action, payload = {}) {
    this.connect();
    if (this.dead) return Promise.reject(this.dead);
    const requestId = `${Date.now().toString(36)}-${++seq}`;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new HostError("HOST_UNAVAILABLE", "timeout"));
      }, this.timeoutMs);
      this.pending.set(requestId, { resolve, reject, timer });
      try {
        this.port.postMessage({ protocolVersion: PROTOCOL_VERSION, requestId, action, payload });
      } catch (e) {
        clearTimeout(timer);
        this.pending.delete(requestId);
        reject(new HostError("HOST_UNAVAILABLE", String(e && e.message)));
      }
    });
  }

  close() {
    try { this.port && this.port.disconnect(); } catch (_) { /* ignore */ }
    this.port = null;
  }
}

// Maps an error to the i18n key of its user-facing message.
export function errorKey(err) {
  switch (err && err.code) {
    case "UNSUPPORTED_URL": return "errorUnsupported";
    case "APP_NOT_FOUND": return "errorNotFound";
    case "RATE_LIMITED": return "errorRateLimited";
    default: return "errorGeneric";
  }
}

export function faviconSrc(u) {
  return typeof u === "string" && /^(https?:|data:image\/)/i.test(u) ? u : "";
}
