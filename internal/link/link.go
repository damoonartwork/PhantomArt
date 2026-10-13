// Package link parses share links (vless://, vmess://, trojan://, ss://)
// into Profile values that the xray package can turn into an outbound.
package link

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Profile is a single server configuration.
type Profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // vless, vmess, trojan, shadowsocks
	Address  string `json:"address"`
	Port     int    `json:"port"`

	// Credentials
	UUID       string `json:"uuid,omitempty"`       // vless / vmess
	AlterID    int    `json:"alterId,omitempty"`    // vmess
	Encryption string `json:"encryption,omitempty"` // vless ("none") / vmess cipher ("auto")
	Flow       string `json:"flow,omitempty"`       // vless, e.g. xtls-rprx-vision
	Password   string `json:"password,omitempty"`   // trojan / shadowsocks
	Method     string `json:"method,omitempty"`     // shadowsocks cipher

	// Transport
	Network     string `json:"network"` // tcp, ws, grpc, httpupgrade, xhttp, h2, kcp
	HeaderType  string `json:"headerType,omitempty"`
	Host        string `json:"host,omitempty"`
	Path        string `json:"path,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Mode        string `json:"mode,omitempty"` // grpc "multi" / xhttp mode

	// Security
	Security      string   `json:"security"` // none, tls, reality
	SNI           string   `json:"sni,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	ALPN          []string `json:"alpn,omitempty"`
	AllowInsecure bool     `json:"allowInsecure,omitempty"`
	PublicKey     string   `json:"publicKey,omitempty"` // reality
	ShortID       string   `json:"shortId,omitempty"`   // reality
	SpiderX       string   `json:"spiderX,omitempty"`   // reality

	Subscription string `json:"subscription,omitempty"` // owning subscription URL, if any
	Raw          string `json:"raw"`
}

// ErrUnsupported is returned for links with an unknown scheme.
var ErrUnsupported = errors.New("unsupported link")

// Parse parses a single share link.
func Parse(raw string) (*Profile, error) {
	raw = strings.TrimSpace(raw)
	scheme, _, ok := strings.Cut(raw, "://")
	if !ok {
		return nil, ErrUnsupported
	}
	var (
		p   *Profile
		err error
	)
	switch strings.ToLower(scheme) {
	case "vless":
		p, err = parseURLStyle(raw, "vless")
	case "trojan":
		p, err = parseURLStyle(raw, "trojan")
	case "vmess":
		p, err = parseVMess(raw)
	case "ss":
		p, err = parseShadowsocks(raw)
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, err
	}
	if p.Address == "" || p.Port <= 0 || p.Port > 65535 {
		return nil, fmt.Errorf("invalid address or port in %s link", scheme)
	}
	if p.Name == "" {
		p.Name = net.JoinHostPort(p.Address, strconv.Itoa(p.Port))
	}
	p.ID = newID()
	p.Raw = raw
	return p, nil
}

// ParseMany extracts every valid link from free text (one link per line or
// whitespace separated). Invalid entries are skipped.
func ParseMany(text string) []*Profile {
	var out []*Profile
	for _, field := range strings.Fields(text) {
		if p, err := Parse(field); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func parseURLStyle(raw, protocol string) (*Profile, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}
	q := u.Query()
	p := &Profile{
		Name:     u.Fragment,
		Protocol: protocol,
		Address:  u.Hostname(),
		Port:     port,
	}
	secret := u.User.Username()
	if protocol == "vless" {
		p.UUID = secret
		p.Encryption = orDefault(q.Get("encryption"), "none")
		p.Flow = q.Get("flow")
	} else {
		p.Password = secret
	}

	p.Network = normalizeNetwork(q.Get("type"))
	p.HeaderType = q.Get("headerType")
	p.Host = q.Get("host")
	p.Path = q.Get("path")
	p.ServiceName = q.Get("serviceName")
	p.Mode = q.Get("mode")

	defSecurity := "none"
	if protocol == "trojan" {
		defSecurity = "tls"
	}
	p.Security = orDefault(q.Get("security"), defSecurity)
	p.SNI = orDefault(q.Get("sni"), q.Get("peer"))
	p.Fingerprint = q.Get("fp")
	p.ALPN = splitList(q.Get("alpn"))
	p.AllowInsecure = q.Get("allowInsecure") == "1" || q.Get("insecure") == "1"
	p.PublicKey = q.Get("pbk")
	p.ShortID = q.Get("sid")
	p.SpiderX = q.Get("spx")
	return p, nil
}

// vmessJSON is the de-facto v2rayN share format. Several fields appear as
// either strings or numbers in the wild, hence json.RawMessage + flexString.
type vmessJSON struct {
	PS   string          `json:"ps"`
	Add  string          `json:"add"`
	Port json.RawMessage `json:"port"`
	ID   string          `json:"id"`
	Aid  json.RawMessage `json:"aid"`
	Scy  string          `json:"scy"`
	Net  string          `json:"net"`
	Type string          `json:"type"`
	Host string          `json:"host"`
	Path string          `json:"path"`
	TLS  string          `json:"tls"`
	SNI  string          `json:"sni"`
	ALPN string          `json:"alpn"`
	FP   string          `json:"fp"`
}

func parseVMess(raw string) (*Profile, error) {
	data, err := decodeBase64(strings.TrimPrefix(raw[len("vmess://"):], "//"))
	if err != nil {
		return nil, fmt.Errorf("vmess: %w", err)
	}
	var v vmessJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("vmess: %w", err)
	}
	port, err := strconv.Atoi(flexString(v.Port))
	if err != nil {
		return nil, fmt.Errorf("vmess: invalid port")
	}
	aid, _ := strconv.Atoi(flexString(v.Aid))
	p := &Profile{
		Name:        v.PS,
		Protocol:    "vmess",
		Address:     v.Add,
		Port:        port,
		UUID:        v.ID,
		AlterID:     aid,
		Encryption:  orDefault(v.Scy, "auto"),
		Network:     normalizeNetwork(v.Net),
		HeaderType:  v.Type,
		Host:        v.Host,
		Path:        v.Path,
		Security:    orDefault(v.TLS, "none"),
		SNI:         v.SNI,
		Fingerprint: v.FP,
		ALPN:        splitList(v.ALPN),
	}
	if p.Network == "grpc" {
		p.ServiceName = v.Path
		p.Path = ""
		if v.Type == "multi" {
			p.Mode = "multi"
		}
		p.HeaderType = ""
	}
	return p, nil
}

// parseShadowsocks supports SIP002 (ss://base64(method:pass)@host:port#name,
// or with a percent-encoded plain userinfo) and the legacy
// ss://base64(method:pass@host:port)#name form.
func parseShadowsocks(raw string) (*Profile, error) {
	body := raw[len("ss://"):]
	body, fragment, _ := strings.Cut(body, "#")
	name, _ := url.PathUnescape(fragment)
	body, _, _ = strings.Cut(body, "?") // plugins are not supported

	if !strings.Contains(body, "@") {
		decoded, err := decodeBase64(body)
		if err != nil {
			return nil, fmt.Errorf("ss: %w", err)
		}
		body = string(decoded)
	}
	at := strings.LastIndex(body, "@")
	if at < 0 {
		return nil, errors.New("ss: missing server")
	}
	userinfo, hostport := body[:at], body[at+1:]

	var method, password string
	if m, pw, ok := strings.Cut(userinfo, ":"); ok {
		method, password = m, pw
	} else if decoded, err := decodeBase64(userinfo); err == nil {
		m, pw, ok := strings.Cut(string(decoded), ":")
		if !ok {
			return nil, errors.New("ss: invalid userinfo")
		}
		method, password = m, pw
	} else {
		return nil, errors.New("ss: invalid userinfo")
	}
	if m, err := url.PathUnescape(method); err == nil {
		method = m
	}
	if pw, err := url.PathUnescape(password); err == nil {
		password = pw
	}

	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		return nil, fmt.Errorf("ss: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("ss: invalid port")
	}
	return &Profile{
		Name:     name,
		Protocol: "shadowsocks",
		Address:  host,
		Port:     port,
		Method:   method,
		Password: password,
		Network:  "tcp",
		Security: "none",
	}, nil
}

// DecodeSubscription returns the text content of a subscription body, which
// is usually base64 of newline-separated links but may also be plain text.
func DecodeSubscription(body []byte) string {
	s := strings.TrimSpace(string(body))
	if strings.Contains(s, "://") {
		return s
	}
	if decoded, err := decodeBase64(strings.Join(strings.Fields(s), "")); err == nil {
		return string(decoded)
	}
	return s
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "=")
	if strings.ContainsAny(s, "-_") {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func flexString(m json.RawMessage) string {
	var s string
	if json.Unmarshal(m, &s) == nil {
		return s
	}
	return strings.TrimSpace(string(m))
}

func normalizeNetwork(n string) string {
	switch n = strings.ToLower(n); n {
	case "", "raw":
		return "tcp"
	case "http":
		return "h2"
	case "splithttp":
		return "xhttp"
	}
	return n
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
