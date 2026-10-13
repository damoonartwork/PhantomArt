package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/damoonartwork/phantomart/internal/core"
	"github.com/damoonartwork/phantomart/internal/link"
	"github.com/damoonartwork/phantomart/internal/store"
	"github.com/damoonartwork/phantomart/internal/subscription"
	"github.com/damoonartwork/phantomart/internal/sysproxy"
	"github.com/damoonartwork/phantomart/internal/xray"
)

const maxLogLines = 500

// App is bound to the frontend; its exported methods are callable from JS
// as window.go.main.App.<Method>.
type App struct {
	ctx    context.Context
	store  *store.Store
	runner *core.Runner

	mu        sync.Mutex
	connected bool
	activeID  string
	proxyAddr string // system proxy we set, so we only undo our own
	logs      []string
}

// State is the snapshot the UI renders.
type State struct {
	Settings          store.Settings       `json:"settings"`
	Profiles          []*link.Profile      `json:"profiles"`
	Subscriptions     []store.Subscription `json:"subscriptions"`
	SelectedID        string               `json:"selectedId"`
	Connected         bool                 `json:"connected"`
	ActiveID          string               `json:"activeId"`
	CoreFound         bool                 `json:"coreFound"`
	CorePath          string               `json:"corePath"`
	SysProxySupported bool                 `json:"sysProxySupported"`
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = "."
	}
	dataDir := filepath.Join(cfgDir, "PhantomArt")
	a.store, err = store.Open(filepath.Join(dataDir, "data.json"))
	if err != nil {
		a.log("could not read saved data, starting fresh: " + err.Error())
		a.store, _ = store.Open(filepath.Join(dataDir, "data.reset.json"))
	}

	exe, _ := os.Executable()
	a.runner = &core.Runner{
		BinDir:  filepath.Join(filepath.Dir(exe), "core"),
		WorkDir: dataDir,
		OnLog:   a.log,
		OnExit:  a.onCoreExit,
	}
}

func (a *App) shutdown(context.Context) {
	_ = a.Disconnect()
}

// GetState returns everything the UI needs.
func (a *App) GetState() State {
	var st State
	a.store.View(func(d *store.Data) {
		st.Settings = d.Settings
		st.Profiles = append([]*link.Profile(nil), d.Profiles...)
		st.Subscriptions = append([]store.Subscription(nil), d.Subscriptions...)
		st.SelectedID = d.SelectedID
	})
	a.mu.Lock()
	st.Connected, st.ActiveID = a.connected, a.activeID
	a.mu.Unlock()
	st.CorePath = a.runner.BinaryPath()
	_, err := os.Stat(st.CorePath)
	st.CoreFound = err == nil
	st.SysProxySupported = sysproxy.Supported()
	return st
}

// ImportText adds every share link found in text and returns how many.
func (a *App) ImportText(text string) (int, error) {
	profiles := link.ParseMany(text)
	if len(profiles) == 0 {
		return 0, errors.New("no valid vless://, vmess://, trojan:// or ss:// link found")
	}
	err := a.store.Update(func(d *store.Data) error {
		d.Profiles = append(d.Profiles, profiles...)
		if d.SelectedID == "" {
			d.SelectedID = profiles[0].ID
		}
		return nil
	})
	return len(profiles), err
}

// ImportClipboard imports links from the system clipboard.
func (a *App) ImportClipboard() (int, error) {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return 0, err
	}
	return a.ImportText(text)
}

// DeleteProfile removes a profile.
func (a *App) DeleteProfile(id string) error {
	return a.store.Update(func(d *store.Data) error {
		kept := d.Profiles[:0]
		for _, p := range d.Profiles {
			if p.ID != id {
				kept = append(kept, p)
			}
		}
		d.Profiles = kept
		if d.SelectedID == id {
			d.SelectedID = ""
			if len(kept) > 0 {
				d.SelectedID = kept[0].ID
			}
		}
		return nil
	})
}

// SelectProfile marks a profile as the one to connect with. If already
// connected, it reconnects using the new profile.
func (a *App) SelectProfile(id string) error {
	if err := a.store.Update(func(d *store.Data) error {
		if d.Profile(id) == nil {
			return errors.New("profile not found")
		}
		d.SelectedID = id
		return nil
	}); err != nil {
		return err
	}
	a.mu.Lock()
	reconnect := a.connected && a.activeID != id
	a.mu.Unlock()
	if reconnect {
		return a.Connect()
	}
	return nil
}

// Connect starts xray with the selected profile and, if enabled, sets the
// system proxy.
func (a *App) Connect() error {
	var (
		p   link.Profile
		set store.Settings
		ok  bool
	)
	a.store.View(func(d *store.Data) {
		set = d.Settings
		if sel := d.Profile(d.SelectedID); sel != nil {
			p, ok = *sel, true
		}
	})
	if !ok {
		return errors.New("select a server first")
	}

	cfg, err := xray.Build(&p, xray.Options{
		SocksPort:  set.SocksPort,
		HTTPPort:   set.HTTPPort,
		AllowLAN:   set.AllowLAN,
		BypassIran: set.BypassIran,
		BlockAds:   set.BlockAds,
		Fragment:   set.Fragment,
		LogLevel:   set.LogLevel,
	})
	if err != nil {
		return err
	}
	a.log(fmt.Sprintf("connecting to %s (%s)", p.Name, p.Protocol))
	if err := a.runner.Start(cfg); err != nil {
		a.setConnected(false, "")
		a.clearSystemProxy()
		return err
	}

	a.clearSystemProxy()
	if set.SystemProxy && sysproxy.Supported() {
		addr := "127.0.0.1:" + strconv.Itoa(set.HTTPPort)
		if err := sysproxy.Enable(addr); err != nil {
			a.log("could not set system proxy: " + err.Error())
		} else {
			a.mu.Lock()
			a.proxyAddr = addr
			a.mu.Unlock()
		}
	}
	a.setConnected(true, p.ID)
	a.log("connected")
	return nil
}

// Disconnect stops xray and restores the system proxy.
func (a *App) Disconnect() error {
	a.clearSystemProxy()
	a.runner.Stop()
	a.mu.Lock()
	was := a.connected
	a.mu.Unlock()
	a.setConnected(false, "")
	if was {
		a.log("disconnected")
	}
	return nil
}

// UpdateSettings saves settings; reconnects if currently connected.
func (a *App) UpdateSettings(s store.Settings) error {
	if s.SocksPort < 1 || s.SocksPort > 65535 || s.HTTPPort < 1 || s.HTTPPort > 65535 || s.SocksPort == s.HTTPPort {
		return errors.New("invalid ports")
	}
	if err := a.store.Update(func(d *store.Data) error { d.Settings = s; return nil }); err != nil {
		return err
	}
	a.mu.Lock()
	connected := a.connected
	a.mu.Unlock()
	if connected {
		return a.Connect()
	}
	return nil
}

// AddSubscription stores a subscription URL and downloads it.
func (a *App) AddSubscription(name, rawURL string) (int, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return 0, errors.New("subscription URL must start with http:// or https://")
	}
	if err := a.store.Update(func(d *store.Data) error {
		for _, s := range d.Subscriptions {
			if s.URL == rawURL {
				return nil
			}
		}
		d.Subscriptions = append(d.Subscriptions, store.Subscription{Name: strings.TrimSpace(name), URL: rawURL})
		return nil
	}); err != nil {
		return 0, err
	}
	return a.refreshSubscription(rawURL)
}

// RemoveSubscription deletes a subscription and its profiles.
func (a *App) RemoveSubscription(rawURL string) error {
	return a.store.Update(func(d *store.Data) error {
		subs := d.Subscriptions[:0]
		for _, s := range d.Subscriptions {
			if s.URL != rawURL {
				subs = append(subs, s)
			}
		}
		d.Subscriptions = subs
		d.Profiles = withoutSubscription(d.Profiles, rawURL)
		if d.Profile(d.SelectedID) == nil {
			d.SelectedID = ""
		}
		return nil
	})
}

// UpdateSubscriptions re-downloads every subscription and returns the total
// number of profiles received.
func (a *App) UpdateSubscriptions() (int, error) {
	var urls []string
	a.store.View(func(d *store.Data) {
		for _, s := range d.Subscriptions {
			urls = append(urls, s.URL)
		}
	})
	total := 0
	var errs []error
	for _, u := range urls {
		n, err := a.refreshSubscription(u)
		total += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	return total, errors.Join(errs...)
}

func (a *App) refreshSubscription(rawURL string) (int, error) {
	// Use the tunnel when connected: subscription hosts are often blocked.
	proxy := ""
	a.mu.Lock()
	if a.connected {
		a.store.View(func(d *store.Data) { proxy = "http://127.0.0.1:" + strconv.Itoa(d.Settings.HTTPPort) })
	}
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	profiles, err := subscription.Fetch(ctx, rawURL, proxy)
	if err != nil {
		a.log("subscription update failed: " + err.Error())
		return 0, err
	}
	err = a.store.Update(func(d *store.Data) error {
		// Keep the selection if the same server is still in the list.
		var selectedRaw string
		if sel := d.Profile(d.SelectedID); sel != nil && sel.Subscription == rawURL {
			selectedRaw = sel.Raw
		}
		d.Profiles = append(withoutSubscription(d.Profiles, rawURL), profiles...)
		if selectedRaw != "" {
			d.SelectedID = ""
			for _, p := range profiles {
				if p.Raw == selectedRaw {
					d.SelectedID = p.ID
				}
			}
		}
		if d.SelectedID == "" && len(d.Profiles) > 0 {
			d.SelectedID = d.Profiles[0].ID
		}
		return nil
	})
	a.log(fmt.Sprintf("subscription updated: %d servers", len(profiles)))
	return len(profiles), err
}

// PingAll measures TCP latency to every server (ms, -1 = unreachable).
func (a *App) PingAll() map[string]int {
	var profiles []link.Profile
	a.store.View(func(d *store.Data) {
		for _, p := range d.Profiles {
			profiles = append(profiles, *p)
		}
	})
	res := make(map[string]int, len(profiles))
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, 16)
	)
	for i := range profiles {
		wg.Add(1)
		go func(p *link.Profile) {
			defer wg.Done()
			sem <- struct{}{}
			ms := subscription.TCPPing(p, 3*time.Second)
			<-sem
			mu.Lock()
			res[p.ID] = ms
			mu.Unlock()
		}(&profiles[i])
	}
	wg.Wait()
	return res
}

// TestConnection checks the active tunnel end to end and returns the delay.
func (a *App) TestConnection() (int, error) {
	a.mu.Lock()
	connected := a.connected
	a.mu.Unlock()
	if !connected {
		return -1, errors.New("not connected")
	}
	var port int
	a.store.View(func(d *store.Data) { port = d.Settings.HTTPPort })
	ctx, cancel := context.WithTimeout(a.ctx, 12*time.Second)
	defer cancel()
	return subscription.RealDelay(ctx, "http://127.0.0.1:"+strconv.Itoa(port))
}

// GetLogs returns recent log lines.
func (a *App) GetLogs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.logs...)
}

// OpenCoreFolder opens the folder where xray.exe must be placed.
func (a *App) OpenCoreFolder() {
	_ = os.MkdirAll(a.runner.BinDir, 0o755)
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(a.runner.BinDir))
}

func (a *App) onCoreExit(err error) {
	a.log(fmt.Sprintf("xray stopped unexpectedly: %v", err))
	a.clearSystemProxy()
	a.setConnected(false, "")
}

func (a *App) clearSystemProxy() {
	a.mu.Lock()
	addr := a.proxyAddr
	a.proxyAddr = ""
	a.mu.Unlock()
	if addr != "" {
		if err := sysproxy.Disable(addr); err != nil {
			a.log("could not clear system proxy: " + err.Error())
		}
	}
}

func (a *App) setConnected(connected bool, id string) {
	a.mu.Lock()
	a.connected, a.activeID = connected, id
	a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "state")
	}
}

func (a *App) log(line string) {
	line = time.Now().Format("15:04:05 ") + line
	a.mu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > maxLogLines {
		a.logs = a.logs[len(a.logs)-maxLogLines:]
	}
	a.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", line)
	}
}

func withoutSubscription(profiles []*link.Profile, subURL string) []*link.Profile {
	var out []*link.Profile
	for _, p := range profiles {
		if p.Subscription != subURL {
			out = append(out, p)
		}
	}
	return out
}
