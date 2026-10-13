#!/usr/bin/env bash
# PhantomArt server installer: Xray-core with VLESS + Reality + XTLS-Vision.
# Run on a fresh Ubuntu/Debian VPS as root:
#   bash <(curl -fsSL https://raw.githubusercontent.com/damoonartwork/PhantomArt/claude/filter-breaker-app-hn2o05/server/vps/install.sh)
#
# Optional environment variables:
#   PORT=443                 listening port
#   SNI=www.microsoft.com    site Reality impersonates (must support TLS 1.3)
#   NAME=PhantomArt          name shown in the client
set -euo pipefail

PORT="${PORT:-443}"
SNI="${SNI:-www.microsoft.com}"
NAME="${NAME:-PhantomArt}"
CONFIG="${CONFIG:-/usr/local/etc/xray/config.json}"
XRAY="${XRAY:-/usr/local/bin/xray}"
DRY_RUN="${DRY_RUN:-0}" # 1 = only write config (for testing)

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

if [[ "$DRY_RUN" != 1 ]]; then
  [[ $EUID -eq 0 ]] || die "run as root (sudo -i)"
  command -v apt-get >/dev/null || die "only Ubuntu/Debian are supported"

  say "Installing packages..."
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq curl openssl qrencode ca-certificates >/dev/null

  say "Installing Xray-core..."
  bash -c "$(curl -fsSL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install >/dev/null
fi
[[ -x "$XRAY" ]] || die "xray not found at $XRAY"

say "Generating keys..."
UUID="$("$XRAY" uuid)"
KEYS="$("$XRAY" x25519)"
# Output format differs between Xray versions ("Private key:" vs "PrivateKey:",
# "Public key:" vs "Password (PublicKey):").
PRIVATE_KEY="$(grep -i 'private' <<<"$KEYS" | awk -F': *' '{print $2}' | tr -d '[:space:]')"
PUBLIC_KEY="$(grep -i 'public' <<<"$KEYS" | awk -F': *' '{print $2}' | tr -d '[:space:]')"
[[ -n "$PRIVATE_KEY" && -n "$PUBLIC_KEY" ]] || die "could not parse x25519 keys: $KEYS"
SHORT_ID="$(openssl rand -hex 8)"

mkdir -p "$(dirname "$CONFIG")"
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
"$XRAY" run -test -c "$CONFIG" >/dev/null || die "generated config is invalid"

if [[ "$DRY_RUN" != 1 ]]; then
  say "Enabling BBR congestion control..."
  grep -q 'tcp_congestion_control=bbr' /etc/sysctl.conf 2>/dev/null || {
    echo 'net.core.default_qdisc=fq' >>/etc/sysctl.conf
    echo 'net.ipv4.tcp_congestion_control=bbr' >>/etc/sysctl.conf
  }
  sysctl -p >/dev/null 2>&1 || true

  if command -v ufw >/dev/null && ufw status | grep -q active; then
    ufw allow "$PORT"/tcp >/dev/null
  fi

  say "Starting Xray..."
  systemctl enable xray >/dev/null 2>&1
  systemctl restart xray
  sleep 1
  systemctl is-active --quiet xray || die "xray failed to start; see: journalctl -u xray -n 50"
fi

IP="${IP:-$(curl -fsS4 --max-time 10 https://api.ipify.org || hostname -I | awk '{print $1}')}"
LINK="vless://$UUID@$IP:$PORT?encryption=none&flow=xtls-rprx-vision&security=reality&sni=$SNI&fp=chrome&pbk=$PUBLIC_KEY&sid=$SHORT_ID&type=tcp#$NAME"
echo "$LINK" >"$(dirname "$CONFIG")/phantomart-link.txt"

echo
say "Done! Your config link (also saved in $(dirname "$CONFIG")/phantomart-link.txt):"
echo
echo "$LINK"
echo
if command -v qrencode >/dev/null; then
  qrencode -t ansiutf8 "$LINK"
fi
echo "Copy the link above and use \"افزودن از کلیپ‌بورد\" in PhantomArt."
