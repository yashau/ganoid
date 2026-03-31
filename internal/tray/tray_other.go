//go:build !windows

package tray

import "github.com/yashau/ganoid/internal/client"

// Run is a no-op stub on non-Windows platforms.
func Run(_ *client.Holder, _ <-chan struct{}) {
	select {} // block forever
}
