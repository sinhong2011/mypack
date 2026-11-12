// Options: the language override, a connection check against the native host, and what the companion can do.
import { createSignal, For, onCleanup, onMount } from 'solid-js';
import { HostConnection } from '../../lib/host';
import { applyDocumentLocale, createTranslator, type Translator } from '../../lib/i18n';
import { getSettings, LOCALE_NAMES, LOCALES, normalizeLocale, resolveLocale, setSettings, type Settings } from '../../lib/locale';

export default function App() {
  const [t, setT] = createSignal<Translator>(createTranslator('en'));
  const [settings, setSettingsSignal] = createSignal<Settings>({ language: 'browser', desktopLocale: null });
  const [saved, setSaved] = createSignal('');
  const [connStatus, setConnStatus] = createSignal('');
  let savedTimer: ReturnType<typeof setTimeout> | undefined;
  onCleanup(() => clearTimeout(savedTimer));

  async function paint() {
    const s = await getSettings();
    const locale = resolveLocale(s);
    const tr = createTranslator(locale);
    setT(() => tr);
    setSettingsSignal(s);
    applyDocumentLocale(locale);
    document.title = tr('optionsTitle');
  }

  onMount(paint);

  const desktopLanguage = () => {
    const dl = settings().desktopLocale;
    if (!dl) return t()('desktopLanguageUnknown');
    const norm = normalizeLocale(dl);
    return t()('desktopLanguageNow', { locale: (norm && LOCALE_NAMES[norm]) || dl });
  };

  const onLanguage = async (value: string) => {
    await setSettings({ language: value });
    await paint();
    setSaved(t()('saved'));
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => setSaved(''), 2500);
  };

  const check = async () => {
    const host = new HostConnection(5000);
    setConnStatus('…');
    try {
      const hello = await host.request('hello');
      setConnStatus(t()('connectionOk', { version: hello.hostVersion }));
      const dl = normalizeLocale(hello.locale);
      if (dl) {
        await setSettings({ desktopLocale: dl });
        await paint();
      }
    } catch {
      setConnStatus(t()('connectionFail'));
    } finally {
      host.close();
    }
  };

  return (
    <>
      <h1>{t()('optionsTitle')}</h1>
      <div class="row">
        <label for="lang">{t()('languageLabel')}</label>
        <select id="lang" value={settings().language} onChange={(e) => void onLanguage(e.currentTarget.value)}>
          <option value="browser">{t()('langFollowBrowser')}</option>
          <option value="desktop">{t()('langFollowDesktop')}</option>
          <For each={LOCALES}>
            {(l) => (
              <option value={l} lang={l}>
                {LOCALE_NAMES[l]}
              </option>
            )}
          </For>
        </select>
        <p class="muted">{t()('languageHelp')}</p>
        <p class="muted">{desktopLanguage()}</p>
        <p class="status" role="status">
          {saved()}
        </p>
      </div>
      <div class="row">
        <h2>{t()('connectionLabel')}</h2>
        <p role="status">{connStatus()}</p>
        <button type="button" onClick={() => void check()}>
          {t()('checkConnection')}
        </button>
      </div>
      <section class="card">
        <h2>{t()('privacyTitle')}</h2>
        <p class="muted">{t()('privacyBody')}</p>
      </section>
    </>
  );
}
