#!/usr/bin/env bash
# PhantomArt bridge: run on a server INSIDE Iran to relay connections to your
# foreign PhantomArt server. Clients connect to the Iranian server, which is
# not filtered, and it forwards the raw traffic abroad.
#
# Uses only the kernel's built-in forwarding (iptables), so nothing has to be
# downloaded from abroad. Usage, as root:
#   TARGET=89.169.55.36:8443 bash install.sh
#
# Optional environment variables:
#   PORT=8443        port clients connect to on this server (default: target port, or a free one)
#   LINK='vless://…' your original config link; a ready-to-use bridged link is printed
#   UNINSTALL=1      remove the bridge listening on PORT
set -euo pipefail

DIR=/usr/local/phantomart-bridge
DRY_RUN="${DRY_RUN:-0}" # 1 = skip systemd (for testing)

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

port_free() {
  if command -v ss >/dev/null; then
    ! ss -Hltn "sport = :$1" | grep -q .
  else
    ! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
  fi
}

[[ $EUID -eq 0 ]] || die "run as root (sudo -i)"
command -v iptables >/dev/null || die "iptables is not installed (apt-get install -y iptables)"

if [[ "${UNINSTALL:-0}" == 1 ]]; then
  [[ -n "${PORT:-}" ]] || die "set PORT=... to choose which bridge to remove"
  [[ -x "$DIR/bridge-$PORT.sh" ]] || die "no bridge on port $PORT"
  "$DIR/bridge-$PORT.sh" stop
  if [[ "$DRY_RUN" != 1 ]]; then
    systemctl disable --now "phantomart-bridge-$PORT" 2>/dev/null || true
    rm -f "/etc/systemd/system/phantomart-bridge-$PORT.service"
    systemctl daemon-reload
  fi
  rm -f "$DIR/bridge-$PORT.sh"
  say "Bridge on port $PORT removed."
  exit 0
fi

[[ -n "${TARGET:-}" ]] || die "set TARGET=ip:port of your foreign server, e.g. TARGET=89.169.55.36:8443"
THOST="${TARGET%:*}"
TPORT="${TARGET##*:}"
[[ "$TPORT" =~ ^[0-9]+$ && "$THOST" != "$TARGET" ]] || die "TARGET must look like 1.2.3.4:8443"
if [[ ! "$THOST" =~ ^[0-9.]+$ ]]; then
  TIP="$(getent ahostsv4 "$THOST" | awk 'NR==1{print $1}')"
  [[ -n "$TIP" ]] || die "cannot resolve $THOST"
else
  TIP="$THOST"
fi

say "Checking that $TIP:$TPORT is reachable from this server..."
timeout 6 bash -c "exec 3<>/dev/tcp/$TIP/$TPORT" 2>/dev/null ||
  die "cannot connect to $TIP:$TPORT from here. Is the foreign server running?"

if [[ -z "${PORT:-}" ]]; then
  for p in "$TPORT" 443 8443 2053 2083 2087 2096; do
    if port_free "$p"; then PORT=$p; break; fi
  done
  while [[ -z "${PORT:-}" ]]; do
    p=$((20000 + RANDOM % 30000))
    port_free "$p" && PORT=$p
  done
elif ! port_free "$PORT"; then
  die "port $PORT is already in use on this server; choose another with PORT=..."
fi

mkdir -p "$DIR"
SCRIPT="$DIR/bridge-$PORT.sh"
cat >"$SCRIPT" <<EOF
#!/bin/sh
# Forward TCP port $PORT on this server to $TIP:$TPORT. Usage: $SCRIPT start|stop
TAG="-m comment --comment phantomart-bridge-$PORT"
DNAT="PREROUTING -p tcp --dport $PORT \$TAG -j DNAT --to-destination $TIP:$TPORT"
SNAT="POSTROUTING -p tcp -d $TIP --dport $TPORT \$TAG -j MASQUERADE"
FWD1="FORWARD -p tcp -d $TIP --dport $TPORT \$TAG -j ACCEPT"
FWD2="FORWARD -p tcp -s $TIP --sport $TPORT -m conntrack --ctstate ESTABLISHED,RELATED \$TAG -j ACCEPT"

# add TABLE -A|-I RULE...: add the rule unless it already exists
add() { t=\$1; m=\$2; shift 2; iptables -t \$t -C "\$@" 2>/dev/null || iptables -t \$t \$m "\$@"; }
# del TABLE RULE...: remove every copy of the rule
del() { t=\$1; shift; while iptables -t \$t -D "\$@" 2>/dev/null; do :; done; }

case "\$1" in
  start)
    sysctl -qw net.ipv4.ip_forward=1
    add nat -A \$DNAT
    add nat -A \$SNAT
    add filter -I \$FWD1
    add filter -I \$FWD2
    ;;
  stop)
    del nat \$DNAT
    del nat \$SNAT
    del filter \$FWD1
    del filter \$FWD2
    ;;
  *) echo "usage: \$0 start|stop" >&2; exit 2 ;;
esac
EOF
chmod 755 "$SCRIPT"
"$SCRIPT" start
grep -q '^net.ipv4.ip_forward *= *1' /etc/sysctl.conf 2>/dev/null || echo 'net.ipv4.ip_forward=1' >>/etc/sysctl.conf

if [[ "$DRY_RUN" != 1 ]]; then
  cat >"/etc/systemd/system/phantomart-bridge-$PORT.service" <<EOF
[Unit]
Description=PhantomArt bridge: port $PORT -> $TIP:$TPORT
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=$SCRIPT start
ExecStop=$SCRIPT stop

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now "phantomart-bridge-$PORT" >/dev/null 2>&1
fi

MYIP="${MYIP:-$(curl -fsS4 --max-time 10 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')}"
say "Bridge ready: $MYIP:$PORT  ->  $TIP:$TPORT"
echo
if [[ -n "${LINK:-}" ]]; then
  # Replace only the host:port after '@'; keep SNI, keys and everything else.
  NEWLINK="$(sed -E "s#@[^:/?]+:[0-9]+#@$MYIP:$PORT#; s/#[^#]*\$/#PhantomArt-Bridge/" <<<"$LINK")"
  say "Your bridged config link:"
  echo
  echo "$NEWLINK"
  echo
  command -v qrencode >/dev/null && qrencode -t ansiutf8 "$NEWLINK"
else
  echo "In your app, edit the server and change only:"
  echo "  Address -> $MYIP"
  echo "  Port    -> $PORT"
  echo "(everything else, including SNI and keys, stays the same)"
fi
