// Package event defines shared event types used across ganoidd and ganoid.
package event

// SwitchEvent is emitted during the profile switch sequence.
type SwitchEvent struct {
	Step    int    `json:"step"`
	Total   int    `json:"total"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
	Done    bool   `json:"done"`
}
