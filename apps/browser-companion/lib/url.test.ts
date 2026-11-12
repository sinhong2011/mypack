import { describe, expect, it } from 'vitest';
import { checkTabUrl, faviconSrc } from './url';

describe('checkTabUrl', () => {
  it('accepts http(s) and strips the fragment', () => {
    expect(checkTabUrl('https://Example.com/a?b=1#frag')).toEqual({ ok: true, host: 'example.com', url: 'https://example.com/a?b=1' });
    expect(checkTabUrl('http://localhost:8080/')).toEqual({ ok: true, host: 'localhost', url: 'http://localhost:8080/' });
  });

  it.each([
    undefined,
    '',
    'not a url',
    'chrome://extensions',
    'edge://settings',
    'file:///etc/hosts',
    'about:blank',
    'https://user:pw@example.com/',
    'https://chromewebstore.google.com/detail/x',
    'https://microsoftedge.microsoft.com/addons/detail/x',
    'https://chrome.google.com/webstore/detail/x',
    `https://example.com/${'a'.repeat(2048)}`,
  ])('rejects %s', (raw) => {
    expect(checkTabUrl(raw).ok).toBe(false);
  });
});

describe('faviconSrc', () => {
  it('allows http(s) and inline images only', () => {
    expect(faviconSrc('https://example.com/favicon.ico')).toBe('https://example.com/favicon.ico');
    expect(faviconSrc('data:image/png;base64,AAAA')).toBe('data:image/png;base64,AAAA');
    expect(faviconSrc('javascript:alert(1)')).toBe('');
    expect(faviconSrc('chrome://favicon/x')).toBe('');
    expect(faviconSrc(undefined)).toBe('');
  });
});
