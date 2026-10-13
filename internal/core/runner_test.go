package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/damoonartwork/phantomart/internal/link"
	"github.com/damoonartwork/phantomart/internal/xray"
)

// Integration tests against a real Xray binary. Run with
// PHANTOMART_XRAY_DIR=/path/to/dir/with/xray go test ./internal/core
func xrayDir(t *testing.T) string {
	dir := os.Getenv("PHANTOMART_XRAY_DIR")
	if dir == "" {
		t.Skip("PHANTOMART_XRAY_DIR not set")
	}
	return dir
}

var sampleLinks = []string{
	"vless://11111111-2222-3333-4444-555555555555@example.com:443?type=tcp&security=reality&sni=www.speedtest.net&fp=chrome&pbk=SbVKOEMjK0sIlbwg4akyBg5mL5KZwwB-ed4eEE7YnRc&sid=ab12&flow=xtls-rprx-vision#reality",
	"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=ws&security=tls&host=cdn.example.com&path=%2Fws#ws",
	"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=xhttp&security=tls&path=%2Fx&mode=auto#xhttp",
	"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:80?type=httpupgrade&path=%2Fh#hu",
	"trojan://pw@t.example.com:443?type=grpc&serviceName=svc&mode=multi#grpc",
	"ss://aes-256-gcm:pw@5.6.7.8:8388#ss",
}

func TestXrayAcceptsGeneratedConfigs(t *testing.T) {
	dir := xrayDir(t)
	for _, raw := range sampleLinks {
		p, err := link.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := xray.Build(p, xray.Options{SocksPort: 20808, HTTPPort: 20809, BypassIran: true, BlockAds: true, Fragment: true})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, cfg, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(filepath.Join(dir, "xray"), "run", "-test", "-c", path)
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: xray rejected config: %v\n%s\n%s", p.Name, err, out, cfg)
		}
	}
}

func TestRunnerStartStop(t *testing.T) {
	dir := xrayDir(t)
	var (
		mu    sync.Mutex
		lines []string
	)
	r := &Runner{
		BinDir:  dir,
		WorkDir: t.TempDir(),
		OnLog:   func(l string) { mu.Lock(); lines = append(lines, l); mu.Unlock() },
		OnExit:  func(err error) { t.Errorf("unexpected exit: %v", err) },
	}

	p, _ := link.Parse(sampleLinks[0])
	cfg, _ := xray.Build(p, xray.Options{SocksPort: 20818, HTTPPort: 20819})
	if err := r.Start(cfg); err != nil {
		t.Fatal(err)
	}
	if !r.Running() {
		t.Fatal("not running after Start")
	}
	r.Stop()
	if r.Running() {
		t.Fatal("still running after Stop")
	}

	// An invalid config must surface as a start error, not a silent exit.
	r.OnExit = nil
	if err := r.Start([]byte(`{"outbounds":[{"protocol":"nope"}]}`)); err == nil {
		t.Fatal("expected error for invalid config")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(lines, "\n"), "Xray") {
		t.Errorf("expected xray output in logs, got %q", lines)
	}
}
