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
		select {
		case <-userQuit:
			return
		default:
			// rebuild triggered — restart
		}
	}
}

func onReady(h *client.Holder, rebuildCh <-chan struct{}, userQuit chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())

	systray.SetIcon(Icon())
	systray.SetTitle("Ganoid")
	systray.SetTooltip("Ganoid — Tailscale profile manager")

	statusItem := systray.AddMenuItem("Status: connecting…", "")
	statusItem.Disable()

	systray.AddSeparator()

	rebuild := func() {
		cancel()
		systray.Quit()
	}
	buildSubmenu(h, ctx, rebuild)

	systray.AddSeparator()

	openItem := systray.AddMenuItem("Open Dashboard", "Open the Ganoid web UI")
	quitItem := systray.AddMenuItem("Quit", "Quit Ganoid")

	// Status poller.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			c := h.Get()
			if c == nil {
				statusItem.SetTitle("Status: ganoidd not running")
			} else {
				reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
				status, err := c.Status(reqCtx)
				reqCancel()
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
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()

	// Rebuild on profile changes from ganoidd.
	go func() {
		select {
		case <-rebuildCh:
			rebuild()
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-openItem.ClickedCh:
			c := h.Get()
			if c == nil {
				break
			}
			OpenBrowser(c.DashboardURL())
		case <-quitItem.ClickedCh:
			cancel()
			close(userQuit)
			systray.Quit()
			return
		}
	}
}

func buildSubmenu(h *client.Holder, ctx context.Context, rebuild func()) {
	c := h.Get()
	if c == nil {
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	store, err := c.Profiles(reqCtx)
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
			continue
		}

		profileID := p.ID
		go func() {
			select {
			case <-item.ClickedCh:
				c := h.Get()
				if c == nil {
					return
				}
				switchCtx, switchCancel := context.WithTimeout(ctx, 3*time.Minute)
				done := make(chan struct{})
				c.SwitchProfile(switchCtx, profileID,
					func(ev event.SwitchEvent) {},
					func() { close(done) },
					func(err error) { close(done) },
				)
				select {
				case <-done:
				case <-ctx.Done():
					switchCancel()
					return
				}
				switchCancel()
				rebuild()
			case <-ctx.Done():
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
