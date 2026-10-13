// PhantomArt server for Cloudflare Workers: VLESS over WebSocket (+TLS by Cloudflare).
//
// Settings (Worker → Settings → Variables and Secrets):
//   UUID      required. Your secret key, e.g. from https://www.uuidgenerator.net
//   PROXYIP   optional. host[:port] used as a relay when the target site is itself
//             behind Cloudflare (Workers cannot open sockets to Cloudflare IPs).
//   CLEAN_IPS optional. Comma-separated Cloudflare IPs/domains that are reachable
//             from your network; an extra config is generated for each one.
//
// Open https://<your-worker-domain>/<UUID> to get your config links.

import { connect } from "cloudflare:sockets";

const WS_OPEN = 1;

export default {
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
        headers: { "content-type": "text/plain; charset=utf-8" },
      });
    }
    if (url.pathname === `/${uuid}`) {
      return new Response(infoPage(url.hostname, uuid, links), {
        headers: { "content-type": "text/html; charset=utf-8" },
      });
    }
    // Anyone without the UUID just sees an ordinary page.
    return new Response("<!doctype html><title>Welcome</title><h1>It works!</h1>", {
      headers: { "content-type": "text/html; charset=utf-8" },
    });
  },
};

// ---------------------------------------------------------------- config links

function configLinks(host, uuid, env) {
  const make = (address, name) => {
    const q = new URLSearchParams({
      encryption: "none",
      security: "tls",
      sni: host,
      fp: "chrome",
      type: "ws",
      host,
      path: "/?ed=2048",
    });
    return { name, link: `vless://${uuid}@${address}:443?${q}#${encodeURIComponent(name)}` };
  };
  const out = [make(host, "PhantomArt")];
  for (const ip of (env.CLEAN_IPS || "").split(",").map((s) => s.trim()).filter(Boolean)) {
    out.push(make(ip.includes(":") && !ip.startsWith("[") ? `[${ip}]` : ip, `PhantomArt ${ip}`));
  }
  return out;
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

function infoPage(host, uuid, links) {
  const sub = `https://${host}/${uuid}/sub`;
  const row = (title, value) => `
    <div class="row"><div class="t">${esc(title)}</div>
    <textarea readonly dir="ltr">${esc(value)}</textarea>
    <button onclick="navigator.clipboard.writeText(this.previousElementSibling.value);this.textContent='کپی شد ✓'">کپی</button></div>`;
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
<h2>سرور PhantomArt شما آماده است</h2>
<p>یکی از لینک‌ها را کپی کنید و در برنامه «افزودن از کلیپ‌بورد» را بزنید.
یا لینک سابسکریپشن را در تب «سابسکریپشن» وارد کنید. این صفحه را با کسی به اشتراک نگذارید.</p>
${row("لینک سابسکریپشن", sub)}
${links.map((l) => row(l.name, l.link)).join("")}
</body></html>`;
}

// ---------------------------------------------------------------- VLESS

function handleVless(request, uuid, env) {
  const [client, ws] = Object.values(new WebSocketPair());
  ws.accept();

  let remote = null; // current outbound TCP socket
  let udpWrite = null; // set for DNS-over-UDP sessions

  const writeRemote = async (data) => {
    const w = remote.writable.getWriter();
    try {
      await w.write(data);
    } finally {
      w.releaseLock();
    }
  };

  websocketStream(ws, request.headers.get("sec-websocket-protocol") || "")
    .pipeTo(
      new WritableStream({
        async write(chunk) {
          if (udpWrite) return udpWrite(chunk);
          if (remote) return writeRemote(chunk);

          const h = parseVlessHeader(chunk, uuid);
          const respHeader = new Uint8Array([h.version, 0]);
          const payload = chunk.subarray(h.payloadIndex);

          if (h.udp) {
            if (h.port !== 53) throw new Error("only DNS is supported over UDP");
            udpWrite = dnsOverHttps(ws, respHeader);
            return udpWrite(payload);
          }

          const open = async (host, port) => {
            remote = connect({ hostname: host, port });
            if (payload.length) await writeRemote(payload);
            return remote;
          };
          // Workers cannot reach Cloudflare's own IPs; if the target never
          // answers, retry once through PROXYIP (when configured).
          const retry = env.PROXYIP
            ? () => {
                const [ph, pp] = splitHostPort(env.PROXYIP.trim(), h.port);
                return open(ph, pp);
              }
            : null;
          let sock;
          try {
            sock = await open(h.address, h.port);
          } catch (e) {
            if (!retry) throw e;
            sock = await retry();
            pipeRemoteToWs(sock, ws, respHeader, null);
            return;
          }
          pipeRemoteToWs(sock, ws, respHeader, retry);
        },
      }),
    )
    .catch(() => safeClose(ws));

  return new Response(null, { status: 101, webSocket: client });
}

async function pipeRemoteToWs(sock, ws, header, retry) {
  let gotData = false;
  await sock.readable
    .pipeTo(
      new WritableStream({
        write(chunk) {
          if (ws.readyState !== WS_OPEN) throw new Error("websocket closed");
          gotData = true;
          if (header) {
            ws.send(concat(header, chunk));
            header = null;
          } else {
            ws.send(chunk);
          }
        },
      }),
    )
    .catch(() => {});
  if (!gotData && retry && ws.readyState === WS_OPEN) {
    try {
      return pipeRemoteToWs(await retry(), ws, header, null);
    } catch {
      // fall through and close
    }
  }
  safeClose(ws);
}

function parseVlessHeader(buf, uuid) {
  const b = new Uint8Array(buf);
  if (b.length < 24) throw new Error("invalid header");
  const version = b[0];
  const id = [...b.subarray(1, 17)].map((x) => x.toString(16).padStart(2, "0")).join("");
  if (id !== uuid.replaceAll("-", "")) throw new Error("invalid user");

  let i = 18 + b[17]; // skip addons
  const cmd = b[i++];
  if (cmd !== 1 && cmd !== 2) throw new Error(`unsupported command ${cmd}`);
  const port = (b[i] << 8) | b[i + 1];
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
    for (let k = 0; k < 8; k++) parts.push(((b[i + 2 * k] << 8) | b[i + 2 * k + 1]).toString(16));
    address = parts.join(":");
    i += 16;
  } else {
    throw new Error(`invalid address type ${atype}`);
  }
  if (!address) throw new Error("empty address");
  return { version, udp: cmd === 2, port, address, payloadIndex: i };
}

// VLESS UDP frames are [length:2][packet]; answer each DNS query via DoH.
function dnsOverHttps(ws, header) {
  let pending = new Uint8Array(0);
  return async (chunk) => {
    pending = concat(pending, chunk);
    while (pending.length >= 2) {
      const len = (pending[0] << 8) | pending[1];
      if (pending.length < 2 + len) break;
      const query = pending.slice(2, 2 + len);
      pending = pending.slice(2 + len);
      const resp = await fetch("https://cloudflare-dns.com/dns-query", {
        method: "POST",
        headers: { "content-type": "application/dns-message" },
        body: query,
      });
      const answer = new Uint8Array(await resp.arrayBuffer());
      const framed = concat(new Uint8Array([answer.length >> 8, answer.length & 0xff]), answer);
      if (ws.readyState !== WS_OPEN) return;
      ws.send(header ? concat(header, framed) : framed);
      header = null;
    }
  };
}

// ---------------------------------------------------------------- helpers

function websocketStream(ws, earlyDataHeader) {
  let cancelled = false;
  return new ReadableStream({
    start(controller) {
      ws.addEventListener("message", (e) => {
        if (cancelled) return;
        const d = e.data;
        controller.enqueue(typeof d === "string" ? new TextEncoder().encode(d) : new Uint8Array(d));
      });
      ws.addEventListener("close", () => {
        safeClose(ws);
        if (!cancelled) {
          try {
            controller.close();
          } catch {}
        }
      });
      ws.addEventListener("error", (e) => controller.error(e));
      // Xray sends the first bytes in Sec-WebSocket-Protocol when path has ?ed=
      const early = base64UrlDecode(earlyDataHeader);
      if (early) controller.enqueue(early);
    },
    cancel() {
      cancelled = true;
      safeClose(ws);
    },
  });
}

function base64UrlDecode(s) {
  if (!s) return null;
  try {
    const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
    return Uint8Array.from(bin, (c) => c.charCodeAt(0));
  } catch {
    return null;
  }
}

function splitHostPort(s, defPort) {
  const m = s.match(/^\[(.+)\](?::(\d+))?$/) || s.match(/^([^:]+)(?::(\d+))?$/);
  if (!m) return [s, defPort];
  return [m[1], m[2] ? Number(m[2]) : defPort];
}

function concat(...parts) {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.byteLength, 0));
  let off = 0;
  for (const p of parts) {
    out.set(new Uint8Array(p), off);
    off += p.byteLength;
  }
  return out;
}

function safeClose(ws) {
  try {
    if (ws.readyState === WS_OPEN || ws.readyState === 2) ws.close();
  } catch {}
}
