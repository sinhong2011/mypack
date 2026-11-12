// The popup, options page and context-menu catalogs. Bundled (no fetch); `en` is the source of truth and the fallback
// for missing keys. Placeholders are `{name}`.
import en from '../i18n/en.json';
import es from '../i18n/es.json';
import fr from '../i18n/fr.json';
import ja from '../i18n/ja.json';
import yueHantHK from '../i18n/yue-Hant-HK.json';
import zhHans from '../i18n/zh-Hans.json';
import zhHant from '../i18n/zh-Hant.json';
import type { Locale } from './locale';

export type MessageKey = keyof typeof en;
export type Catalog = Partial<Record<MessageKey, string>>;

export const CATALOGS: Record<Locale, Catalog> = {
  en,
  'zh-Hant': zhHant,
  'zh-Hans': zhHans,
  'yue-Hant-HK': yueHantHK,
  es,
  fr,
  ja,
};

export interface Translator {
  (key: MessageKey, vars?: Record<string, string | number>): string;
  readonly locale: Locale;
}

export function createTranslator(locale: Locale): Translator {
  const cat = CATALOGS[locale] ?? en;
  const t = (key: MessageKey, vars?: Record<string, string | number>) => {
    let s: string = cat[key] ?? en[key] ?? key;
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v));
    return s;
  };
  return Object.assign(t, { locale });
}

export function applyDocumentLocale(locale: Locale) {
  document.documentElement.lang = locale;
  document.documentElement.dir = 'ltr';
}
