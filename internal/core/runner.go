// Package core manages the Xray-core child process.
package core

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Runner starts and stops a single xray process.
type Runner struct {
	BinDir  string            // directory holding xray(.exe), geoip.dat, geosite.dat
	WorkDir string            // directory for the generated config.json
	OnLog   func(line string) // called for every line xray prints
	OnExit  func(err error)   // called when the process exits on its own

	mu      sync.Mutex
	cmd     *exec.Cmd
	done    chan struct{}
	stopped bool
}

// BinaryPath returns the expected location of the xray executable.
func (r *Runner) BinaryPath() string {
	name := "xray"
	if runtime.GOOS == "windows" {
		name = "xray.exe"
	}
	return filepath.Join(r.BinDir, name)
}

// Running reports whether xray is currently running.
func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil
}

// Start writes config to disk and launches xray with it. Any running
// instance is stopped first.
func (r *Runner) Start(config []byte) error {
	r.Stop()

	bin := r.BinaryPath()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("xray core not found at %s", bin)
	}
	if err := os.MkdirAll(r.WorkDir, 0o700); err != nil {
		return err
	}
	cfgPath := filepath.Join(r.WorkDir, "config.json")
	if err := os.WriteFile(cfgPath, config, 0o600); err != nil {
		return err
	}

	cmd := exec.Command(bin, "run", "-c", cfgPath)
	cmd.Dir = r.BinDir
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+r.BinDir)
	hideWindow(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start xray: %w", err)
	}
	bindToParent(cmd)

	done := make(chan struct{})
	r.mu.Lock()
	r.cmd, r.done, r.stopped = cmd, done, false
	r.mu.Unlock()

	go r.pump(pr)
	exitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		pw.Close()
		r.mu.Lock()
		stopped := r.stopped
		if r.cmd == cmd {
			r.cmd = nil
		}
		r.mu.Unlock()
		close(done)
		exitCh <- err
		if !stopped && r.OnExit != nil {
			r.OnExit(err)
		}
	}()

	// xray exits almost immediately on a bad config; report that as a
	// start failure instead of a later disconnect.
	select {
	case err := <-exitCh:
		if err == nil {
			err = errors.New("exited immediately")
		}
		return fmt.Errorf("xray failed to start (%v); see logs", err)
	case <-time.After(700 * time.Millisecond):
		return nil
	}
}

// Stop terminates xray if it is running and waits for it to exit.
func (r *Runner) Stop() {
	r.mu.Lock()
	cmd, done := r.cmd, r.done
	r.stopped = true
	r.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func (r *Runner) pump(rd io.Reader) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if r.OnLog != nil {
			r.OnLog(sc.Text())
		}
	}
	_, _ = io.Copy(io.Discard, rd)
}
