import { describe, expect, it } from 'vitest';
import { normalizeLocale, resolveLocale } from './locale';

describe('normalizeLocale', () => {
  it.each([
    ['en', 'en'],
    ['en-GB', 'en'],
    ['en_US', 'en'],
    ['zh-TW', 'zh-Hant'],
    ['zh-HK', 'zh-Hant'],
    ['zh-MO', 'zh-Hant'],
    ['zh-Hant-HK', 'zh-Hant'],
    ['zh_TW', 'zh-Hant'],
    ['zh', 'zh-Hans'],
    ['zh-CN', 'zh-Hans'],
    ['zh-SG', 'zh-Hans'],
    ['zh-Hans', 'zh-Hans'],
    ['yue', 'yue-Hant-HK'],
    ['yue-HK', 'yue-Hant-HK'],
    ['yue-Hant-HK', 'yue-Hant-HK'],
    ['es-419', 'es'],
    ['fr-CA', 'fr'],
    ['ja-JP', 'ja'],
  ])('%s → %s', (tag, want) => {
    expect(normalizeLocale(tag)).toBe(want);
  });

  it('returns null for unknown or empty tags', () => {
    expect(normalizeLocale('de')).toBeNull();
    expect(normalizeLocale('')).toBeNull();
    expect(normalizeLocale(undefined)).toBeNull();
    expect(normalizeLocale(42)).toBeNull();
  });
});

describe('resolveLocale', () => {
  const ui = () => 'fr' as const;
  it('follows the browser by default', () => {
    expect(resolveLocale({ language: 'browser', desktopLocale: 'ja' }, ui)).toBe('fr');
  });
  it('uses an explicit choice, including Cantonese', () => {
    expect(resolveLocale({ language: 'yue-Hant-HK', desktopLocale: null }, ui)).toBe('yue-Hant-HK');
  });
  it('follows Desktop when known, else the browser', () => {
    expect(resolveLocale({ language: 'desktop', desktopLocale: 'zh-HK' }, ui)).toBe('zh-Hant');
    expect(resolveLocale({ language: 'desktop', desktopLocale: null }, ui)).toBe('fr');
  });
  it('treats an unknown stored value as follow browser', () => {
    expect(resolveLocale({ language: 'klingon', desktopLocale: null }, ui)).toBe('fr');
  });
});
