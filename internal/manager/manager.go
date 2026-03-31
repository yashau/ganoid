package manager

import (
	"context"
	"encoding/base64"
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

		// Step 1: Stop Tailscale daemon first so state files are stable for backup
		send(1, "Stopping Tailscale daemon…")
		if err := m.plat.StopService(); err != nil {
			fail(1, fmt.Errorf("stop service: %w", err))
			return
		}

		// Step 2: Back up current state dir (before any changes, while state is clean)
		send(2, fmt.Sprintf("Backing up state for profile %q…", currentProfile.Name))
		backupDest := m.plat.ProfileStateDirPath(currentProfile.ID)
		if err := verifyState(m.plat.StateDirPath()); err != nil {
			fail(2, fmt.Errorf("live state is invalid, refusing to overwrite backup: %w", err))
			return
		}
		// Confirm the live state's ControlURL actually matches the current profile,
		// not the target — guards against backing up the wrong state.
		liveURL, err := stateControlURL(m.plat.StateDirPath())
		if err != nil {
			fail(2, fmt.Errorf("read live ControlURL: %w", err))
			return
		}
		expectedURL := currentProfile.LoginServer
		if expectedURL == "" {
			expectedURL = "https://controlplane.tailscale.com"
		}
		if liveURL != expectedURL {
			fail(2, fmt.Errorf("live state ControlURL %q does not match current profile %q (%q) — aborting to avoid corrupting backup", liveURL, currentProfile.Name, expectedURL))
			return
		}
		rotateBackup(backupDest, 3)
		if err := copyDir(m.plat.StateDirPath(), backupDest); err != nil {
			fail(2, fmt.Errorf("backup state: %w", err))
			return
		}

		// Step 3: Clear active state dir
		send(3, "Clearing active Tailscale state…")
		if err := clearDir(m.plat.StateDirPath()); err != nil {
			fail(3, fmt.Errorf("clear state: %w", err))
			return
		}

		// Step 4: Restore target profile state (if exists), trying versions if needed
		send(4, fmt.Sprintf("Restoring state for profile %q…", targetProfile.Name))
		src := m.plat.ProfileStateDirPath(targetID)
		wantURL := targetProfile.LoginServer
		if wantURL == "" {
			wantURL = "https://controlplane.tailscale.com"
		}
		restored := false
		for _, candidate := range stateVersions(src, 3) {
			if _, err := os.Stat(candidate); err != nil {
				continue
			}
			if err := verifyState(candidate); err != nil {
				continue
			}
			candidateURL, err := stateControlURL(candidate)
			if err != nil || candidateURL != wantURL {
				continue // ControlURL mismatch — skip
			}
			if err := copyDir(candidate, m.plat.StateDirPath()); err != nil {
				fail(4, fmt.Errorf("restore state: %w", err))
				return
			}
			restored = true
			break
		}
		if !restored {
			send(4, "No valid saved state for target profile — starting fresh")
		}

		// Step 5: Write login server to registry
		send(5, "Configuring login server…")
		if targetProfile.LoginServer == "" {
			if err := m.plat.ClearLoginServer(); err != nil {
				fail(5, fmt.Errorf("clear login server: %w", err))
				return
			}
		} else {
			if err := m.plat.SetLoginServer(targetProfile.LoginServer); err != nil {
				fail(5, fmt.Errorf("set login server: %w", err))
				return
			}
		}

		// Step 6: Start Tailscale daemon
		send(6, "Starting Tailscale daemon…")
		if err := m.plat.StartService(); err != nil {
			fail(6, fmt.Errorf("start service: %w", err))
			return
		}

		// Steps 7-8: placeholder sends to keep total=8 progress consistent
		send(7, "Finalizing…")
		send(8, "Updating active profile…")
		if err := m.cfg.SetActiveProfile(targetID); err != nil {
			fail(8, fmt.Errorf("update active profile: %w", err))
			return
		}

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

// stateControlURL reads the ControlURL from the active profile in a Tailscale state directory.
func stateControlURL(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "server-state.conf"))
	if err != nil {
		return "", fmt.Errorf("read server-state.conf: %w", err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	// Each profile-XXXX value is a base64-encoded JSON prefs object.
	for k, v := range state {
		if len(k) <= 8 || k[:8] != "profile-" || string(v) == "null" {
			continue
		}
		decoded, err := decodeBase64JSON(v)
		if err != nil {
			continue
		}
		var prefs struct {
			ControlURL string `json:"ControlURL"`
		}
		if err := json.Unmarshal(decoded, &prefs); err != nil {
			continue
		}
		return prefs.ControlURL, nil
	}
	return "", fmt.Errorf("no profile found in state")
}

func decodeBase64JSON(raw json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(s)
	}
	return b, err
}

// verifyState checks that a Tailscale state directory has valid, non-empty profile data.
func verifyState(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "server-state.conf"))
	if err != nil {
		return fmt.Errorf("read server-state.conf: %w", err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	for k, v := range state {
		if len(k) > 8 && k[:8] == "profile-" && string(v) != "null" {
			return nil
		}
	}
	return fmt.Errorf("no valid profile data found")
}

// stateVersions returns the base path followed by up to n rotated versions.
func stateVersions(base string, n int) []string {
	paths := make([]string, 0, n+1)
	paths = append(paths, base)
	for i := 1; i <= n; i++ {
		paths = append(paths, fmt.Sprintf("%s.v%d", base, i))
	}
	return paths
}

// rotateBackup shifts existing versioned backups down and removes the oldest.
// base.v(n) is dropped, base.v(n-1) → base.v(n), …, base → base.v1.
func rotateBackup(base string, n int) {
	os.RemoveAll(fmt.Sprintf("%s.v%d", base, n))
	for i := n - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.v%d", base, i), fmt.Sprintf("%s.v%d", base, i+1))
	}
	os.Rename(base, fmt.Sprintf("%s.v1", base))
}

// stub to suppress unused import warning during development
var _ = time.Now
