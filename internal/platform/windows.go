//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	tailscaleServiceName = "Tailscale"
	tailscaleRegKey      = `SOFTWARE\Tailscale IPN`
	tailscaleRegValue    = "LoginServer"
)

// Windows implements Platform for Windows.
type Windows struct{}

func New() Platform {
	return &Windows{}
}

func (w *Windows) StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(tailscaleServiceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	// Check current state before sending stop — if already stopped, nothing to do.
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("query service status: %w", err)
	}
	if status.State == svc.Stopped {
		return nil
	}

	status, err = s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("send stop control: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for service to stop")
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("query service status: %w", err)
		}
	}
	return nil
}

func (w *Windows) StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(tailscaleServiceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for service to start")
		}
		time.Sleep(300 * time.Millisecond)
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("query service status: %w", err)
		}
		if status.State == svc.Running {
			return nil
		}
	}
}

func (w *Windows) ServiceStatus() (ServiceState, error) {
	m, err := mgr.Connect()
	if err != nil {
		return ServiceUnknown, fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(tailscaleServiceName)
	if err != nil {
		return ServiceUnknown, fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return ServiceUnknown, fmt.Errorf("query service: %w", err)
	}

	switch status.State {
	case svc.Running:
		return ServiceRunning, nil
	case svc.Stopped:
		return ServiceStopped, nil
	default:
		return ServiceUnknown, nil
	}
}

func (w *Windows) StateDirPath() string {
	return filepath.Join(os.Getenv("ProgramData"), "Tailscale")
}

func (w *Windows) ProfileStateDirPath(profileID string) string {
	appdata := os.Getenv("APPDATA")
	return filepath.Join(appdata, "ganoid", "states", profileID)
}

func (w *Windows) SetLoginServer(url string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, tailscaleRegKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(tailscaleRegValue, url); err != nil {
		return fmt.Errorf("set registry value: %w", err)
	}
	return nil
}

func (w *Windows) GetLoginServer() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tailscaleRegKey, registry.QUERY_VALUE)
	if err != nil {
		// Key doesn't exist = official Tailscale (no custom login server)
		return "", nil
	}
	defer k.Close()

	val, _, err := k.GetStringValue(tailscaleRegValue)
	if err != nil {
		return "", nil
	}
	return val, nil
}

func (w *Windows) ClearLoginServer() error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tailscaleRegKey, registry.SET_VALUE)
	if err != nil {
		// Key doesn't exist, nothing to clear
		return nil
	}
	defer k.Close()
	err = k.DeleteValue(tailscaleRegValue)
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete registry value: %w", err)
	}
	return nil
}

func (w *Windows) TailscaleBinaryPath() string {
	// Tailscale CLI is typically in ProgramFiles
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Tailscale", "tailscale.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Tailscale", "tailscale.exe"),
		"tailscale.exe", // fallback: must be in PATH
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "tailscale.exe"
}
