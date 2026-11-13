// @vitest-environment jsdom
import { render } from 'solid-js/web';
import { afterEach, expect, it, vi } from 'vitest';

const store: Record<string, unknown> = {};
vi.hoisted(() => {
  (globalThis as Record<string, unknown>).chrome = {
    runtime: { id: 'test', connectNative: () => ({}) },
    i18n: { getUILanguage: () => 'en' },
    storage: {
      local: {
        get: async (d: object) => ({ ...d, ...(globalThis as { __store?: object }).__store }),
        set: async (p: object) => void Object.assign((globalThis as { __store?: object }).__store!, p),
      },
    },
  };
});
(globalThis as { __store?: object }).__store = store;
const { default: App } = await import('./App');

let dispose: (() => void) | undefined;
afterEach(() => dispose?.());

it('shows the stored language and saves a new choice', async () => {
  Object.assign(store, { language: 'yue-Hant-HK', desktopLocale: 'fr' });
  const root = document.createElement('main');
  document.body.replaceChildren(root);
  dispose = render(() => <App />, root);
  const select = root.querySelector('select')!;
  await vi.waitFor(() => expect(select.value).toBe('yue-Hant-HK'));
  expect(select.options).toHaveLength(9);
  expect(document.documentElement.lang).toBe('yue-Hant-HK');
  expect(root.textContent).toContain('Français');

  select.value = 'ja';
  select.dispatchEvent(new Event('change'));
  await vi.waitFor(() => expect(root.querySelector('h1')?.textContent).toBe('MyPack コンパニオンの設定'));
  expect(store.language).toBe('ja');
  expect(select.value).toBe('ja');
});
