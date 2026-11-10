// Service worker: context menu "Create Desktop App from this page".
// Creates a draft only (never builds), then asks Desktop to show it.
import { HostConnection, checkTabUrl, getSettings, resolveLocale, createTranslator } from "./lib.js";

const MENU_ID = "mypack-create-app";

async function buildMenu() {
  const t = await createTranslator(resolveLocale(await getSettings()));
  await chrome.contextMenus.removeAll();
  chrome.contextMenus.create({ id: MENU_ID, title: t("contextMenuTitle"), contexts: ["page"], documentUrlPatterns: ["http://*/*", "https://*/*"] });
}

chrome.runtime.onInstalled.addListener(() => { buildMenu(); });
chrome.runtime.onStartup.addListener(() => { buildMenu(); });
chrome.storage.onChanged.addListener((changes) => {
  if (changes.language || changes.desktopLocale) buildMenu();
});

async function badge(tabId, text, color) {
  try {
    await chrome.action.setBadgeBackgroundColor({ color, tabId });
    await chrome.action.setBadgeText({ text, tabId });
    setTimeout(() => chrome.action.setBadgeText({ text: "", tabId }).catch(() => {}), 4000);
  } catch (_) { /* tab may be gone */ }
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId !== MENU_ID || !tab) return;
  const chk = checkTabUrl(tab.url || info.pageUrl);
  if (!chk.ok) return badge(tab.id, "!", "#C0392B");
  const host = new HostConnection();
  try {
    await host.request("hello");
    const draft = await host.request("createDraft", { url: chk.url, title: tab.title || "" });
    await host.request("openManager", { projectId: draft.projectId }).catch(() => {});
    await badge(tab.id, "✓", "#1F70F0");
  } catch (_) {
    await badge(tab.id, "!", "#C0392B");
  } finally {
    host.close();
  }
});
