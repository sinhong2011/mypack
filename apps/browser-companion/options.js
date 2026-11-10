import {
  LOCALES, LOCALE_NAMES, HostConnection, getSettings, setSettings, resolveLocale, createTranslator,
  applyDocumentLocale, normalizeLocale
} from "./lib.js";

const $ = (id) => document.getElementById(id);
let t;

async function paint() {
  const settings = await getSettings();
  const locale = resolveLocale(settings);
  t = await createTranslator(locale);
  applyDocumentLocale(locale);
  document.title = t("optionsTitle");
  $("title").textContent = t("optionsTitle");
  $("langLabel").textContent = t("languageLabel");
  $("langHelp").textContent = t("languageHelp");
  $("connLabel").textContent = t("connectionLabel");
  $("check").textContent = t("checkConnection");
  $("privTitle").textContent = t("privacyTitle");
  $("privBody").textContent = t("privacyBody");

  const sel = $("lang");
  sel.replaceChildren();
  const add = (value, label) => { const o = document.createElement("option"); o.value = value; o.textContent = label; sel.append(o); };
  add("browser", t("langFollowBrowser"));
  add("desktop", t("langFollowDesktop"));
  for (const l of LOCALES) { add(l, LOCALE_NAMES[l]); sel.lastChild.lang = l; }
  sel.value = settings.language;
  $("desktopLang").textContent = settings.desktopLocale
    ? t("desktopLanguageNow", { locale: LOCALE_NAMES[normalizeLocale(settings.desktopLocale)] || settings.desktopLocale })
    : t("desktopLanguageUnknown");
}

$("lang").addEventListener("change", async (e) => {
  await setSettings({ language: e.target.value });
  await paint();
  $("saved").textContent = t("saved");
  setTimeout(() => { $("saved").textContent = ""; }, 2500);
});

$("check").addEventListener("click", async () => {
  const host = new HostConnection(5000);
  $("connStatus").textContent = "…";
  try {
    const hello = await host.request("hello");
    $("connStatus").textContent = t("connectionOk", { version: hello.hostVersion });
    const dl = normalizeLocale(hello.locale);
    if (dl) { await setSettings({ desktopLocale: dl }); await paint(); }
  } catch (_) {
    $("connStatus").textContent = t("connectionFail");
  } finally { host.close(); }
});

paint();
