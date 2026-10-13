package xray

import (
	"encoding/json"
	"testing"

	"github.com/damoonartwork/phantomart/internal/link"
)

func build(t *testing.T, raw string, opt Options) map[string]any {
	t.Helper()
	p, err := link.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build(p, opt)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func get(v any, path ...any) any {
	for _, k := range path {
		switch k := k.(type) {
		case string:
			v = v.(map[string]any)[k]
		case int:
			v = v.([]any)[k]
		}
	}
	return v
}

func TestBuildVLESSReality(t *testing.T) {
	cfg := build(t, "vless://uuid@example.com:443?security=reality&sni=www.speedtest.net&pbk=KEY&sid=ab&flow=xtls-rprx-vision",
		Options{SocksPort: 10808, HTTPPort: 10809, BypassIran: true})

	out := get(cfg, "outbounds", 0)
	if get(out, "tag") != "proxy" || get(out, "protocol") != "vless" {
		t.Fatalf("outbound = %v", out)
	}
	user := get(out, "settings", "vnext", 0, "users", 0)
	if get(user, "id") != "uuid" || get(user, "flow") != "xtls-rprx-vision" || get(user, "encryption") != "none" {
		t.Errorf("user = %v", user)
	}
	ss := get(out, "streamSettings")
	if get(ss, "security") != "reality" || get(ss, "realitySettings", "publicKey") != "KEY" ||
		get(ss, "realitySettings", "serverName") != "www.speedtest.net" || get(ss, "realitySettings", "fingerprint") != "chrome" {
		t.Errorf("stream = %v", ss)
	}

	if get(cfg, "inbounds", 0, "port") != float64(10808) || get(cfg, "inbounds", 1, "port") != float64(10809) {
		t.Error("wrong inbound ports")
	}
	if get(cfg, "inbounds", 0, "listen") != "127.0.0.1" {
		t.Error("must listen on loopback by default")
	}
	rules := get(cfg, "routing", "rules").([]any)
	if len(rules) != 3 {
		t.Errorf("want private + 2 iran rules, got %d", len(rules))
	}
}

func TestBuildWSTLSUsesHostAsSNI(t *testing.T) {
	cfg := build(t, "vless://u@1.2.3.4:443?type=ws&security=tls&host=cdn.example.com&path=%2Fws",
		Options{SocksPort: 1, HTTPPort: 2})
	ss := get(cfg, "outbounds", 0, "streamSettings")
	if get(ss, "wsSettings", "path") != "/ws" || get(ss, "wsSettings", "host") != "cdn.example.com" {
		t.Errorf("ws = %v", ss)
	}
	if get(ss, "tlsSettings", "serverName") != "cdn.example.com" {
		t.Errorf("sni = %v", get(ss, "tlsSettings", "serverName"))
	}
	if n := len(get(cfg, "routing", "rules").([]any)); n != 1 {
		t.Errorf("without bypass want 1 rule, got %d", n)
	}
}

func TestBuildShadowsocksAndTrojan(t *testing.T) {
	cfg := build(t, "ss://aes-256-gcm:pw@5.6.7.8:8388", Options{SocksPort: 1, HTTPPort: 2, AllowLAN: true})
	srv := get(cfg, "outbounds", 0, "settings", "servers", 0)
	if get(srv, "method") != "aes-256-gcm" || get(srv, "password") != "pw" {
		t.Errorf("ss = %v", srv)
	}
	if get(cfg, "inbounds", 0, "listen") != "0.0.0.0" {
		t.Error("AllowLAN should listen on all interfaces")
	}

	cfg = build(t, "trojan://pw@t.example.com:443?type=grpc&serviceName=svc", Options{SocksPort: 1, HTTPPort: 2})
	ss := get(cfg, "outbounds", 0, "streamSettings")
	if get(ss, "security") != "tls" || get(ss, "grpcSettings", "serviceName") != "svc" {
		t.Errorf("trojan stream = %v", ss)
	}
}

func TestBuildFragmentOnlyForTLS(t *testing.T) {
	cfg := build(t, "vless://u@cdn.example.com:443?type=ws&security=tls", Options{SocksPort: 1, HTTPPort: 2, Fragment: true})
	if get(cfg, "outbounds", 0, "streamSettings", "sockopt", "dialerProxy") != "fragment" {
		t.Error("proxy outbound should dial through the fragment outbound")
	}
	outs := get(cfg, "outbounds").([]any)
	if last := outs[len(outs)-1]; get(last, "tag") != "fragment" || get(last, "settings", "fragment", "packets") != "tlshello" {
		t.Errorf("fragment outbound = %v", last)
	}

	cfg = build(t, "vless://u@h:443?security=reality&pbk=K", Options{SocksPort: 1, HTTPPort: 2, Fragment: true})
	if _, ok := get(cfg, "outbounds", 0, "streamSettings").(map[string]any)["sockopt"]; ok {
		t.Error("fragment must not apply to reality")
	}
}
