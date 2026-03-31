// Package daemon manages the connection info file that ganoidd writes on startup
// and ganoid reads to locate the running daemon.
package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Info is written by ganoidd and read by ganoid.
type Info struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// InfoPath returns the path to the daemon connection info file.
func InfoPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "Ganoid", "daemon.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ganoid", "daemon.json")
}

// Write persists the daemon info to disk with restricted permissions.
func Write(info Info) error {
	path := InfoPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create daemon info dir: %w", err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Read loads the daemon info file. Returns an error if ganoidd is not running.
func Read() (Info, error) {
	data, err := os.ReadFile(InfoPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Info{}, fmt.Errorf("ganoidd is not running (no daemon.json found)")
		}
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("parse daemon.json: %w", err)
	}
	return info, nil
}

// Remove deletes the daemon info file. Called by ganoidd on clean shutdown.
func Remove() error {
	err := os.Remove(InfoPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
