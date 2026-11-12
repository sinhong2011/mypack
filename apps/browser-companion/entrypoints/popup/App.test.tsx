// @vitest-environment jsdom
// Popup states against a fake Native Messaging port, plus an opt-in run against the real host binary:
//   go build -o /tmp/mypack-native-host ./cmd/mypack-native-host
//   MYPACK_NATIVE_HOST=/tmp/mypack-native-host pnpm test
import { spawn } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { render } from 'solid-js/web';
import { afterEach, describe, expect, it, vi } from 'vitest';

// `wxt/browser` binds to `globalThis.chrome` when first imported, so install one object up front and reconfigure it
// per test.
const chromeStub = vi.hoisted(() => {
  const stub = {} as Record<string, unknown>;
  (globalThis as Record<string, unknown>).chrome = stub;
  return stub;
});
const { default: App } = await import('./App');

type Req = { protocolVersion: number; requestId: string; action: string; payload: Record<string, unknown> };
type Reply = { ok: true; result: unknown } | { ok: false; error: { code: string } };
type Listener = (...args: unknown[]) => void;

const HELLO = { hostVersion: 'test', protocolVersion: 1, actions: ['hello', 'lookup', 'createDraft', 'openManagedApp', 'openManager'], locale: 'en' };

interface FakeChrome {
  runtime: { id: string; lastError?: { message: string }; connectNative: () => unknown };
  i18n: { getUILanguage: () => string };
  storage: { local: { get: (d: object) => Promise<object>; set: (p: object) => Promise<void> } };
  tabs: { query: () => Promise<unknown[]> };
}

function makePort(handle: (req: Req) => Reply | Promise<Reply>, log: Req[] = []) {
  const onMessage: Listener[] = [];
  return {
    postMessage(req: Req) {
      log.push(req);
      void Promise.resolve(handle(req)).then((r) => onMessage.forEach((l) => l({ requestId: req.requestId, ...r })));
    },
    onMessage: { addListener: (l: Listener) => onMessage.push(l) },
    onDisconnect: { addListener: () => {} },
    disconnect() {},
  };
}

function missingPort(chrome: FakeChrome) {
  const onDisconnect: Listener[] = [];
  queueMicrotask(() => {
    chrome.runtime.lastError = { message: 'Specified native messaging host not found.' };
    onDisconnect.forEach((l) => l());
    chrome.runtime.lastError = undefined;
  });
  return {
    postMessage() {},
    onMessage: { addListener() {} },
    onDisconnect: { addListener: (l: Listener) => onDisconnect.push(l) },
    disconnect() {},
  };
}

/** A port to the real `mypack-native-host`, framed like Chromium does (4-byte little-endian length + JSON). */
function realPort(bin: string, env: NodeJS.ProcessEnv) {
  const child = spawn(bin, [], { env, stdio: ['pipe', 'pipe', 'inherit'] });
  const onMessage: Listener[] = [];
  let buf = Buffer.alloc(0);
  child.stdout.on('data', (chunk: Buffer) => {
    buf = Buffer.concat([buf, chunk]);
    while (buf.length >= 4 && buf.length >= 4 + buf.readUInt32LE(0)) {
      const n = buf.readUInt32LE(0);
      const msg = JSON.parse(buf.subarray(4, 4 + n).toString('utf8'));
      buf = buf.subarray(4 + n);
      onMessage.forEach((l) => l(msg));
    }
  });
  return {
    postMessage(req: Req) {
      // openManager would bring the real Desktop app to the front; answer it locally instead.
      if (req.action === 'openManager' || req.action === 'openManagedApp') {
        queueMicrotask(() => onMessage.forEach((l) => l({ requestId: req.requestId, ok: true, result: {} })));
        return;
      }
      const body = Buffer.from(JSON.stringify(req), 'utf8');
      const head = Buffer.alloc(4);
      head.writeUInt32LE(body.length, 0);
      child.stdin.write(Buffer.concat([head, body]));
    },
    onMessage: { addListener: (l: Listener) => onMessage.push(l) },
    onDisconnect: { addListener() {} },
    disconnect: () => child.kill(),
  };
}

let dispose: (() => void) | undefined;

async function mount(chrome: FakeChrome) {
  for (const k of Object.keys(chromeStub)) delete chromeStub[k];
  Object.assign(chromeStub, chrome);
  const root = document.createElement('main');
  document.body.replaceChildren(root);
  dispose = render(() => <App root={root} />, root);
  return root;
}

function fakeChrome(port: (c: FakeChrome) => unknown, url = 'https://example.com/path#x', store: Record<string, unknown> = {}): FakeChrome {
  const chrome: FakeChrome = {
    runtime: { id: 'abcdefghijklmnopabcdefghijklmnop', connectNative: () => port(chrome) },
    i18n: { getUILanguage: () => 'en-US' },
    storage: {
      local: {
        get: async (d) => ({ ...d, ...store }),
        set: async (p) => void Object.assign(store, p),
      },
    },
    tabs: { query: async () => [{ url, title: 'Example Site', favIconUrl: '' }] },
  };
  return chrome;
}

const button = (root: HTMLElement, text: string) =>
  [...root.querySelectorAll('button')].find((b) => b.textContent?.includes(text)) as HTMLButtonElement | undefined;

afterEach(() => {
  dispose?.();
  dispose = undefined;
});

describe('popup', () => {
  it('shows the restricted state for non-web pages without contacting the host', async () => {
    const connect = vi.fn();
    const chrome = fakeChrome(connect, 'chrome://extensions');
    const root = await mount(chrome);
    await vi.waitFor(() => expect(root.textContent).toContain("This page can't become an app"));
    expect(connect).not.toHaveBeenCalled();
  });

  it('creates a draft for an unmanaged site and opens the manager', async () => {
    const log: Req[] = [];
    const port = makePort((req) => {
      if (req.action === 'hello') return { ok: true, result: HELLO };
      if (req.action === 'lookup') return { ok: true, result: { match: 'none', matches: [] } };
      if (req.action === 'createDraft') return { ok: true, result: { projectId: 'p1' } };
      return { ok: true, result: {} };
    }, log);
    const root = await mount(fakeChrome(() => port));
    await vi.waitFor(() => expect(button(root, 'Create Desktop App')).toBeTruthy());
    expect(root.querySelector('.host')?.textContent).toBe('example.com');
    expect(document.activeElement).toBe(button(root, 'Create Desktop App'));
    button(root, 'Create Desktop App')!.click();
    await vi.waitFor(() => expect(root.textContent).toContain('Draft created'));
    expect(log.map((r) => r.action)).toEqual(['hello', 'lookup', 'createDraft', 'openManager']);
    expect(log[1]!.payload).toEqual({ url: 'https://example.com/path', title: 'Example Site' });
    expect(log[3]!.payload).toEqual({ projectId: 'p1' });
    expect(log.every((r) => r.protocolVersion === 1)).toBe(true);
    expect(button(root, 'Manage in mypack')).toBeTruthy();
  });

  it('offers Open and Manage for a unique installed match', async () => {
    const log: Req[] = [];
    const m = { projectId: 'p1', installationId: 'i1', name: 'Example', state: 'managed', profileId: 'work' };
    const port = makePort((req) => {
      if (req.action === 'hello') return { ok: true, result: HELLO };
      if (req.action === 'lookup') return { ok: true, result: { match: 'unique', matches: [m] } };
      return { ok: true, result: {} };
    }, log);
    const root = await mount(fakeChrome(() => port));
    await vi.waitFor(() => expect(button(root, 'Open in Desktop App')).toBeTruthy());
    expect(root.textContent).toContain('Installed · Profile: work');
    button(root, 'Open in Desktop App')!.click();
    await vi.waitFor(() => expect(button(root, 'Opened in mypack.')).toBeTruthy());
    expect(log.at(-1)).toMatchObject({ action: 'openManagedApp', payload: { installationId: 'i1' } });
  });

  it('lists several matches without choosing one', async () => {
    const ms = [
      { projectId: 'a', name: 'A', state: 'draft' },
      { projectId: 'b', installationId: 'ib', name: 'B', state: 'managed' },
    ];
    const port = makePort((req) =>
      req.action === 'hello' ? { ok: true, result: HELLO } : { ok: true, result: { match: 'multiple', matches: ms } },
    );
    const root = await mount(fakeChrome(() => port));
    await vi.waitFor(() => expect(root.textContent).toContain('Choose an app'));
    expect(root.querySelectorAll('ul.choices button')).toHaveLength(2);
    expect(root.querySelector('ul.choices button')?.getAttribute('aria-label')).toBe('A, Draft');
  });

  it('explains how to register a missing host', async () => {
    const root = await mount(fakeChrome((c) => missingPort(c)));
    await vi.waitFor(() => expect(root.textContent).toContain('mypack helper not found'));
    expect(root.querySelector('code')?.textContent).toBe('mypack-native-host install --extension-id abcdefghijklmnopabcdefghijklmnop');
  });

  it('reports a protocol mismatch', async () => {
    const port = makePort(() => ({ ok: true, result: { ...HELLO, protocolVersion: 2 } }));
    const root = await mount(fakeChrome(() => port));
    await vi.waitFor(() => expect(root.textContent).toContain('Update mypack or the companion'));
  });

  it('shows the localized error for a failed action', async () => {
    const port = makePort((req) => {
      if (req.action === 'hello') return { ok: true, result: HELLO };
      if (req.action === 'lookup') return { ok: true, result: { match: 'none' } };
      return { ok: false, error: { code: 'RATE_LIMITED' } };
    });
    const root = await mount(fakeChrome(() => port));
    await vi.waitFor(() => expect(button(root, 'Create Desktop App')).toBeTruthy());
    button(root, 'Create Desktop App')!.click();
    await vi.waitFor(() => expect(root.textContent).toContain('Too many requests'));
    expect(button(root, 'Create Desktop App')!.disabled).toBe(false);
  });

  it('follows the Desktop language when asked to', async () => {
    const store = { language: 'desktop' };
    const port = makePort((req) =>
      req.action === 'hello' ? { ok: true, result: { ...HELLO, locale: 'ja-JP' } } : { ok: true, result: { match: 'none' } },
    );
    const root = await mount(fakeChrome(() => port, undefined, store));
    await vi.waitFor(() => expect(button(root, 'デスクトップアプリを作成')).toBeTruthy());
    expect(store).toMatchObject({ desktopLocale: 'ja' });
    expect(document.documentElement.lang).toBe('ja');
  });
});

const HOST_BIN = process.env.MYPACK_NATIVE_HOST;

describe.skipIf(!HOST_BIN)('popup against the real native host', () => {
  it('creates a draft, then finds it on the next lookup', async () => {
    const tmp = mkdtempSync(join(tmpdir(), 'mypack-companion-'));
    const env = { ...process.env, MYPACK_DB: join(tmp, 'mypack.db'), MYPACK_CACHE_DIR: join(tmp, 'cache') };
    const ports: ReturnType<typeof realPort>[] = [];
    const connect = () => {
      const p = realPort(HOST_BIN!, env);
      ports.push(p);
      return p;
    };
    try {
      let root = await mount(fakeChrome(connect, 'https://companion-e2e.invalid/start'));
      await vi.waitFor(() => expect(button(root, 'Create Desktop App')).toBeTruthy(), { timeout: 5000 });
      button(root, 'Create Desktop App')!.click();
      await vi.waitFor(() => expect(root.textContent).toContain('Draft created'), { timeout: 5000 });
      dispose?.();

      root = await mount(fakeChrome(connect, 'https://companion-e2e.invalid/other'));
      await vi.waitFor(() => expect(button(root, 'Manage in mypack')).toBeTruthy(), { timeout: 5000 });
      expect(root.textContent).toContain('Draft');
    } finally {
      ports.forEach((p) => p.disconnect());
    }
  });
});
