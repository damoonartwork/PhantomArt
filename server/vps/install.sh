#!/usr/bin/env bash
# PhantomArt server installer: Xray-core with VLESS + Reality + XTLS-Vision.
# Run on an Ubuntu/Debian VPS as root:
#   bash <(curl -fsSL https://raw.githubusercontent.com/damoonartwork/PhantomArt/claude/filter-breaker-app-hn2o05/server/vps/install.sh)
#
# Safe on servers that already run other things: everything lives in its own
# directory (/usr/local/phantomart) with its own Xray binary and its own
# systemd service ("phantomart"). Existing Xray/V2Ray/panel installs and ports
# are never touched. Running it again just prints the existing link.
#
# Optional environment variables:
#   PORT=443                 listening port (default: 443 if free, else a free one)
#   SNI=www.microsoft.com    site Reality impersonates (must support TLS 1.3)
#   NAME=PhantomArt          name shown in the client
#   REGENERATE=1             create new keys/link (the old link stops working)
#   UNINSTALL=1              remove PhantomArt from this server
set -euo pipefail

DIR="${DIR:-/usr/local/phantomart}"
SNI="${SNI:-www.microsoft.com}"
NAME="${NAME:-PhantomArt}"
SERVICE=phantomart
DRY_RUN="${DRY_RUN:-0}" # 1 = no root/apt/systemd/download (for testing)

XRAY="$DIR/xray"
CONFIG="$DIR/config.json"
LINKFILE="$DIR/link.txt"

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

port_free() {
  if command -v ss >/dev/null; then
    ! ss -Hltn "sport = :$1" | grep -q .
  else
    # No ss: a port is busy if something accepts connections on it.
    ! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
  fi
}

show_link() {
  local link
  link="$(cat "$LINKFILE")"
  echo
  say "Your config link (saved in $LINKFILE):"
  echo
  echo "$link"
  echo
  command -v qrencode >/dev/null && qrencode -t ansiutf8 "$link"
  echo "Copy the link above and use \"افزودن از کلیپ‌بورد\" in PhantomArt (or import it in v2rayN / v2rayNG)."
}

if [[ "$DRY_RUN" != 1 ]]; then
  [[ $EUID -eq 0 ]] || die "run as root (sudo -i)"
  command -v apt-get >/dev/null || die "only Ubuntu/Debian are supported"
fi

if [[ "${UNINSTALL:-0}" == 1 ]]; then
  systemctl disable --now "$SERVICE" 2>/dev/null || true
  rm -f "/etc/systemd/system/$SERVICE.service"
  systemctl daemon-reload
  rm -rf "$DIR"
  say "PhantomArt removed."
  exit 0
fi

# Already installed: keep the same link, just make sure it is running.
if [[ -f "$CONFIG" && -f "$LINKFILE" && "${REGENERATE:-0}" != 1 ]]; then
  if [[ "$DRY_RUN" != 1 ]]; then
    systemctl restart "$SERVICE"
    sleep 1
    systemctl is-active --quiet "$SERVICE" || die "service failed to start; see: journalctl -u $SERVICE -n 50"
  fi
  say "PhantomArt is already installed and running."
  show_link
  exit 0
fi

if [[ "$DRY_RUN" != 1 ]]; then
  say "Installing packages..."
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq curl unzip openssl qrencode ca-certificates iproute2 >/dev/null
fi

mkdir -p "$DIR"
chmod 700 "$DIR"

if [[ ! -x "$XRAY" ]]; then
  case "$(uname -m)" in
    x86_64 | amd64) ARCH=64 ;;
    aarch64 | arm64) ARCH=arm64-v8a ;;
    *) die "unsupported CPU architecture: $(uname -m)" ;;
  esac
  say "Downloading Xray-core..."
  TMP="$(mktemp -d)"
  curl -fsSL -o "$TMP/xray.zip" "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-$ARCH.zip"
  unzip -oq "$TMP/xray.zip" xray geoip.dat geosite.dat -d "$DIR"
  rm -rf "$TMP"
  chmod 755 "$XRAY"
fi

# Pick a port that nothing else is using.
if [[ -z "${PORT:-}" ]]; then
  for p in 443 8443 2053 2083 2087 2096; do
    if port_free "$p"; then PORT=$p; break; fi
  done
  while [[ -z "${PORT:-}" ]]; do
    p=$((20000 + RANDOM % 30000))
    port_free "$p" && PORT=$p
  done
elif ! port_free "$PORT"; then
  die "port $PORT is already in use; choose another with PORT=..."
fi

say "Generating keys..."
UUID="$("$XRAY" uuid)"
KEYS="$("$XRAY" x25519)"
# Output format differs between Xray versions ("Private key:" vs "PrivateKey:",
# "Public key:" vs "Password (PublicKey):").
PRIVATE_KEY="$(grep -i 'private' <<<"$KEYS" | awk -F': *' '{print $2}' | tr -d '[:space:]')"
PUBLIC_KEY="$(grep -i 'public' <<<"$KEYS" | awk -F': *' '{print $2}' | tr -d '[:space:]')"
[[ -n "$PRIVATE_KEY" && -n "$PUBLIC_KEY" ]] || die "could not parse x25519 keys: $KEYS"
SHORT_ID="$(openssl rand -hex 8)"

cat >"$CONFIG" <<EOF
{
  "log": { "loglevel": "warning" },
  "inbounds": [
    {
      "listen": "0.0.0.0",
      "port": $PORT,
      "protocol": "vless",
      "settings": {
        "clients": [ { "id": "$UUID", "flow": "xtls-rprx-vision" } ],
        "decryption": "none"
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "dest": "$SNI:443",
          "serverNames": [ "$SNI" ],
          "privateKey": "$PRIVATE_KEY",
          "shortIds": [ "$SHORT_ID" ]
        }
      },
      "sniffing": { "enabled": true, "destOverride": [ "http", "tls", "quic" ], "routeOnly": true }
    }
  ],
  "outbounds": [
    { "tag": "direct", "protocol": "freedom" },
    { "tag": "block", "protocol": "blackhole" }
  ],
  "routing": {
    "rules": [
      { "type": "field", "ip": [ "geoip:private" ], "outboundTag": "block" },
      { "type": "field", "protocol": [ "bittorrent" ], "outboundTag": "block" }
    ]
  }
}
EOF
chmod 600 "$CONFIG"
XRAY_LOCATION_ASSET="$DIR" "$XRAY" run -test -c "$CONFIG" >/dev/null || die "generated config is invalid"

if [[ "$DRY_RUN" != 1 ]]; then
  cat >"/etc/systemd/system/$SERVICE.service" <<EOF
[Unit]
Description=PhantomArt (Xray VLESS Reality)
After=network-online.target
Wants=network-online.target

[Service]
Environment=XRAY_LOCATION_ASSET=$DIR
ExecStart=$XRAY run -c $CONFIG
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

  if command -v ufw >/dev/null && ufw status | grep -q 'Status: active'; then
    ufw allow "$PORT"/tcp >/dev/null
  fi

  say "Starting PhantomArt service..."
  systemctl daemon-reload
  systemctl enable "$SERVICE" >/dev/null 2>&1
  systemctl restart "$SERVICE"
  sleep 1
  systemctl is-active --quiet "$SERVICE" || die "service failed to start; see: journalctl -u $SERVICE -n 50"
fi

IP="${IP:-$(curl -fsS4 --max-time 10 https://api.ipify.org || hostname -I | awk '{print $1}')}"
echo "vless://$UUID@$IP:$PORT?encryption=none&flow=xtls-rprx-vision&security=reality&sni=$SNI&fp=chrome&pbk=$PUBLIC_KEY&sid=$SHORT_ID&type=tcp#$NAME" >"$LINKFILE"

say "Done!"
show_link
