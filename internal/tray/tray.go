package tray

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/getlantern/systray"
	"github.com/yashau/ganoid/internal/client"
	"github.com/yashau/ganoid/internal/event"
)

// Run starts the systray. It blocks until the tray quits.
// rebuildCh receives a signal whenever the client holder changes.
func Run(h *client.Holder, rebuildCh <-chan struct{}) {
	systray.Run(
		func() { onReady(h, rebuildCh) },
		func() {},
	)
}

func onReady(h *client.Holder, rebuildCh <-chan struct{}) {
	systray.SetIcon(Icon())
	systray.SetTitle("Ganoid")
	systray.SetTooltip("Ganoid — Tailscale profile manager")

	statusItem := systray.AddMenuItem("Status: connecting…", "")
	statusItem.Disable()

	systray.AddSeparator()

	// Build submenu if ganoidd is already up.
	if c := h.Get(); c != nil {
		buildSubmenu(h)
	}

	systray.AddSeparator()

	openItem := systray.AddMenuItem("Open Dashboard", "Open the Ganoid web UI")
	quitItem := systray.AddMenuItem("Quit", "Quit Ganoid")

	// Poll ganoidd for status to update the tray label.
	go func() {
		for {
			c := h.Get()
			if c == nil {
				statusItem.SetTitle("Status: ganoidd not running")
				time.Sleep(5 * time.Second)
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			status, err := c.Status(ctx)
			cancel()

			if err == nil && status != nil {
				statusItem.SetTitle(fmt.Sprintf("Status: %s (%s)",
					status.Tailscale.BackendState, status.ActiveProfile.Name))
			} else {
				statusItem.SetTitle("Status: ganoidd unreachable")
			}
			time.Sleep(10 * time.Second)
		}
	}()

	// rebuildCh fires when ganoidd connects/disconnects or profiles change.
	// Since systray doesn't support removing items, we update the status label
	// as a lightweight signal. A full menu rebuild requires restarting the tray.
	go func() {
		for range rebuildCh {
			c := h.Get()
			if c == nil {
				statusItem.SetTitle("Status: ganoidd not running")
			} else {
				statusItem.SetTitle("Status: reconnected — restart tray to refresh menu")
			}
		}
	}()

	for {
		select {
		case <-openItem.ClickedCh:
			c := h.Get()
			if c == nil {
				break
			}
			OpenBrowser(c.DashboardURL())
		case <-quitItem.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func buildSubmenu(h *client.Holder) {
	c := h.Get()
	if c == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	store, err := c.Profiles(ctx)
	cancel()
	if err != nil || store == nil {
		return
	}

	sub := systray.AddMenuItem("Switch Profile", "")

	for _, p := range store.Profiles {
		label := "  " + p.Name
		if p.ID == store.ActiveProfileID {
			label = "✓ " + p.Name
		}
		item := sub.AddSubMenuItem(label, p.LoginServer)
		if p.ID == store.ActiveProfileID {
			item.Disable()
		}

		profileID := p.ID
		go func() {
			for range item.ClickedCh {
				c := h.Get()
				if c == nil {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				done := make(chan struct{})
				c.SwitchProfile(ctx, profileID,
					func(ev event.SwitchEvent) {},
					func() { close(done) },
					func(err error) { close(done) },
				)
				<-done
				cancel()
				if c := h.Get(); c != nil {
					OpenBrowser(c.DashboardURL())
				}
			}
		}()
	}
}

// OpenBrowser opens the given URL in the default system browser.
func OpenBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

// NewRebuildChan returns a channel and a notify function.
func NewRebuildChan() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	notify := func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return ch, notify
}
