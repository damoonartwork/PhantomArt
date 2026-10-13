"use strict";

const $ = (sel) => document.querySelector(sel);
const api = () => window.go.main.App;

let state = null;
let pings = {};
let busy = false;

function toast(msg, isError = false) {
  const el = $("#toast");
  el.textContent = msg;
  el.classList.toggle("error", isError);
  el.classList.remove("hidden");
  clearTimeout(toast.t);
  toast.t = setTimeout(() => el.classList.add("hidden"), 4000);
}

function errText(e) {
  return typeof e === "string" ? e : (e && e.message) || String(e);
}

async function run(fn, okMsg) {
  try {
    const r = await fn();
    if (okMsg) toast(typeof okMsg === "function" ? okMsg(r) : okMsg);
    await refresh();
    return r;
  } catch (e) {
    toast(errText(e), true);
    await refresh();
  }
}

async function refresh() {
  state = await api().GetState();
  render();
}

function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") e.className = v;
    else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v);
  }
  for (const c of children) if (c != null) e.append(c);
  return e;
}

function pingEl(ms) {
  if (ms === undefined) return el("span", { class: "ping" }, "");
  if (ms < 0) return el("span", { class: "ping bad" }, "timeout");
  const cls = ms < 300 ? "good" : ms < 800 ? "mid" : "bad";
  return el("span", { class: "ping " + cls }, ms + " ms");
}

function render() {
  const s = state;

  // Connection panel
  const btn = $("#connectBtn");
  btn.classList.toggle("on", s.connected);
  btn.classList.toggle("busy", busy);
  const st = $("#status");
  st.textContent = busy ? "در حال اتصال…" : s.connected ? "متصل" : "قطع";
  st.className = "status " + (s.connected ? "on" : "off");
  const active = s.profiles.find((p) => p.id === s.activeId);
  $("#activeName").textContent = active ? active.name : "";
  $("#testBtn").disabled = !s.connected;

  // Core warning
  $("#coreWarning").classList.toggle("hidden", s.coreFound);
  $("#corePath").textContent = s.corePath;

  // Servers
  const list = $("#profiles");
  list.replaceChildren();
  for (const p of s.profiles) {
    const cls = ["item"];
    if (p.id === s.selectedId) cls.push("selected");
    if (p.id === s.activeId) cls.push("active");
    const transport = p.network + (p.security && p.security !== "none" ? " · " + p.security : "");
    list.append(
      el("li", { class: cls.join(" "), ondblclick: () => connectTo(p.id), onclick: () => select(p.id) },
        el("span", { class: "dot" }),
        el("div", { class: "info" },
          el("div", { class: "name" }, p.name),
          el("div", { class: "meta" }, `${p.address}:${p.port}`)),
        el("span", { class: "tag" }, p.protocol),
        el("span", { class: "tag" }, transport),
        pingEl(pings[p.id]),
        el("button", {
          class: "danger", title: "حذف",
          onclick: (ev) => { ev.stopPropagation(); run(() => api().DeleteProfile(p.id)); },
        }, "✕")));
  }
  $("#emptyServers").classList.toggle("hidden", s.profiles.length > 0);

  // Subscriptions
  const subs = $("#subs");
  subs.replaceChildren();
  for (const sub of s.subscriptions) {
    const count = s.profiles.filter((p) => p.subscription === sub.url).length;
    subs.append(
      el("li", { class: "item" },
        el("div", { class: "info" },
          el("div", { class: "name" }, sub.name || "سابسکریپشن"),
          el("div", { class: "meta" }, sub.url)),
        el("span", { class: "tag" }, count + " servers"),
        el("button", {
          class: "danger", title: "حذف",
          onclick: () => run(() => api().RemoveSubscription(sub.url)),
        }, "✕")));
  }

  // Settings (only fill when not being edited)
  const form = $("#settingsForm");
  if (!form.contains(document.activeElement)) {
    const set = s.settings;
    for (const k of ["systemProxy", "bypassIran", "blockAds", "fragment", "allowLan"]) form.elements[k].checked = !!set[k];
    form.elements.systemProxy.disabled = !s.sysProxySupported;
    form.elements.socksPort.value = set.socksPort;
    form.elements.httpPort.value = set.httpPort;
    form.elements.logLevel.value = set.logLevel || "warning";
  }
}

async function select(id) {
  if (id === state.selectedId) return;
  await run(() => api().SelectProfile(id));
}

async function connectTo(id) {
  await run(() => api().SelectProfile(id));
  if (!state.connected) await toggle();
}

async function toggle() {
  if (busy) return;
  busy = true;
  render();
  try {
    if (state.connected) await api().Disconnect();
    else await api().Connect();
  } catch (e) {
    toast(errText(e), true);
  } finally {
    busy = false;
    $("#testResult").textContent = "";
    await refresh();
  }
}

function appendLog(line) {
  const pre = $("#logs");
  pre.textContent += line + "\n";
  const lines = pre.textContent.split("\n");
  if (lines.length > 600) pre.textContent = lines.slice(-500).join("\n");
  pre.scrollTop = pre.scrollHeight;
}

function bind() {
  $("#connectBtn").addEventListener("click", toggle);

  $("#testBtn").addEventListener("click", async () => {
    const out = $("#testResult");
    out.textContent = "در حال تست…";
    try {
      const ms = await api().TestConnection();
      out.textContent = `✓ اتصال برقرار است (${ms} ms)`;
    } catch (e) {
      out.textContent = "✗ " + errText(e);
    }
  });

  document.querySelectorAll(".tab").forEach((t) =>
    t.addEventListener("click", () => {
      document.querySelectorAll(".tab").forEach((x) => x.classList.toggle("active", x === t));
      document.querySelectorAll(".panel").forEach((p) => p.classList.toggle("hidden", p.id !== "tab-" + t.dataset.tab));
    }));

  $("#pasteBtn").addEventListener("click", () =>
    run(() => api().ImportClipboard(), (n) => `${n} سرور اضافه شد`));

  $("#manualBtn").addEventListener("click", () => $("#manualBox").classList.toggle("hidden"));
  $("#manualAdd").addEventListener("click", async () => {
    const text = $("#manualText").value;
    const n = await run(() => api().ImportText(text), (n) => `${n} سرور اضافه شد`);
    if (n) { $("#manualText").value = ""; $("#manualBox").classList.add("hidden"); }
  });

  $("#pingBtn").addEventListener("click", async () => {
    const b = $("#pingBtn");
    b.disabled = true;
    try { pings = await api().PingAll(); render(); } finally { b.disabled = false; }
  });

  $("#subAdd").addEventListener("click", async () => {
    const n = await run(() => api().AddSubscription($("#subName").value, $("#subUrl").value),
      (n) => `${n} سرور دریافت شد`);
    if (n !== undefined) { $("#subName").value = ""; $("#subUrl").value = ""; }
  });
  $("#subUpdate").addEventListener("click", () =>
    run(() => api().UpdateSubscriptions(), (n) => `${n} سرور دریافت شد`));

  $("#settingsForm").addEventListener("submit", (ev) => {
    ev.preventDefault();
    const f = ev.target.elements;
    const settings = {
      socksPort: Number(f.socksPort.value),
      httpPort: Number(f.httpPort.value),
      systemProxy: f.systemProxy.checked,
      allowLan: f.allowLan.checked,
      bypassIran: f.bypassIran.checked,
      blockAds: f.blockAds.checked,
      fragment: f.fragment.checked,
      logLevel: f.logLevel.value,
    };
    document.activeElement.blur();
    run(() => api().UpdateSettings(settings), "تنظیمات ذخیره شد");
  });

  $("#openCoreBtn").addEventListener("click", () => api().OpenCoreFolder());

  window.runtime.EventsOn("log", appendLog);
  window.runtime.EventsOn("state", refresh);
}

window.addEventListener("DOMContentLoaded", async () => {
  bind();
  (await api().GetLogs()).forEach(appendLog);
  await refresh();
});
