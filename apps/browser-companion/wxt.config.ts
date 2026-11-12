import { defineConfig } from 'wxt';

// One source, two targets: Chrome and Edge (MV3 service worker). The version comes from package.json.
// Manifest strings (name, description, toolbar tooltip, context-menu default): `locales/*.json` → `_locales/*/messages.json`
// via `@wxt-dev/i18n`. The popup, options page and context menu use the seven catalogs in `i18n/` instead, because the
// language can be overridden in options or follow mypack Desktop (see lib/locale.ts and docs/companion.md).
export default defineConfig({
  modules: ['@wxt-dev/module-solid', '@wxt-dev/i18n/module'],
  manifestVersion: 3,
  manifest: {
    default_locale: 'en',
    name: '__MSG_appName__',
    description: '__MSG_appDescription__',
    minimum_chrome_version: '110',
    icons: { 16: 'icons/icon16.png', 32: 'icons/icon32.png', 48: 'icons/icon48.png', 128: 'icons/icon128.png' },
    action: {
      default_title: '__MSG_actionTitle__',
      default_icon: { 16: 'icons/icon16.png', 32: 'icons/icon32.png', 48: 'icons/icon48.png' },
    },
    // Only what the popup needs: the clicked tab (activeTab), the local helper, the language setting and the menu.
    // No host permissions and no `tabs`.
    permissions: ['activeTab', 'nativeMessaging', 'storage', 'contextMenus'],
    content_security_policy: { extension_pages: "script-src 'self'; object-src 'self'" },
  },
  hooks: {
    // WXT takes the toolbar tooltip from the popup's <title>; keep it localized instead.
    'build:manifestGenerated': (_wxt, manifest) => {
      manifest.action = { ...manifest.action, default_title: '__MSG_actionTitle__' };
    },
  },
});
