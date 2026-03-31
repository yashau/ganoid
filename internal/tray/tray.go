package tray

import (
	"context"
	"fmt"
	"time"

	"github.com/getlantern/systray"
	"github.com/yashau/ganoid/internal/client"
	"github.com/yashau/ganoid/internal/event"
)

// Run starts the systray in a loop, restarting whenever rebuildCh fires.
// Blocks until the user clicks Quit.
func Run(h *client.Holder, rebuildCh <-chan struct{}) {
	for {
		userQuit := make(chan struct{})
		systray.Run(
			func() { onReady(h, rebuildCh, userQuit) },
			func() {},
		)
		// If the user clicked Quit, userQuit is closed — stop the loop.
		select {
		case <-userQuit:
			return
		default:
			// Rebuild triggered — restart the tray.
		}
	}
}

func onReady(h *client.Holder, rebuildCh <-chan struct{}, userQuit chan struct{}) {
	systray.SetIcon(Icon())
	systray.SetTitle("Ganoid")
	systray.SetTooltip("Ganoid — Tailscale profile manager")

	statusItem := systray.AddMenuItem("Status: connecting…", "")
	statusItem.Disable()

	systray.AddSeparator()
	buildSubmenu(h)
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
				state := status.Tailscale.BackendState
				if state == "Not installed" {
					statusItem.SetTitle("Status: Tailscale not installed")
				} else {
					statusItem.SetTitle(fmt.Sprintf("Status: %s (%s)", state, status.ActiveProfile.Name))
				}
			} else {
				statusItem.SetTitle("Status: ganoidd unreachable")
			}
			time.Sleep(10 * time.Second)
		}
	}()

	// Rebuild tray when profiles change.
	go func() {
		for range rebuildCh {
			systray.Quit()
			return
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
			close(userQuit)
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
			}
		}()
	}
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
