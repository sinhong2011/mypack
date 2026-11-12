// Service worker: context menu "Create Desktop App from this page".
// Creates a draft only (never builds), then asks Desktop to show it.
import { browser } from 'wxt/browser';
import { HostConnection } from '../lib/host';
import { createTranslator } from '../lib/i18n';
import { getSettings, resolveLocale } from '../lib/locale';
import { checkTabUrl } from '../lib/url';

const MENU_ID = 'mypack-create-app';

export default defineBackground(() => {
  browser.runtime.onInstalled.addListener(() => void buildMenu());
  browser.runtime.onStartup.addListener(() => void buildMenu());
  browser.storage.onChanged.addListener((changes) => {
    if (changes.language || changes.desktopLocale) void buildMenu();
  });

  browser.contextMenus.onClicked.addListener(async (info, tab) => {
    if (info.menuItemId !== MENU_ID || !tab || tab.id == null) return;
    const chk = checkTabUrl(tab.url || info.pageUrl);
    if (!chk.ok) return badge(tab.id, '!', '#C0392B');
    const host = new HostConnection();
    try {
      await host.request('hello');
      const draft = await host.request('createDraft', { url: chk.url, title: tab.title || '' });
      await host.request('openManager', { projectId: draft.projectId }).catch(() => {});
      await badge(tab.id, '✓', '#1F70F0');
    } catch {
      await badge(tab.id, '!', '#C0392B');
    } finally {
      host.close();
    }
  });
});

async function buildMenu() {
  const t = createTranslator(resolveLocale(await getSettings()));
  await browser.contextMenus.removeAll();
  browser.contextMenus.create({
    id: MENU_ID,
    title: t('contextMenuTitle'),
    contexts: ['page'],
    documentUrlPatterns: ['http://*/*', 'https://*/*'],
  });
}

async function badge(tabId: number, text: string, color: string) {
  try {
    await browser.action.setBadgeBackgroundColor({ color, tabId });
    await browser.action.setBadgeText({ text, tabId });
    setTimeout(() => browser.action.setBadgeText({ text: '', tabId }).catch(() => {}), 4000);
  } catch {
    /* tab may be gone */
  }
}
