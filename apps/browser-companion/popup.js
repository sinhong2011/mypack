import {
  HostConnection, HostError, checkTabUrl, getSettings, setSettings, resolveLocale, createTranslator,
  applyDocumentLocale, errorKey, faviconSrc, normalizeLocale, DOCS_URL, PROTOCOL_VERSION
} from "./lib.js";

const root = document.getElementById("app");
const host = new HostConnection();
let t = (k) => k;
let tab = null;      // {url, host, title, favicon}
let lookup = null;

function el(tag, props = {}, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === "class") n.className = v;
    else if (k.startsWith("on")) n.addEventListener(k.slice(2), v);
    else if (v === true) n.setAttribute(k, "");
    else if (v !== false && v != null) n.setAttribute(k, v);
  }
  for (const c of children.flat()) if (c != null) n.append(c);
  return n;
}

function render(...nodes) {
  root.replaceChildren(...nodes);
  root.setAttribute("aria-busy", "false");
  const first = root.querySelector("[data-autofocus]");
  if (first) first.focus();
}

function siteHeader() {
  const fav = el("div", { class: "fav", "aria-hidden": "true" }, (tab.host[0] || "?").toUpperCase());
  const src = faviconSrc(tab.favicon);
  if (src) {
    const img = el("img", { src, alt: "" });
    img.addEventListener("error", () => img.remove());
    fav.replaceChildren(img);
  }
  return el("div", { class: "site" }, fav,
    el("div", { class: "site-text" },
      el("div", { class: "host" }, tab.host),
      el("div", { class: "title" }, tab.title || t("untitled"))));
}

function setBusy(btn, label) { btn.disabled = true; btn.textContent = label; }

function errorCard(title, body, extra) {
  return el("section", { class: "card error", role: "alert" }, el("h2", { class: "err" }, title), body && el("p", {}, body), extra);
}

// ---- views --------------------------------------------------------------------------

function viewRestricted() {
  render(el("section", { class: "card" }, el("h2", {}, t("restrictedTitle")), el("p", { class: "muted" }, t("restrictedBody"))));
}

function viewHostMissing() {
  const cmd = `mypack-native-host install --extension-id ${chrome.runtime.id}`;
  render(errorCard(t("hostMissingTitle"), t("hostMissingBody"),
    el("div", {},
      el("ol", {}, el("li", {}, t("hostStep1")),
        el("li", {}, t("hostStep2"), el("code", { tabindex: "0" }, cmd)),
        el("li", {}, t("hostStep3"))),
      el("p", { class: "muted" }, t("docsLink", { url: DOCS_URL })))),
    el("button", { class: "primary", "data-autofocus": true, onclick: start }, t("retry")));
}

function viewHostError() {
  render(errorCard(t("hostErrorTitle"), t("hostErrorBody")),
    el("button", { class: "primary", "data-autofocus": true, onclick: start }, t("retry")));
}

function viewProtocol() {
  render(errorCard(t("protocolTitle"), t("protocolBody")),
    el("button", { onclick: start, "data-autofocus": true }, t("retry")));
}

function viewMessage(msgKey, err) {
  const msg = el("p", { class: "muted", role: "alert" }, t(msgKey || errorKey(err)));
  const old = root.querySelector("[data-msg]");
  if (old) old.remove();
  msg.dataset.msg = "1";
  root.append(msg);
}

async function act(btn, busyLabel, fn) {
  const label = btn.textContent;
  setBusy(btn, busyLabel);
  try {
    await fn();
  } catch (err) {
    btn.disabled = false;
    btn.textContent = label;
    if (err instanceof HostError && err.code === "PROTOCOL_MISMATCH") return viewProtocol();
    if (err instanceof HostError && (err.code === "HOST_MISSING")) return viewHostMissing();
    viewMessage(null, err);
  }
}

async function createDraft(btn) {
  await act(btn, t("creating"), async () => {
    const draft = await host.request("createDraft", { url: tab.url, title: tab.title || "", faviconUrl: faviconSrc(tab.favicon).startsWith("http") ? tab.favicon : undefined });
    let opened = true;
    try { await host.request("openManager", { projectId: draft.projectId }); } catch (_) { opened = false; }
    viewDraftCreated(draft, opened);
  });
}

function viewDraftCreated(draft, opened) {
  render(siteHeader(),
    el("section", { class: "card success", role: "status" },
      el("h2", { class: "ok" }, t("draftCreated")),
      el("p", { class: "muted" }, t("draftCreatedBody")),
      !opened && el("p", {}, t("draftOpenFailed"))),
    !opened && el("button", { class: "primary", "data-autofocus": true, onclick: (e) => act(e.currentTarget, t("opening"), async () => {
      await host.request("openManager", { projectId: draft.projectId });
      viewDraftCreated(draft, true);
    }) }, t("openMypack")),
    opened && el("button", { "data-autofocus": true, onclick: (e) => act(e.currentTarget, t("opening"), async () => {
      await host.request("openManager", { projectId: draft.projectId });
      e.currentTarget.disabled = false; e.currentTarget.textContent = t("manageApp");
    }) }, t("manageApp")));
}

function stateLabel(s) {
  switch (s) {
    case "managed": return t("stateManaged");
    case "missing": return t("stateMissing");
    case "attention": return t("stateAttention");
    case "external_readonly": return t("stateExternal");
    case "draft": return t("stateDraft");
    default: return t("stateProject");
  }
}

function openMatch(btn, m) {
  return act(btn, t("opening"), async () => {
    if (m.installationId) await host.request("openManagedApp", { installationId: m.installationId });
    else await host.request("openManager", { projectId: m.projectId });
    btn.disabled = false;
    btn.textContent = t("opened");
  });
}

function manageMatch(btn, m) {
  return act(btn, t("opening"), async () => {
    await host.request("openManager", { projectId: m.projectId || undefined });
    btn.disabled = false;
    btn.textContent = t("manageApp");
  });
}

function viewNotManaged() {
  render(siteHeader(),
    el("button", { class: "primary", "data-autofocus": true, onclick: (e) => createDraft(e.currentTarget) }, t("createApp")));
}

function viewUnique(m) {
  const canOpen = !!m.installationId && m.state !== "missing";
  const nodes = [siteHeader(),
    el("section", { class: "card" }, el("div", { class: "choice-name" }, m.name),
      el("div", { class: "choice-meta" }, `${stateLabel(m.state)}${m.profileId ? " · " + t("profile", { profile: m.profileId }) : ""}`))];
  if (canOpen) {
    nodes.push(el("button", { class: "primary", "data-autofocus": true, onclick: (e) => openMatch(e.currentTarget, m) }, t("openApp")));
    nodes.push(el("button", { onclick: (e) => manageMatch(e.currentTarget, m) }, t("manageApp")));
  } else {
    nodes.push(el("button", { class: "primary", "data-autofocus": true, onclick: (e) => manageMatch(e.currentTarget, m) }, t("manageApp")));
  }
  nodes.push(el("button", { class: "link", onclick: viewNotManaged }, t("createAnother")));
  render(...nodes);
}

function viewMultiple(matches) {
  const list = el("ul", { class: "choices" }, matches.map((m, i) => el("li", {},
    el("button", { "data-autofocus": i === 0, onclick: (e) => openMatch(e.currentTarget, m),
      "aria-label": `${m.name}, ${stateLabel(m.state)}${m.profileId ? ", " + t("profile", { profile: m.profileId }) : ""}` },
      el("span", { class: "choice-name" }, m.name),
      el("span", { class: "choice-meta" }, `${stateLabel(m.state)}${m.profileId ? " · " + t("profile", { profile: m.profileId }) : ""}`)))));
  render(siteHeader(), el("h2", {}, t("chooseTitle")), el("p", { class: "muted" }, t("chooseBody")), list,
    el("button", { class: "link", onclick: viewNotManaged }, t("createAnother")));
}

// ---- startup --------------------------------------------------------------------------

async function start() {
  root.setAttribute("aria-busy", "true");
  render(el("p", { class: "muted" }, t("checking")));
  root.setAttribute("aria-busy", "true");
  host.close();
  host.dead = null;
  let hello;
  try {
    hello = await host.request("hello");
  } catch (err) {
    if (err.code === "PROTOCOL_MISMATCH") return viewProtocol();
    return err.code === "HOST_MISSING" ? viewHostMissing() : viewHostError();
  }
  if (hello.protocolVersion !== PROTOCOL_VERSION || !["lookup", "createDraft", "openManager"].every((a) => (hello.actions || []).includes(a))) {
    return viewProtocol();
  }
  // Optional "Follow mypack Desktop": remember the Desktop locale, re-render if it changes ours.
  const dl = normalizeLocale(hello.locale);
  const settings = await getSettings();
  if (dl && dl !== settings.desktopLocale) await setSettings({ desktopLocale: dl });
  if (settings.language === "desktop" && dl && dl !== t.locale) {
    t = await createTranslator(dl);
    applyDocumentLocale(dl);
  }
  try {
    lookup = await host.request("lookup", { url: tab.url, title: tab.title || "" });
  } catch (err) {
    if (err.code === "UNSUPPORTED_URL") return viewRestricted();
    if (err.code === "PROTOCOL_MISMATCH") return viewProtocol();
    return err.code === "HOST_MISSING" ? viewHostMissing() : viewHostError();
  }
  const ms = lookup.matches || [];
  if (lookup.match === "unique" && ms.length === 1) return viewUnique(ms[0]);
  if (lookup.match === "multiple" || ms.length > 1) return viewMultiple(ms);
  return viewNotManaged();
}

async function init() {
  const settings = await getSettings();
  const locale = resolveLocale(settings);
  t = await createTranslator(locale);
  applyDocumentLocale(locale);
  document.title = t("appName");
  const [active] = await chrome.tabs.query({ active: true, currentWindow: true }).catch(() => []);
  const chk = checkTabUrl(active && active.url);
  if (!chk.ok) {
    tab = { host: "", url: "", title: "" };
    return viewRestricted();
  }
  tab = { host: chk.host, url: chk.url, title: (active.title || "").slice(0, 200), favicon: active.favIconUrl };
  start();
}

window.addEventListener("pagehide", () => host.close());
init();
