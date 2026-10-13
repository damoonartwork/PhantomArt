var __defProp = Object.defineProperty;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });

// worker.js
import { connect } from "cloudflare:sockets";
var WS_OPEN = 1;
var worker_default = {
  async fetch(request, env) {
    const uuid = (env.UUID || "").trim().toLowerCase();
    if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(uuid)) {
      return new Response("Server is not configured: set the UUID variable.", { status: 500 });
    }
    if ((request.headers.get("Upgrade") || "").toLowerCase() === "websocket") {
      return handleVless(request, uuid, env);
    }
    const url = new URL(request.url);
    const links = configLinks(url.hostname, uuid, env);
    if (url.pathname === `/${uuid}/sub`) {
      return new Response(btoa(links.map((l) => l.link).join("\n")), {
        headers: { "content-type": "text/plain; charset=utf-8" }
      });
    }
    if (url.pathname === `/${uuid}`) {
      return new Response(infoPage(url.hostname, uuid, links), {
        headers: { "content-type": "text/html; charset=utf-8" }
      });
    }
    return new Response("<!doctype html><title>Welcome</title><h1>It works!</h1>", {
      headers: { "content-type": "text/html; charset=utf-8" }
    });
  }
};
function configLinks(host, uuid, env) {
  const make = /* @__PURE__ */ __name((address, name) => {
    const q = new URLSearchParams({
      encryption: "none",
      security: "tls",
      sni: host,
      fp: "chrome",
      type: "ws",
      host,
      path: "/?ed=2048"
    });
    return { name, link: `vless://${uuid}@${address}:443?${q}#${encodeURIComponent(name)}` };
  }, "make");
  const out = [make(host, "PhantomArt")];
  for (const ip of (env.CLEAN_IPS || "").split(",").map((s) => s.trim()).filter(Boolean)) {
    out.push(make(ip.includes(":") && !ip.startsWith("[") ? `[${ip}]` : ip, `PhantomArt ${ip}`));
  }
  return out;
}
__name(configLinks, "configLinks");
function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}
__name(esc, "esc");
function infoPage(host, uuid, links) {
  const sub = `https://${host}/${uuid}/sub`;
  const row = /* @__PURE__ */ __name((title, value) => `
    <div class="row"><div class="t">${esc(title)}</div>
    <textarea readonly dir="ltr">${esc(value)}</textarea>
    <button onclick="navigator.clipboard.writeText(this.previousElementSibling.value);this.textContent='\u06A9\u067E\u06CC \u0634\u062F \u2713'">\u06A9\u067E\u06CC</button></div>`, "row");
  return `<!doctype html><html lang="fa" dir="rtl"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>PhantomArt</title>
<style>
body{font-family:Tahoma,sans-serif;background:#0f111a;color:#e6e8f0;max-width:760px;margin:auto;padding:16px}
.row{background:#171a26;border:1px solid #2a2f44;border-radius:10px;padding:12px;margin:12px 0}
.t{font-weight:bold;margin-bottom:6px}
textarea{width:100%;box-sizing:border-box;height:84px;background:#1f2333;color:#e6e8f0;border:1px solid #2a2f44;border-radius:8px;padding:8px;font-size:12px}
button{margin-top:8px;background:#7c5cff;color:#fff;border:0;border-radius:8px;padding:8px 16px;cursor:pointer}
p{color:#8a90a8;line-height:1.9}
</style></head><body>
<h2>\u0633\u0631\u0648\u0631 PhantomArt \u0634\u0645\u0627 \u0622\u0645\u0627\u062F\u0647 \u0627\u0633\u062A</h2>
<p>\u06CC\u06A9\u06CC \u0627\u0632 \u0644\u06CC\u0646\u06A9\u200C\u0647\u0627 \u0631\u0627 \u06A9\u067E\u06CC \u06A9\u0646\u06CC\u062F \u0648 \u062F\u0631 \u0628\u0631\u0646\u0627\u0645\u0647 \xAB\u0627\u0641\u0632\u0648\u062F\u0646 \u0627\u0632 \u06A9\u0644\u06CC\u067E\u200C\u0628\u0648\u0631\u062F\xBB \u0631\u0627 \u0628\u0632\u0646\u06CC\u062F.
\u06CC\u0627 \u0644\u06CC\u0646\u06A9 \u0633\u0627\u0628\u0633\u06A9\u0631\u06CC\u067E\u0634\u0646 \u0631\u0627 \u062F\u0631 \u062A\u0628 \xAB\u0633\u0627\u0628\u0633\u06A9\u0631\u06CC\u067E\u0634\u0646\xBB \u0648\u0627\u0631\u062F \u06A9\u0646\u06CC\u062F. \u0627\u06CC\u0646 \u0635\u0641\u062D\u0647 \u0631\u0627 \u0628\u0627 \u06A9\u0633\u06CC \u0628\u0647 \u0627\u0634\u062A\u0631\u0627\u06A9 \u0646\u06AF\u0630\u0627\u0631\u06CC\u062F.</p>
${row("\u0644\u06CC\u0646\u06A9 \u0633\u0627\u0628\u0633\u06A9\u0631\u06CC\u067E\u0634\u0646", sub)}
${links.map((l) => row(l.name, l.link)).join("")}
</body></html>`;
}
__name(infoPage, "infoPage");
function handleVless(request, uuid, env) {
  const [client, ws] = Object.values(new WebSocketPair());
  ws.accept();
  let remote = null;
  let udpWrite = null;
  const writeRemote = /* @__PURE__ */ __name(async (data) => {
    const w = remote.writable.getWriter();
    try {
      await w.write(data);
    } finally {
      w.releaseLock();
    }
  }, "writeRemote");
  websocketStream(ws, request.headers.get("sec-websocket-protocol") || "").pipeTo(
    new WritableStream({
      async write(chunk) {
        if (udpWrite)
          return udpWrite(chunk);
        if (remote)
          return writeRemote(chunk);
        const h = parseVlessHeader(chunk, uuid);
        const respHeader = new Uint8Array([h.version, 0]);
        const payload = chunk.subarray(h.payloadIndex);
        if (h.udp) {
          if (h.port !== 53)
            throw new Error("only DNS is supported over UDP");
          udpWrite = dnsOverHttps(ws, respHeader);
          return udpWrite(payload);
        }
        const open = /* @__PURE__ */ __name(async (host, port) => {
          remote = connect({ hostname: host, port });
          if (payload.length)
            await writeRemote(payload);
          return remote;
        }, "open");
        const retry = env.PROXYIP ? () => {
          const [ph, pp] = splitHostPort(env.PROXYIP.trim(), h.port);
          return open(ph, pp);
        } : null;
        let sock;
        try {
          sock = await open(h.address, h.port);
        } catch (e) {
          if (!retry)
            throw e;
          sock = await retry();
          pipeRemoteToWs(sock, ws, respHeader, null);
          return;
        }
        pipeRemoteToWs(sock, ws, respHeader, retry);
      }
    })
  ).catch(() => safeClose(ws));
  return new Response(null, { status: 101, webSocket: client });
}
__name(handleVless, "handleVless");
async function pipeRemoteToWs(sock, ws, header, retry) {
  let gotData = false;
  await sock.readable.pipeTo(
    new WritableStream({
      write(chunk) {
        if (ws.readyState !== WS_OPEN)
          throw new Error("websocket closed");
        gotData = true;
        if (header) {
          ws.send(concat(header, chunk));
          header = null;
        } else {
          ws.send(chunk);
        }
      }
    })
  ).catch(() => {
  });
  if (!gotData && retry && ws.readyState === WS_OPEN) {
    try {
      return pipeRemoteToWs(await retry(), ws, header, null);
    } catch {
    }
  }
  safeClose(ws);
}
__name(pipeRemoteToWs, "pipeRemoteToWs");
function parseVlessHeader(buf, uuid) {
  const b = new Uint8Array(buf);
  if (b.length < 24)
    throw new Error("invalid header");
  const version = b[0];
  const id = [...b.subarray(1, 17)].map((x) => x.toString(16).padStart(2, "0")).join("");
  if (id !== uuid.replaceAll("-", ""))
    throw new Error("invalid user");
  let i = 18 + b[17];
  const cmd = b[i++];
  if (cmd !== 1 && cmd !== 2)
    throw new Error(`unsupported command ${cmd}`);
  const port = b[i] << 8 | b[i + 1];
  i += 2;
  const atype = b[i++];
  let address;
  if (atype === 1) {
    address = [...b.subarray(i, i + 4)].join(".");
    i += 4;
  } else if (atype === 2) {
    const len = b[i++];
    address = new TextDecoder().decode(b.subarray(i, i + len));
    i += len;
  } else if (atype === 3) {
    const parts = [];
    for (let k = 0; k < 8; k++)
      parts.push((b[i + 2 * k] << 8 | b[i + 2 * k + 1]).toString(16));
    address = parts.join(":");
    i += 16;
  } else {
    throw new Error(`invalid address type ${atype}`);
  }
  if (!address)
    throw new Error("empty address");
  return { version, udp: cmd === 2, port, address, payloadIndex: i };
}
__name(parseVlessHeader, "parseVlessHeader");
function dnsOverHttps(ws, header) {
  let pending = new Uint8Array(0);
  return async (chunk) => {
    pending = concat(pending, chunk);
    while (pending.length >= 2) {
      const len = pending[0] << 8 | pending[1];
      if (pending.length < 2 + len)
        break;
      const query = pending.slice(2, 2 + len);
      pending = pending.slice(2 + len);
      const resp = await fetch("https://cloudflare-dns.com/dns-query", {
        method: "POST",
        headers: { "content-type": "application/dns-message" },
        body: query
      });
      const answer = new Uint8Array(await resp.arrayBuffer());
      const framed = concat(new Uint8Array([answer.length >> 8, answer.length & 255]), answer);
      if (ws.readyState !== WS_OPEN)
        return;
      ws.send(header ? concat(header, framed) : framed);
      header = null;
    }
  };
}
__name(dnsOverHttps, "dnsOverHttps");
function websocketStream(ws, earlyDataHeader) {
  let cancelled = false;
  return new ReadableStream({
    start(controller) {
      ws.addEventListener("message", (e) => {
        if (cancelled)
          return;
        const d = e.data;
        controller.enqueue(typeof d === "string" ? new TextEncoder().encode(d) : new Uint8Array(d));
      });
      ws.addEventListener("close", () => {
        safeClose(ws);
        if (!cancelled) {
          try {
            controller.close();
          } catch {
          }
        }
      });
      ws.addEventListener("error", (e) => controller.error(e));
      const early = base64UrlDecode(earlyDataHeader);
      if (early)
        controller.enqueue(early);
    },
    cancel() {
      cancelled = true;
      safeClose(ws);
    }
  });
}
__name(websocketStream, "websocketStream");
function base64UrlDecode(s) {
  if (!s)
    return null;
  try {
    const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
    return Uint8Array.from(bin, (c) => c.charCodeAt(0));
  } catch {
    return null;
  }
}
__name(base64UrlDecode, "base64UrlDecode");
function splitHostPort(s, defPort) {
  const m = s.match(/^\[(.+)\](?::(\d+))?$/) || s.match(/^([^:]+)(?::(\d+))?$/);
  if (!m)
    return [s, defPort];
  return [m[1], m[2] ? Number(m[2]) : defPort];
}
__name(splitHostPort, "splitHostPort");
function concat(...parts) {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.byteLength, 0));
  let off = 0;
  for (const p of parts) {
    out.set(new Uint8Array(p), off);
    off += p.byteLength;
  }
  return out;
}
__name(concat, "concat");
function safeClose(ws) {
  try {
    if (ws.readyState === WS_OPEN || ws.readyState === 2)
      ws.close();
  } catch {
  }
}
__name(safeClose, "safeClose");

// ../../../../../tmp/claude-0/-home-user-PhantomArt/1a80b194-02ff-5b4e-b20d-0aa2018e4f63/scratchpad/wr/node_modules/wrangler/templates/middleware/middleware-ensure-req-body-drained.ts
var drainBody = /* @__PURE__ */ __name(async (request, env, _ctx, middlewareCtx) => {
  try {
    return await middlewareCtx.next(request, env);
  } finally {
    try {
      if (request.body !== null && !request.bodyUsed) {
        const reader = request.body.getReader();
        while (!(await reader.read()).done) {
        }
      }
    } catch (e) {
      console.error("Failed to drain the unused request body.", e);
    }
  }
}, "drainBody");
var middleware_ensure_req_body_drained_default = drainBody;

// ../../../../../tmp/claude-0/-home-user-PhantomArt/1a80b194-02ff-5b4e-b20d-0aa2018e4f63/scratchpad/wr/node_modules/wrangler/templates/middleware/middleware-miniflare3-json-error.ts
function reduceError(e) {
  return {
    name: e?.name,
    message: e?.message ?? String(e),
    stack: e?.stack,
    cause: e?.cause === void 0 ? void 0 : reduceError(e.cause)
  };
}
__name(reduceError, "reduceError");
var jsonError = /* @__PURE__ */ __name(async (request, env, _ctx, middlewareCtx) => {
  try {
    return await middlewareCtx.next(request, env);
  } catch (e) {
    const error = reduceError(e);
    return Response.json(error, {
      status: 500,
      headers: { "MF-Experimental-Error-Stack": "true" }
    });
  }
}, "jsonError");
var middleware_miniflare3_json_error_default = jsonError;

// .wrangler/tmp/bundle-iZPr6s/middleware-insertion-facade.js
var __INTERNAL_WRANGLER_MIDDLEWARE__ = [
  middleware_ensure_req_body_drained_default,
  middleware_miniflare3_json_error_default
];
var middleware_insertion_facade_default = worker_default;

// ../../../../../tmp/claude-0/-home-user-PhantomArt/1a80b194-02ff-5b4e-b20d-0aa2018e4f63/scratchpad/wr/node_modules/wrangler/templates/middleware/common.ts
var __facade_middleware__ = [];
function __facade_register__(...args) {
  __facade_middleware__.push(...args.flat());
}
__name(__facade_register__, "__facade_register__");
function __facade_invokeChain__(request, env, ctx, dispatch, middlewareChain) {
  const [head, ...tail] = middlewareChain;
  const middlewareCtx = {
    dispatch,
    next(newRequest, newEnv) {
      return __facade_invokeChain__(newRequest, newEnv, ctx, dispatch, tail);
    }
  };
  return head(request, env, ctx, middlewareCtx);
}
__name(__facade_invokeChain__, "__facade_invokeChain__");
function __facade_invoke__(request, env, ctx, dispatch, finalMiddleware) {
  return __facade_invokeChain__(request, env, ctx, dispatch, [
    ...__facade_middleware__,
    finalMiddleware
  ]);
}
__name(__facade_invoke__, "__facade_invoke__");

// .wrangler/tmp/bundle-iZPr6s/middleware-loader.entry.ts
var __Facade_ScheduledController__ = class {
  constructor(scheduledTime, cron, noRetry) {
    this.scheduledTime = scheduledTime;
    this.cron = cron;
    this.#noRetry = noRetry;
  }
  #noRetry;
  noRetry() {
    if (!(this instanceof __Facade_ScheduledController__)) {
      throw new TypeError("Illegal invocation");
    }
    this.#noRetry();
  }
};
__name(__Facade_ScheduledController__, "__Facade_ScheduledController__");
function wrapExportedHandler(worker) {
  if (__INTERNAL_WRANGLER_MIDDLEWARE__ === void 0 || __INTERNAL_WRANGLER_MIDDLEWARE__.length === 0) {
    return worker;
  }
  for (const middleware of __INTERNAL_WRANGLER_MIDDLEWARE__) {
    __facade_register__(middleware);
  }
  const fetchDispatcher = /* @__PURE__ */ __name(function(request, env, ctx) {
    if (worker.fetch === void 0) {
      throw new Error("Handler does not export a fetch() function.");
    }
    return worker.fetch(request, env, ctx);
  }, "fetchDispatcher");
  return {
    ...worker,
    fetch(request, env, ctx) {
      const dispatcher = /* @__PURE__ */ __name(function(type, init) {
        if (type === "scheduled" && worker.scheduled !== void 0) {
          const controller = new __Facade_ScheduledController__(
            Date.now(),
            init.cron ?? "",
            () => {
            }
          );
          return worker.scheduled(controller, env, ctx);
        }
      }, "dispatcher");
      return __facade_invoke__(request, env, ctx, dispatcher, fetchDispatcher);
    }
  };
}
__name(wrapExportedHandler, "wrapExportedHandler");
function wrapWorkerEntrypoint(klass) {
  if (__INTERNAL_WRANGLER_MIDDLEWARE__ === void 0 || __INTERNAL_WRANGLER_MIDDLEWARE__.length === 0) {
    return klass;
  }
  for (const middleware of __INTERNAL_WRANGLER_MIDDLEWARE__) {
    __facade_register__(middleware);
  }
  return class extends klass {
    #fetchDispatcher = (request, env, ctx) => {
      this.env = env;
      this.ctx = ctx;
      if (super.fetch === void 0) {
        throw new Error("Entrypoint class does not define a fetch() function.");
      }
      return super.fetch(request);
    };
    #dispatcher = (type, init) => {
      if (type === "scheduled" && super.scheduled !== void 0) {
        const controller = new __Facade_ScheduledController__(
          Date.now(),
          init.cron ?? "",
          () => {
          }
        );
        return super.scheduled(controller);
      }
    };
    fetch(request) {
      return __facade_invoke__(
        request,
        this.env,
        this.ctx,
        this.#dispatcher,
        this.#fetchDispatcher
      );
    }
  };
}
__name(wrapWorkerEntrypoint, "wrapWorkerEntrypoint");
var WRAPPED_ENTRY;
if (typeof middleware_insertion_facade_default === "object") {
  WRAPPED_ENTRY = wrapExportedHandler(middleware_insertion_facade_default);
} else if (typeof middleware_insertion_facade_default === "function") {
  WRAPPED_ENTRY = wrapWorkerEntrypoint(middleware_insertion_facade_default);
}
var middleware_loader_entry_default = WRAPPED_ENTRY;
export {
  __INTERNAL_WRANGLER_MIDDLEWARE__,
  middleware_loader_entry_default as default
};
//# sourceMappingURL=worker.js.map
