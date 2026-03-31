//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const tailscalePlistDomain = "/Library/Preferences/io.tailscale.ipn.macos"
const tailscaleLaunchdLabel = "com.tailscale.ipn.macos"

// Darwin implements Platform for macOS via launchctl.
type Darwin struct{}

func New() Platform {
	return &Darwin{}
}

func (d *Darwin) StopService() error {
	return runLaunchctl("stop", tailscaleLaunchdLabel)
}

func (d *Darwin) StartService() error {
	return runLaunchctl("start", tailscaleLaunchdLabel)
}

func (d *Darwin) ServiceStatus() (ServiceState, error) {
	cmd := exec.Command("launchctl", "list", tailscaleLaunchdLabel)
	_, err := cmd.Output()
	if err != nil {
		return ServiceStopped, nil
	}
	return ServiceRunning, nil
}

func (d *Darwin) StateDirPath() string {
	return "/Library/Tailscale"
}

func (d *Darwin) ProfileStateDirPath(profileID string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ganoid", "states", profileID)
}

func (d *Darwin) TailscaleBinaryPath() string {
	candidates := []string{
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
		"/usr/local/bin/tailscale",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	return "tailscale"
}

func runLaunchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %v: %w\n%s", args, err, out)
	}
	return nil
}
