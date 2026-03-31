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

const tailscaleEnvFile = "/etc/default/tailscaled"
const loginServerLine = "TS_LOGIN_SERVER"

func (l *Linux) SetLoginServer(url string) error {
	return writeEnvValue(tailscaleEnvFile, loginServerLine, url)
}

func (l *Linux) GetLoginServer() (string, error) {
	return readEnvValue(tailscaleEnvFile, loginServerLine)
}

func (l *Linux) ClearLoginServer() error {
	return deleteEnvValue(tailscaleEnvFile, loginServerLine)
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

// writeEnvValue writes KEY=VALUE to a simple env file, updating if exists.
func writeEnvValue(path, key, value string) error {
	content, _ := os.ReadFile(path)
	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func readEnvValue(path, key string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"="), nil
		}
	}
	return "", nil
}

func deleteEnvValue(path, key string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(content), "\n")
	out := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(line, key+"=") {
			out = append(out, line)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}
