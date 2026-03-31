package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/yashau/ganoid/internal/config"
	"github.com/yashau/ganoid/internal/event"
	"github.com/yashau/ganoid/internal/logger"
	"github.com/yashau/ganoid/internal/platform"
)

// TailscaleStatus is the parsed output of `tailscale status --json`.
type TailscaleStatus struct {
	BackendState string `json:"BackendState"`
	Self         *struct {
		DNSName string `json:"DNSName"`
	} `json:"Self"`
	Peer map[string]json.RawMessage `json:"Peer"`
}

// Manager orchestrates profile switching and Tailscale state queries.
type Manager struct {
	mu       sync.Mutex
	cfg      *config.Config
	plat     platform.Platform
	onChange func() // called when profiles change (for tray rebuild)
}

func New(cfg *config.Config, plat platform.Platform, onChange func()) *Manager {
	return &Manager{cfg: cfg, plat: plat, onChange: onChange}
}

// SetOnChange replaces the change notification callback.
func (m *Manager) SetOnChange(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// SwitchProfile executes the full 8-step switch sequence.
// Progress events are sent to the returned channel; the channel is closed when done.
func (m *Manager) SwitchProfile(ctx context.Context, targetID string) <-chan event.SwitchEvent {
	ch := make(chan event.SwitchEvent, 16)
	go func() {
		defer close(ch)
		m.mu.Lock()
		defer m.mu.Unlock()

		const total = 8

		send := func(step int, msg string) {
			ch <- event.SwitchEvent{Step: step, Total: total, Message: msg}
		}
		fail := func(step int, err error) {
			ch <- event.SwitchEvent{Step: step, Total: total, Error: err.Error(), Done: true}
		}

		currentProfile, ok := m.cfg.ActiveProfile()
		if !ok {
			fail(0, fmt.Errorf("no active profile found"))
			return
		}
		if currentProfile.ID == targetID {
			fail(0, fmt.Errorf("profile %q is already active", targetID))
			return
		}
		targetProfile, ok := m.cfg.GetProfile(targetID)
		if !ok {
			fail(0, fmt.Errorf("profile %q not found", targetID))
			return
		}

		logger.Info("switch started: %q -> %q", currentProfile.Name, targetProfile.Name)

		// Step 1: tailscale logout (best-effort)
		send(1, "Logging out from current coordination server…")
		if err := m.runTailscale(ctx, "logout"); err != nil {
			logger.Debug("step 1: tailscale logout: %v (ignored)", err)
		} else {
			logger.Debug("step 1: tailscale logout: ok")
		}

		// Step 2: Stop Tailscale daemon
		send(2, "Stopping Tailscale daemon…")
		logger.Debug("step 2: stopping Tailscale service")
		if err := m.plat.StopService(); err != nil {
			logger.Error("step 2: stop service failed: %v", err)
			fail(2, fmt.Errorf("stop service: %w", err))
			return
		}
		logger.Debug("step 2: Tailscale service stopped")

		// Step 3: Back up current state dir
		send(3, fmt.Sprintf("Backing up state for profile %q…", currentProfile.Name))
		backupDest := m.plat.ProfileStateDirPath(currentProfile.ID)
		logger.Debug("step 3: backing up %s -> %s", m.plat.StateDirPath(), backupDest)
		if err := copyDir(m.plat.StateDirPath(), backupDest); err != nil {
			logger.Error("step 3: backup state failed: %v", err)
			fail(3, fmt.Errorf("backup state: %w", err))
			return
		}
		logger.Debug("step 3: backup ok")

		// Step 4: Clear active state dir
		send(4, "Clearing active Tailscale state…")
		logger.Debug("step 4: clearing %s", m.plat.StateDirPath())
		if err := clearDir(m.plat.StateDirPath()); err != nil {
			logger.Error("step 4: clear state failed: %v", err)
			fail(4, fmt.Errorf("clear state: %w", err))
			return
		}
		logger.Debug("step 4: clear ok")

		// Step 5: Restore target profile state (if exists)
		send(5, fmt.Sprintf("Restoring state for profile %q…", targetProfile.Name))
		src := m.plat.ProfileStateDirPath(targetID)
		if _, err := os.Stat(src); err == nil {
			logger.Debug("step 5: restoring %s -> %s", src, m.plat.StateDirPath())
			if err := copyDir(src, m.plat.StateDirPath()); err != nil {
				logger.Error("step 5: restore state failed: %v", err)
				fail(5, fmt.Errorf("restore state: %w", err))
				return
			}
			logger.Debug("step 5: restore ok")
		} else {
			logger.Debug("step 5: no saved state for %q, starting fresh", targetProfile.Name)
			send(5, "No saved state for target profile — starting fresh")
		}

		// Step 6: Write login server
		send(6, "Configuring login server…")
		logger.Debug("step 6: setting login server to %q", targetProfile.LoginServer)
		if targetProfile.LoginServer == "" {
			if err := m.plat.ClearLoginServer(); err != nil {
				logger.Error("step 6: clear login server failed: %v", err)
				fail(6, fmt.Errorf("clear login server: %w", err))
				return
			}
		} else {
			if err := m.plat.SetLoginServer(targetProfile.LoginServer); err != nil {
				logger.Error("step 6: set login server failed: %v", err)
				fail(6, fmt.Errorf("set login server: %w", err))
				return
			}
		}
		logger.Debug("step 6: login server configured")

		// Step 7: Start Tailscale daemon
		send(7, "Starting Tailscale daemon…")
		logger.Debug("step 7: starting Tailscale service")
		if err := m.plat.StartService(); err != nil {
			logger.Error("step 7: start service failed: %v", err)
			fail(7, fmt.Errorf("start service: %w", err))
			return
		}
		logger.Debug("step 7: Tailscale service started")

		// Step 8: Update active profile in config
		send(8, "Updating active profile…")
		logger.Debug("step 8: setting active profile to %q", targetID)
		if err := m.cfg.SetActiveProfile(targetID); err != nil {
			logger.Error("step 8: update active profile failed: %v", err)
			fail(8, fmt.Errorf("update active profile: %w", err))
			return
		}
		logger.Info("switch complete: active profile is now %q", targetProfile.Name)

		ch <- event.SwitchEvent{Step: total, Total: total, Message: "Switch complete", Done: true}

		if m.onChange != nil {
			m.onChange()
		}
	}()
	return ch
}

// ActualControlURL returns the ControlURL Tailscale is currently using,
// parsed from `tailscale debug prefs`. Returns "" for the official server.
func (m *Manager) ActualControlURL(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, m.plat.TailscaleBinaryPath(), "debug", "prefs")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tailscale debug prefs: %w", err)
	}
	var prefs struct {
		ControlURL string `json:"ControlURL"`
	}
	if err := json.Unmarshal(out, &prefs); err != nil {
		return "", fmt.Errorf("parse prefs: %w", err)
	}
	// Official Tailscale server — treat as empty to match profiles with no login server set.
	if prefs.ControlURL == "https://controlplane.tailscale.com" {
		return "", nil
	}
	return prefs.ControlURL, nil
}

// TailscaleStatus queries `tailscale status --json`.
func (m *Manager) TailscaleStatus(ctx context.Context) (*TailscaleStatus, error) {
	cmd := exec.CommandContext(ctx, m.plat.TailscaleBinaryPath(), "status", "--json")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			// Binary not found — Tailscale is not installed.
			return &TailscaleStatus{BackendState: "Not installed"}, nil
		}
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	var s TailscaleStatus
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, fmt.Errorf("parse tailscale status: %w", err)
	}
	return &s, nil
}

func (m *Manager) runTailscale(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, m.plat.TailscaleBinaryPath(), args...)
	return cmd.Run()
}

// copyDir recursively copies src to dst, creating dst if needed.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// clearDir removes all contents of dir without removing dir itself.
func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// PeerCount returns the number of peers from a TailscaleStatus.
func PeerCount(s *TailscaleStatus) int {
	if s == nil {
		return 0
	}
	return len(s.Peer)
}

// BackendState returns the BackendState string, or "Unknown" if nil.
func BackendState(s *TailscaleStatus) string {
	if s == nil {
		return "Unknown"
	}
	return s.BackendState
}

// stub to suppress unused import warning during development
var _ = time.Now
