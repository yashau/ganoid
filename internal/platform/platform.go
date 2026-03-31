package platform

// ServiceState represents the running state of the Tailscale daemon.
type ServiceState int

const (
	ServiceStopped ServiceState = iota
	ServiceRunning
	ServiceUnknown
)

func (s ServiceState) String() string {
	switch s {
	case ServiceStopped:
		return "stopped"
	case ServiceRunning:
		return "running"
	default:
		return "unknown"
	}
}

// Platform abstracts all OS-specific operations needed to manage Tailscale.
type Platform interface {
	// Service lifecycle
	StopService() error
	StartService() error
	ServiceStatus() (ServiceState, error)

	// State directory paths
	StateDirPath() string
	ProfileStateDirPath(profileID string) string

	// Tailscale CLI
	TailscaleBinaryPath() string
}
