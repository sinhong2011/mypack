import { getLocale } from "./paraglide/runtime.js";

/** The site's languages: the same seven as the app and the READMEs. `path` is the URL prefix. */
export const LOCALES = [
  { id: "en", lang: "en", code: "EN", label: "English", path: "" },
  { id: "zh-hant", lang: "zh-Hant-TW", code: "繁", label: "繁體中文", path: "zh-hant/" },
  { id: "zh-hans", lang: "zh-Hans", code: "简", label: "简体中文", path: "zh-hans/" },
  { id: "yue", lang: "yue-Hant-HK", code: "粵", label: "粵語", path: "yue/" },
  { id: "es", lang: "es", code: "ES", label: "Español", path: "es/" },
  { id: "fr", lang: "fr", code: "FR", label: "Français", path: "fr/" },
  { id: "ja", lang: "ja", code: "日", label: "日本語", path: "ja/" },
] as const;

export type Locale = (typeof LOCALES)[number];

export const currentLocale = (): Locale => LOCALES.find((l) => l.id === getLocale()) ?? LOCALES[0];

/** The home page in a given language, under the deploy base. */
export const localeHref = (l: Locale, page = "") => `${import.meta.env.BASE_URL.replace(/\/$/, "")}/${l.path}${page}`;

/** CJK pages get their own type stack (see global.css). */
export const isCJK = (l: Locale) => ["zh-hant", "zh-hans", "yue", "ja"].includes(l.id);
