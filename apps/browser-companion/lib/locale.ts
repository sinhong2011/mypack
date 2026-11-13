// Language resolution: follow the browser (default), an explicit choice in options, or follow MyPack Desktop
// (the locale the host reports in `hello`, cached in storage). See docs/companion.md › Language mapping.
import { browser } from 'wxt/browser';

export const LOCALES = ['en', 'zh-Hant', 'zh-Hans', 'yue-Hant-HK', 'es', 'fr', 'ja'] as const;
export type Locale = (typeof LOCALES)[number];

export const LOCALE_NAMES: Record<Locale, string> = {
  en: 'English',
  'zh-Hant': '繁體中文',
  'zh-Hans': '简体中文',
  'yue-Hant-HK': '粵語',
  es: 'Español',
  fr: 'Français',
  ja: '日本語',
};

/** `browser` (default), `desktop`, or one of LOCALES. */
export type LanguageSetting = 'browser' | 'desktop' | Locale;

export interface Settings {
  language: LanguageSetting | string;
  desktopLocale: string | null;
}

const DEFAULTS: Settings = { language: 'browser', desktopLocale: null };

export function isLocale(value: unknown): value is Locale {
  return (LOCALES as readonly unknown[]).includes(value);
}

/**
 * Map a BCP-47 tag (browser UI language or Desktop preference) to one of the seven catalogs.
 * Order: exact → language family → null. zh-HK/zh-MO map to zh-Hant (Chrome's zh-HK UI is written Traditional
 * Chinese); the Cantonese catalog is only chosen explicitly (options) or when a tag says "yue".
 */
export function normalizeLocale(tag: unknown): Locale | null {
  if (!tag || typeof tag !== 'string') return null;
  const t = tag.replace(/_/g, '-').toLowerCase();
  const exact = LOCALES.find((l) => l.toLowerCase() === t);
  if (exact) return exact;
  if (t === 'yue' || t.startsWith('yue-')) return 'yue-Hant-HK';
  if (t.startsWith('zh')) {
    if (/(^|-)hant(-|$)|-(tw|hk|mo)(-|$)/.test(t)) return 'zh-Hant';
    return 'zh-Hans'; // zh, zh-CN, zh-SG, zh-Hans-*
  }
  const lang = t.split('-')[0];
  return LOCALES.find((l) => l === lang) ?? null;
}

export function browserLocale(): Locale {
  let tag = 'en';
  try {
    tag = browser.i18n.getUILanguage() || navigator.language || 'en';
  } catch {
    tag = (typeof navigator !== 'undefined' && navigator.language) || 'en';
  }
  return normalizeLocale(tag) ?? 'en';
}

export async function getSettings(): Promise<Settings> {
  try {
    return (await browser.storage.local.get({ ...DEFAULTS } as Record<string, unknown>)) as unknown as Settings;
  } catch {
    return { ...DEFAULTS };
  }
}

export async function setSettings(patch: Partial<Settings>): Promise<void> {
  try {
    await browser.storage.local.set(patch);
  } catch {
    /* ignore */
  }
}

export function resolveLocale(settings: Settings, uiLocale: () => Locale = browserLocale): Locale {
  const lang = settings.language;
  if (isLocale(lang)) return lang;
  if (lang === 'desktop') return normalizeLocale(settings.desktopLocale) ?? uiLocale();
  return uiLocale();
}
