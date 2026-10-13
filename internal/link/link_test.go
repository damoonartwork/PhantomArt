package link

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func TestParseVLESSReality(t *testing.T) {
	raw := "vless://11111111-2222-3333-4444-555555555555@example.com:443?type=tcp&security=reality&sni=www.speedtest.net&fp=chrome&pbk=PUBKEY&sid=ab12&flow=xtls-rprx-vision&spx=%2F#My%20Server"
	p, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := Profile{
		Name: "My Server", Protocol: "vless", Address: "example.com", Port: 443,
		UUID: "11111111-2222-3333-4444-555555555555", Encryption: "none", Flow: "xtls-rprx-vision",
		Network: "tcp", Security: "reality", SNI: "www.speedtest.net", Fingerprint: "chrome",
		PublicKey: "PUBKEY", ShortID: "ab12", SpiderX: "/",
	}
	want.ID, want.Raw = p.ID, raw
	if !reflect.DeepEqual(*p, want) {
		t.Errorf("got  %+v\nwant %+v", *p, want)
	}
}

func TestParseVLESSWebSocketIPv6(t *testing.T) {
	p, err := Parse("vless://uuid@[2001:db8::1]:8443?type=ws&security=tls&host=cdn.example.com&path=%2Fws%3Fed%3D2048&alpn=h2,http/1.1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Address != "2001:db8::1" || p.Port != 8443 {
		t.Errorf("address = %s:%d", p.Address, p.Port)
	}
	if p.Network != "ws" || p.Host != "cdn.example.com" || p.Path != "/ws?ed=2048" {
		t.Errorf("transport = %s %s %s", p.Network, p.Host, p.Path)
	}
	if !reflect.DeepEqual(p.ALPN, []string{"h2", "http/1.1"}) {
		t.Errorf("alpn = %v", p.ALPN)
	}
	if p.Name != "[2001:db8::1]:8443" {
		t.Errorf("default name = %q", p.Name)
	}
}

func TestParseTrojanDefaultsToTLS(t *testing.T) {
	p, err := Parse("trojan://secret@t.example.com:443?type=grpc&serviceName=svc&mode=multi#tr")
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != "trojan" || p.Password != "secret" || p.Security != "tls" {
		t.Errorf("got %+v", p)
	}
	if p.Network != "grpc" || p.ServiceName != "svc" || p.Mode != "multi" {
		t.Errorf("grpc = %+v", p)
	}
}

func TestParseVMess(t *testing.T) {
	js := `{"v":"2","ps":"vm","add":"v.example.com","port":"443","id":"uuid-1","aid":0,"scy":"auto","net":"ws","type":"none","host":"h.example.com","path":"/p","tls":"tls","sni":"s.example.com","fp":"firefox"}`
	p, err := Parse("vmess://" + base64.StdEncoding.EncodeToString([]byte(js)))
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "vm" || p.Address != "v.example.com" || p.Port != 443 || p.UUID != "uuid-1" {
		t.Errorf("got %+v", p)
	}
	if p.Network != "ws" || p.Host != "h.example.com" || p.Path != "/p" || p.Security != "tls" || p.SNI != "s.example.com" {
		t.Errorf("stream = %+v", p)
	}
}

func TestParseVMessNumericPortGRPC(t *testing.T) {
	js := `{"ps":"g","add":"1.2.3.4","port":2053,"id":"u","aid":"0","net":"grpc","path":"svc","type":"multi","tls":""}`
	p, err := Parse("vmess://" + base64.RawURLEncoding.EncodeToString([]byte(js)))
	if err != nil {
		t.Fatal(err)
	}
	if p.Port != 2053 || p.ServiceName != "svc" || p.Mode != "multi" || p.Path != "" || p.Security != "none" {
		t.Errorf("got %+v", p)
	}
}

func TestParseShadowsocks(t *testing.T) {
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pa:ss"))
	cases := []string{
		"ss://" + userinfo + "@1.2.3.4:8388#SS%20one",
		"ss://" + base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pa:ss@1.2.3.4:8388")) + "#SS%20one",
		"ss://chacha20-ietf-poly1305:pa%3Ass@1.2.3.4:8388#SS%20one",
	}
	for _, raw := range cases {
		p, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if p.Method != "chacha20-ietf-poly1305" || p.Password != "pa:ss" || p.Address != "1.2.3.4" || p.Port != 8388 || p.Name != "SS one" {
			t.Errorf("%s: got %+v", raw, p)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{"", "http://example.com", "vless://uuid@host", "vmess://notbase64!!", "ss://garbage"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", raw)
		}
	}
}

func TestParseManyAndSubscription(t *testing.T) {
	text := "vless://a@h1:443#one\nnot a link\r\ntrojan://b@h2:443#two\n"
	sub := base64.StdEncoding.EncodeToString([]byte(text))
	got := ParseMany(DecodeSubscription([]byte(sub)))
	if len(got) != 2 || got[0].Name != "one" || got[1].Name != "two" {
		t.Fatalf("got %d profiles", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Error("IDs must be unique")
	}
	if n := len(ParseMany(DecodeSubscription([]byte(text)))); n != 2 {
		t.Errorf("plain subscription: got %d", n)
	}
}
