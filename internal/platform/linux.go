//go:build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Linux implements Platform for Linux via systemd.
type Linux struct{}

func New() Platform {
	return &Linux{}
}

func (l *Linux) StopService() error {
	return runSystemctl("stop", "tailscaled")
}

func (l *Linux) StartService() error {
	return runSystemctl("start", "tailscaled")
}

func (l *Linux) ServiceStatus() (ServiceState, error) {
	cmd := exec.Command("systemctl", "is-active", "tailscaled")
	out, err := cmd.Output()
	if err != nil {
		// exit code 3 = inactive
		return ServiceStopped, nil
	}
	if strings.TrimSpace(string(out)) == "active" {
		return ServiceRunning, nil
	}
	return ServiceStopped, nil
}

func (l *Linux) StateDirPath() string {
	return "/var/lib/tailscale"
}

func (l *Linux) ProfileStateDirPath(profileID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ganoid", "states", profileID)
}

func (l *Linux) TailscaleBinaryPath() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	return "tailscale"
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %v: %w\n%s", args, err, out)
	}
	return nil
}

