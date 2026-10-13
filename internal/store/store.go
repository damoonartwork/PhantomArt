// Package store persists profiles and settings as a JSON file.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/damoonartwork/phantomart/internal/link"
)

// Settings are user preferences.
type Settings struct {
	SocksPort   int    `json:"socksPort"`
	HTTPPort    int    `json:"httpPort"`
	SystemProxy bool   `json:"systemProxy"`
	AllowLAN    bool   `json:"allowLan"`
	BypassIran  bool   `json:"bypassIran"`
	BlockAds    bool   `json:"blockAds"`
	Fragment    bool   `json:"fragment"`
	LogLevel    string `json:"logLevel"`
}

// Subscription is a remote list of share links.
type Subscription struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Data is everything that is saved to disk.
type Data struct {
	Settings      Settings        `json:"settings"`
	Profiles      []*link.Profile `json:"profiles"`
	Subscriptions []Subscription  `json:"subscriptions"`
	SelectedID    string          `json:"selectedId"`
}

// DefaultSettings returns the settings used on first run.
func DefaultSettings() Settings {
	return Settings{
		SocksPort:   10808,
		HTTPPort:    10809,
		SystemProxy: true,
		BypassIran:  true,
		Fragment:    true,
		LogLevel:    "warning",
	}
}

// Store guards Data and writes it atomically to Path.
type Store struct {
	Path string

	mu   sync.Mutex
	data Data
}

// Open loads path, or starts with defaults if it does not exist yet.
func Open(path string) (*Store, error) {
	s := &Store{Path: path, data: Data{Settings: DefaultSettings()}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, err
	}
	return s, nil
}

// View calls fn with the current data under the lock. fn must not keep
// references past the call.
func (s *Store) View(fn func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
}

// Update calls fn under the lock and saves the result.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	return s.save()
}

// Profile returns the profile with the given ID, or nil.
func (d *Data) Profile(id string) *link.Profile {
	for _, p := range d.Profiles {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
