// Package xray builds Xray-core JSON configurations from link.Profile values.
package xray

import (
	"encoding/json"
	"fmt"

	"github.com/damoonartwork/phantomart/internal/link"
)

// Options controls the local side of the generated configuration.
type Options struct {
	SocksPort  int
	HTTPPort   int
	AllowLAN   bool // listen on 0.0.0.0 instead of 127.0.0.1
	BypassIran bool // route Iranian domains/IPs directly
	BlockAds   bool
	Fragment   bool   // split the TLS ClientHello to evade SNI filtering
	LogLevel   string // debug, info, warning, error, none
}

type obj = map[string]any

// Build returns a complete Xray config for the given profile.
func Build(p *link.Profile, opt Options) ([]byte, error) {
	out, err := Outbound(p)
	if err != nil {
		return nil, err
	}
	listen := "127.0.0.1"
	if opt.AllowLAN {
		listen = "0.0.0.0"
	}
	sniffing := obj{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true}

	outbounds := []obj{
		out,
		{"tag": "direct", "protocol": "freedom", "settings": obj{}},
		{"tag": "block", "protocol": "blackhole", "settings": obj{}},
	}
	if opt.Fragment && p.Security == "tls" {
		out["streamSettings"].(obj)["sockopt"] = obj{"dialerProxy": "fragment"}
		outbounds = append(outbounds, obj{
			"tag":            "fragment",
			"protocol":       "freedom",
			"settings":       obj{"fragment": obj{"packets": "tlshello", "length": "100-200", "interval": "10-20"}},
			"streamSettings": obj{"sockopt": obj{"tcpNoDelay": true}},
		})
	}

	cfg := obj{
		"log": obj{"loglevel": orDefault(opt.LogLevel, "warning")},
		"inbounds": []obj{
			{
				"tag": "socks-in", "listen": listen, "port": opt.SocksPort, "protocol": "socks",
				"settings": obj{"udp": true, "auth": "noauth"}, "sniffing": sniffing,
			},
			{
				"tag": "http-in", "listen": listen, "port": opt.HTTPPort, "protocol": "http",
				"settings": obj{}, "sniffing": sniffing,
			},
		},
		"outbounds": outbounds,
		"routing": obj{
			// AsIs: never resolve domains locally, so DNS queries are not
			// leaked to (or poisoned by) the local network.
			"domainStrategy": "AsIs",
			"rules":          routingRules(opt),
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

func routingRules(opt Options) []obj {
	rules := []obj{
		{"type": "field", "ip": []string{"geoip:private"}, "outboundTag": "direct"},
	}
	if opt.BlockAds {
		rules = append(rules, obj{"type": "field", "domain": []string{"geosite:category-ads-all"}, "outboundTag": "block"})
	}
	if opt.BypassIran {
		rules = append(rules,
			obj{"type": "field", "domain": []string{"domain:ir", "geosite:category-ir"}, "outboundTag": "direct"},
			obj{"type": "field", "ip": []string{"geoip:ir"}, "outboundTag": "direct"},
		)
	}
	return rules
}

// Outbound returns the "proxy" outbound object for a profile.
func Outbound(p *link.Profile) (obj, error) {
	var settings obj
	switch p.Protocol {
	case "vless":
		user := obj{"id": p.UUID, "encryption": orDefault(p.Encryption, "none")}
		if p.Flow != "" {
			user["flow"] = p.Flow
		}
		settings = obj{"vnext": []obj{{"address": p.Address, "port": p.Port, "users": []obj{user}}}}
	case "vmess":
		user := obj{"id": p.UUID, "alterId": p.AlterID, "security": orDefault(p.Encryption, "auto")}
		settings = obj{"vnext": []obj{{"address": p.Address, "port": p.Port, "users": []obj{user}}}}
	case "trojan":
		settings = obj{"servers": []obj{{"address": p.Address, "port": p.Port, "password": p.Password}}}
	case "shadowsocks":
		settings = obj{"servers": []obj{{"address": p.Address, "port": p.Port, "method": p.Method, "password": p.Password}}}
	default:
		return nil, fmt.Errorf("unsupported protocol %q", p.Protocol)
	}
	return obj{
		"tag":            "proxy",
		"protocol":       p.Protocol,
		"settings":       settings,
		"streamSettings": streamSettings(p),
	}, nil
}

func streamSettings(p *link.Profile) obj {
	network := orDefault(p.Network, "tcp")
	ss := obj{"network": network}

	switch network {
	case "tcp":
		if p.HeaderType == "http" {
			req := obj{"path": []string{orDefault(p.Path, "/")}}
			if p.Host != "" {
				req["headers"] = obj{"Host": splitComma(p.Host)}
			}
			ss["tcpSettings"] = obj{"header": obj{"type": "http", "request": req}}
		}
	case "ws":
		ws := obj{"path": orDefault(p.Path, "/")}
		if p.Host != "" {
			ws["host"] = p.Host
		}
		ss["wsSettings"] = ws
	case "httpupgrade":
		hu := obj{"path": orDefault(p.Path, "/")}
		if p.Host != "" {
			hu["host"] = p.Host
		}
		ss["httpupgradeSettings"] = hu
	case "xhttp":
		xh := obj{"path": orDefault(p.Path, "/")}
		if p.Host != "" {
			xh["host"] = p.Host
		}
		if p.Mode != "" {
			xh["mode"] = p.Mode
		}
		ss["xhttpSettings"] = xh
	case "grpc":
		ss["grpcSettings"] = obj{"serviceName": p.ServiceName, "multiMode": p.Mode == "multi"}
	case "h2":
		h2 := obj{"path": orDefault(p.Path, "/")}
		if p.Host != "" {
			h2["host"] = splitComma(p.Host)
		}
		ss["httpSettings"] = h2
	case "kcp":
		ss["kcpSettings"] = obj{"header": obj{"type": orDefault(p.HeaderType, "none")}}
	}

	switch p.Security {
	case "tls":
		ss["security"] = "tls"
		tls := obj{"serverName": serverName(p), "allowInsecure": p.AllowInsecure}
		if p.Fingerprint != "" {
			tls["fingerprint"] = p.Fingerprint
		}
		if len(p.ALPN) > 0 {
			tls["alpn"] = p.ALPN
		}
		ss["tlsSettings"] = tls
	case "reality":
		ss["security"] = "reality"
		ss["realitySettings"] = obj{
			"serverName":  serverName(p),
			"fingerprint": orDefault(p.Fingerprint, "chrome"),
			"publicKey":   p.PublicKey,
			"shortId":     p.ShortID,
			"spiderX":     p.SpiderX,
		}
	default:
		ss["security"] = "none"
	}
	return ss
}

func serverName(p *link.Profile) string {
	if p.SNI != "" {
		return p.SNI
	}
	if p.Host != "" {
		return splitComma(p.Host)[0]
	}
	return p.Address
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := s[start:i]; part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	if len(out) == 0 {
		out = []string{s}
	}
	return out
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
