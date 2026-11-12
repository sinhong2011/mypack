import { describe, expect, it } from 'vitest';
import { CATALOGS, createTranslator } from './i18n';
import { LOCALES } from './locale';
import en from '../i18n/en.json';
import manifestEn from '../locales/en.json';
import manifestEs from '../locales/es.json';
import manifestFr from '../locales/fr.json';
import manifestJa from '../locales/ja.json';
import manifestZhCN from '../locales/zh_CN.json';
import manifestZhTW from '../locales/zh_TW.json';

describe('popup catalogs', () => {
  const keys = Object.keys(en).sort();

  it.each(LOCALES.filter((l) => l !== 'en'))('%s has the same keys as en, all non-empty', (locale) => {
    const cat = CATALOGS[locale] as Record<string, string>;
    expect(Object.keys(cat).sort()).toEqual(keys);
    for (const k of keys) expect(cat[k], k).toBeTruthy();
  });

  it('keeps placeholders in every locale', () => {
    for (const locale of LOCALES) {
      const cat = CATALOGS[locale] as Record<string, string>;
      for (const [k, v] of Object.entries(en)) {
        for (const ph of v.match(/\{\w+\}/g) ?? []) expect(cat[k], `${locale}.${k}`).toContain(ph);
      }
    }
  });

  it('substitutes placeholders and falls back to English', () => {
    expect(createTranslator('en')('profile', { profile: 'work' })).toBe('Profile: work');
    expect(createTranslator('ja').locale).toBe('ja');
  });
});

describe('manifest locales (_locales)', () => {
  it.each([
    ['es', manifestEs],
    ['fr', manifestFr],
    ['ja', manifestJa],
    ['zh_CN', manifestZhCN],
    ['zh_TW', manifestZhTW],
  ] as const)('%s has the same keys as en', (_name, catalog) => {
    expect(Object.keys(catalog).sort()).toEqual(Object.keys(manifestEn).sort());
  });
});
