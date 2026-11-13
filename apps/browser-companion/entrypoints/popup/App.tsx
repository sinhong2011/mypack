// The toolbar popup: looks the current tab up in MyPack Desktop and offers to open the matching app, manage it, or
// create a draft. Nothing is ever built or installed from here.
import { createEffect, createSignal, For, type JSX, Match, on, onCleanup, onMount, Show, Switch } from 'solid-js';
import { browser } from 'wxt/browser';
import {
  DOCS_URL,
  type Draft,
  errorKey,
  HostConnection,
  HostError,
  type Match as AppMatch,
  PROTOCOL_VERSION,
} from '../../lib/host';
import { applyDocumentLocale, createTranslator, type MessageKey, type Translator } from '../../lib/i18n';
import { getSettings, normalizeLocale, resolveLocale, setSettings } from '../../lib/locale';
import { checkTabUrl, faviconSrc } from '../../lib/url';

interface TabInfo {
  host: string;
  url: string;
  title: string;
  favicon?: string;
}

type View =
  | { kind: 'checking' }
  | { kind: 'restricted' }
  | { kind: 'hostMissing' }
  | { kind: 'hostError' }
  | { kind: 'protocol' }
  | { kind: 'notManaged' }
  | { kind: 'unique'; match: AppMatch }
  | { kind: 'multiple'; matches: AppMatch[] }
  | { kind: 'draftCreated'; draft: Draft; opened: boolean };

const REQUIRED_ACTIONS = ['lookup', 'createDraft', 'openManager'];

export default function App(props: { root: HTMLElement }) {
  const host = new HostConnection();
  const [translator, setTranslator] = createSignal<Translator>(createTranslator('en'));
  const t = (key: MessageKey, vars?: Record<string, string | number>) => translator()(key, vars);
  const [view, setViewSignal] = createSignal<View>({ kind: 'checking' });
  const [message, setMessage] = createSignal<MessageKey | null>(null);
  let tab: TabInfo = { host: '', url: '', title: '' };

  const setView = (next: View) => {
    setMessage(null);
    setViewSignal(next);
  };

  // Every view change: announce busy/idle and move focus to the view's main action.
  createEffect(
    on(view, (v) => {
      props.root.setAttribute('aria-busy', v.kind === 'checking' ? 'true' : 'false');
      queueMicrotask(() => props.root.querySelector<HTMLElement>('[data-autofocus]')?.focus());
    }),
  );

  /** Shared failure handling for the action buttons. */
  const fail = (err: unknown) => {
    if (err instanceof HostError && err.code === 'PROTOCOL_MISMATCH') return setView({ kind: 'protocol' });
    if (err instanceof HostError && err.code === 'HOST_MISSING') return setView({ kind: 'hostMissing' });
    setMessage(errorKey(err) as MessageKey);
  };

  /** A button that shows `busyLabel` while its action runs; the action may return a new label for afterwards. */
  function ActionButton(p: {
    label?: string;
    busyLabel: string;
    run: () => Promise<string | void>;
    primary?: boolean;
    autofocus?: boolean;
    ariaLabel?: string;
    children?: JSX.Element;
  }) {
    const [busy, setBusy] = createSignal(false);
    const [done, setDone] = createSignal<string>();
    const click = async () => {
      setBusy(true);
      try {
        const next = await p.run();
        setBusy(false);
        if (next) setDone(next);
      } catch (err) {
        setBusy(false);
        fail(err);
      }
    };
    return (
      <button
        type="button"
        class={p.primary ? 'primary' : undefined}
        data-autofocus={p.autofocus || undefined}
        aria-label={p.ariaLabel}
        disabled={busy()}
        onClick={click}
      >
        {busy() ? p.busyLabel : (done() ?? p.children ?? p.label)}
      </button>
    );
  }

  // ---- actions --------------------------------------------------------------------------

  const createDraft = async () => {
    const draft = await host.request('createDraft', {
      url: tab.url,
      title: tab.title || '',
      faviconUrl: faviconSrc(tab.favicon).startsWith('http') ? tab.favicon : undefined,
    });
    let opened = true;
    try {
      await host.request('openManager', { projectId: draft.projectId });
    } catch {
      opened = false;
    }
    setView({ kind: 'draftCreated', draft, opened });
  };

  const openMatch = async (m: AppMatch) => {
    if (m.installationId) await host.request('openManagedApp', { installationId: m.installationId });
    else await host.request('openManager', { projectId: m.projectId });
    return t('opened');
  };

  const manageMatch = async (m: AppMatch) => {
    await host.request('openManager', { projectId: m.projectId || undefined });
    return t('manageApp');
  };

  const stateLabel = (s: string) => {
    switch (s) {
      case 'managed':
        return t('stateManaged');
      case 'missing':
        return t('stateMissing');
      case 'attention':
        return t('stateAttention');
      case 'external_readonly':
        return t('stateExternal');
      case 'draft':
        return t('stateDraft');
      default:
        return t('stateProject');
    }
  };
  const profileSuffix = (m: AppMatch, sep: string) => (m.profileId ? sep + t('profile', { profile: m.profileId }) : '');

  // ---- startup --------------------------------------------------------------------------

  async function start() {
    setView({ kind: 'checking' });
    host.close(true);
    let hello;
    try {
      hello = await host.request('hello');
    } catch (err) {
      const code = (err as HostError).code;
      if (code === 'PROTOCOL_MISMATCH') return setView({ kind: 'protocol' });
      return setView({ kind: code === 'HOST_MISSING' ? 'hostMissing' : 'hostError' });
    }
    if (hello.protocolVersion !== PROTOCOL_VERSION || !REQUIRED_ACTIONS.every((a) => (hello.actions || []).includes(a))) {
      return setView({ kind: 'protocol' });
    }
    // Optional "Follow MyPack Desktop": remember the Desktop locale, re-render if it changes ours.
    const dl = normalizeLocale(hello.locale);
    const settings = await getSettings();
    if (dl && dl !== settings.desktopLocale) await setSettings({ desktopLocale: dl });
    if (settings.language === 'desktop' && dl && dl !== translator().locale) {
      setTranslator(() => createTranslator(dl));
      applyDocumentLocale(dl);
    }
    let lookup;
    try {
      lookup = await host.request('lookup', { url: tab.url, title: tab.title || '' });
    } catch (err) {
      const code = (err as HostError).code;
      if (code === 'UNSUPPORTED_URL') return setView({ kind: 'restricted' });
      if (code === 'PROTOCOL_MISMATCH') return setView({ kind: 'protocol' });
      return setView({ kind: code === 'HOST_MISSING' ? 'hostMissing' : 'hostError' });
    }
    const ms = lookup.matches || [];
    if (lookup.match === 'unique' && ms.length === 1) return setView({ kind: 'unique', match: ms[0]! });
    if (lookup.match === 'multiple' || ms.length > 1) return setView({ kind: 'multiple', matches: ms });
    return setView({ kind: 'notManaged' });
  }

  onMount(async () => {
    const locale = resolveLocale(await getSettings());
    setTranslator(() => createTranslator(locale));
    applyDocumentLocale(locale);
    document.title = t('appName');
    const [active] = await browser.tabs.query({ active: true, currentWindow: true }).catch(() => []);
    const chk = checkTabUrl(active?.url);
    if (!chk.ok) return setView({ kind: 'restricted' });
    tab = { host: chk.host, url: chk.url, title: (active!.title || '').slice(0, 200), favicon: active!.favIconUrl };
    void start();
  });

  const closeHost = () => host.close();
  window.addEventListener('pagehide', closeHost);
  onCleanup(() => window.removeEventListener('pagehide', closeHost));

  // ---- views --------------------------------------------------------------------------

  function SiteHeader() {
    const [iconFailed, setIconFailed] = createSignal(false);
    const src = faviconSrc(tab.favicon);
    return (
      <div class="site">
        <div class="fav" aria-hidden="true">
          <Show when={src && !iconFailed()} fallback={(tab.host[0] || '?').toUpperCase()}>
            <img src={src} alt="" onError={() => setIconFailed(true)} />
          </Show>
        </div>
        <div class="site-text">
          <div class="host">{tab.host}</div>
          <div class="title">{tab.title || t('untitled')}</div>
        </div>
      </div>
    );
  }

  function ErrorCard(p: { title: string; body?: string; children?: JSX.Element }) {
    return (
      <section class="card error" role="alert">
        <h2 class="err">{p.title}</h2>
        <Show when={p.body}>
          <p>{p.body}</p>
        </Show>
        {p.children}
      </section>
    );
  }

  const CreateAnother = () => (
    <button type="button" class="link" onClick={() => setView({ kind: 'notManaged' })}>
      {t('createAnother')}
    </button>
  );

  const Retry = (p: { primary?: boolean }) => (
    <button type="button" class={p.primary ? 'primary' : undefined} data-autofocus onClick={() => void start()}>
      {t('retry')}
    </button>
  );

  return (
    <>
      <Switch>
        <Match when={view().kind === 'checking'}>
          <p class="muted">{t('checking')}</p>
        </Match>

        <Match when={view().kind === 'restricted'}>
          <section class="card">
            <h2>{t('restrictedTitle')}</h2>
            <p class="muted">{t('restrictedBody')}</p>
          </section>
        </Match>

        <Match when={view().kind === 'hostMissing'}>
          <ErrorCard title={t('hostMissingTitle')} body={t('hostMissingBody')}>
            <div>
              <ol>
                <li>{t('hostStep1')}</li>
                <li>
                  {t('hostStep2')}
                  <code tabindex="0">{`mypack-native-host install --extension-id ${browser.runtime.id}`}</code>
                </li>
                <li>{t('hostStep3')}</li>
              </ol>
              <p class="muted">{t('docsLink', { url: DOCS_URL })}</p>
            </div>
          </ErrorCard>
          <Retry primary />
        </Match>

        <Match when={view().kind === 'hostError'}>
          <ErrorCard title={t('hostErrorTitle')} body={t('hostErrorBody')} />
          <Retry primary />
        </Match>

        <Match when={view().kind === 'protocol'}>
          <ErrorCard title={t('protocolTitle')} body={t('protocolBody')} />
          <Retry />
        </Match>

        <Match when={view().kind === 'notManaged'}>
          <SiteHeader />
          <ActionButton primary autofocus label={t('createApp')} busyLabel={t('creating')} run={createDraft} />
        </Match>

        <Match when={(() => { const v = view(); return v.kind === 'unique' && v; })()} keyed>
          {(v) => {
            const m = v.match;
            const canOpen = !!m.installationId && m.state !== 'missing';
            return (
              <>
                <SiteHeader />
                <section class="card">
                  <div class="choice-name">{m.name}</div>
                  <div class="choice-meta">{stateLabel(m.state) + profileSuffix(m, ' · ')}</div>
                </section>
                <Show
                  when={canOpen}
                  fallback={
                    <ActionButton primary autofocus label={t('manageApp')} busyLabel={t('opening')} run={() => manageMatch(m)} />
                  }
                >
                  <ActionButton primary autofocus label={t('openApp')} busyLabel={t('opening')} run={() => openMatch(m)} />
                  <ActionButton label={t('manageApp')} busyLabel={t('opening')} run={() => manageMatch(m)} />
                </Show>
                <CreateAnother />
              </>
            );
          }}
        </Match>

        <Match when={(() => { const v = view(); return v.kind === 'multiple' && v; })()} keyed>
          {(v) => (
            <>
              <SiteHeader />
              <h2>{t('chooseTitle')}</h2>
              <p class="muted">{t('chooseBody')}</p>
              <ul class="choices">
                <For each={v.matches}>
                  {(m, i) => (
                    <li>
                      <ActionButton
                        autofocus={i() === 0}
                        busyLabel={t('opening')}
                        ariaLabel={`${m.name}, ${stateLabel(m.state)}${profileSuffix(m, ', ')}`}
                        run={() => openMatch(m)}
                      >
                        <span class="choice-name">{m.name}</span>
                        <span class="choice-meta">{stateLabel(m.state) + profileSuffix(m, ' · ')}</span>
                      </ActionButton>
                    </li>
                  )}
                </For>
              </ul>
              <CreateAnother />
            </>
          )}
        </Match>

        <Match when={(() => { const v = view(); return v.kind === 'draftCreated' && v; })()} keyed>
          {(v) => (
            <>
              <SiteHeader />
              <section class="card success" role="status">
                <h2 class="ok">{t('draftCreated')}</h2>
                <p class="muted">{t('draftCreatedBody')}</p>
                <Show when={!v.opened}>
                  <p>{t('draftOpenFailed')}</p>
                </Show>
              </section>
              <Show
                when={v.opened}
                fallback={
                  <ActionButton
                    primary
                    autofocus
                    label={t('openMyPack')}
                    busyLabel={t('opening')}
                    run={async () => {
                      await host.request('openManager', { projectId: v.draft.projectId });
                      setView({ kind: 'draftCreated', draft: v.draft, opened: true });
                    }}
                  />
                }
              >
                <ActionButton
                  autofocus
                  label={t('manageApp')}
                  busyLabel={t('opening')}
                  run={async () => {
                    await host.request('openManager', { projectId: v.draft.projectId });
                    return t('manageApp');
                  }}
                />
              </Show>
            </>
          )}
        </Match>
      </Switch>
      <Show when={message()}>
        {(key) => (
          <p class="muted" role="alert">
            {t(key())}
          </p>
        )}
      </Show>
    </>
  );
}
