// Native Messaging client for `cmd/mypack-native-host` (protocol v1, docs/companion.md).
// Protocol keys and error codes are never translated (spec 09G.4); only the strings in i18n/*.json are.
import { browser } from 'wxt/browser';

export const HOST_NAME = 'dev.mypack.companion';
export const PROTOCOL_VERSION = 1;
export const DOCS_URL = 'https://mypack.dev/docs/companion';

export type Action = 'hello' | 'lookup' | 'createDraft' | 'openManagedApp' | 'openManager';

/** Protocol error codes, plus `HOST_MISSING` when the browser cannot start the host at all. */
export type ErrorCode =
  | 'HOST_MISSING'
  | 'HOST_UNAVAILABLE'
  | 'APP_NOT_FOUND'
  | 'AMBIGUOUS_MATCH'
  | 'UNSUPPORTED_URL'
  | 'PROTOCOL_MISMATCH'
  | 'INVALID_REQUEST'
  | 'RATE_LIMITED'
  | (string & {});

export interface Hello {
  hostVersion: string;
  protocolVersion: number;
  actions?: string[];
  locale?: string;
}

export type MatchState = 'managed' | 'missing' | 'attention' | 'external_readonly' | 'draft' | (string & {});

export interface Match {
  projectId?: string;
  installationId?: string;
  name: string;
  state: MatchState;
  profileId?: string;
}

export interface Lookup {
  match: 'none' | 'unique' | 'multiple';
  matches?: Match[];
}

export interface Draft {
  projectId: string;
}

interface Response {
  requestId: string;
  ok: boolean;
  result?: unknown;
  error?: { code?: string; details?: unknown };
}

export class HostError extends Error {
  readonly code: ErrorCode;
  readonly details: unknown;
  constructor(code: ErrorCode, details?: unknown) {
    super(code);
    this.code = code;
    this.details = details;
  }
}

type Port = ReturnType<typeof browser.runtime.connectNative>;
interface Pending {
  resolve: (value: unknown) => void;
  reject: (reason: HostError) => void;
  timer: ReturnType<typeof setTimeout>;
}

let seq = 0;

export class HostConnection {
  private pending = new Map<string, Pending>();
  private port: Port | null = null;
  dead: HostError | null = null;

  constructor(private readonly timeoutMs = 10000) {}

  private connect() {
    if (this.port || this.dead) return;
    let port: Port;
    try {
      port = browser.runtime.connectNative(HOST_NAME);
    } catch (e) {
      this.dead = new HostError('HOST_MISSING', String((e as Error)?.message));
      return;
    }
    this.port = port;
    port.onMessage.addListener((msg: Response) => {
      const p = msg && this.pending.get(msg.requestId);
      if (!p) return;
      this.pending.delete(msg.requestId);
      clearTimeout(p.timer);
      if (msg.ok) p.resolve(msg.result);
      else p.reject(new HostError(msg.error?.code || 'HOST_UNAVAILABLE', msg.error?.details));
    });
    port.onDisconnect.addListener(() => {
      const msg = browser.runtime.lastError?.message || '';
      // "Specified native messaging host not found." / "Access to the specified native messaging host is forbidden."
      const missing = /not found|forbidden|not exist/i.test(msg);
      this.dead = new HostError(missing ? 'HOST_MISSING' : 'HOST_UNAVAILABLE', msg);
      this.port = null;
      for (const [, p] of this.pending) {
        clearTimeout(p.timer);
        p.reject(this.dead);
      }
      this.pending.clear();
    });
  }

  request(action: 'hello'): Promise<Hello>;
  request(action: 'lookup', payload: { url: string; title?: string }): Promise<Lookup>;
  request(action: 'createDraft', payload: { url: string; title?: string; faviconUrl?: string }): Promise<Draft>;
  request(action: 'openManagedApp', payload: { installationId: string }): Promise<unknown>;
  request(action: 'openManager', payload: { projectId?: string }): Promise<unknown>;
  request(action: Action, payload: Record<string, unknown> = {}): Promise<unknown> {
    this.connect();
    if (this.dead) return Promise.reject(this.dead);
    const requestId = `${Date.now().toString(36)}-${++seq}`;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new HostError('HOST_UNAVAILABLE', 'timeout'));
      }, this.timeoutMs);
      this.pending.set(requestId, { resolve, reject, timer });
      try {
        this.port!.postMessage({ protocolVersion: PROTOCOL_VERSION, requestId, action, payload });
      } catch (e) {
        clearTimeout(timer);
        this.pending.delete(requestId);
        reject(new HostError('HOST_UNAVAILABLE', String((e as Error)?.message)));
      }
    });
  }

  /** Closes the port. `reset` also forgets a previous failure so the next request tries again. */
  close(reset = false) {
    try {
      this.port?.disconnect();
    } catch {
      /* ignore */
    }
    this.port = null;
    if (reset) this.dead = null;
  }
}

/** The i18n key of an error's user-facing message. */
export function errorKey(err: unknown): string {
  switch ((err as HostError | undefined)?.code) {
    case 'UNSUPPORTED_URL':
      return 'errorUnsupported';
    case 'APP_NOT_FOUND':
      return 'errorNotFound';
    case 'RATE_LIMITED':
      return 'errorRateLimited';
    default:
      return 'errorGeneric';
  }
}
