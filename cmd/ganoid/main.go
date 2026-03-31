package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yashau/ganoid/internal/client"
	"github.com/yashau/ganoid/internal/daemon"
	"github.com/yashau/ganoid/internal/tray"
)

// Set via -ldflags at build time.
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	noBrowser  := flag.Bool("no-browser", false, "Do not open browser on start")
	noTray     := flag.Bool("no-tray", false, "Disable systray icon")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Ganoid v%s\nBuild time: %s\nGit commit: %s\n", version, buildTime, gitCommit)
		os.Exit(0)
	}

	holder := &client.Holder{}
	rebuildCh, notify := tray.NewRebuildChan()

	// Background poller: continuously watches for ganoidd.
	// Updates holder and notifies the tray whenever the connection state changes.
	go func() {
		var lastConnected bool
		var browserOpened bool

		for {
			info, err := daemon.Read()
			if err != nil {
				if lastConnected {
					holder.Set(nil)
					notify()
					lastConnected = false
				}
				time.Sleep(2 * time.Second)
				continue
			}

			c := client.New(info.Port, info.Token)

			// Verify ganoidd is actually responding.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, err = c.Status(ctx)
			cancel()

			if err != nil {
				if lastConnected {
					holder.Set(nil)
					notify()
					lastConnected = false
				}
				time.Sleep(2 * time.Second)
				continue
			}

			// Connected.
			if !lastConnected {
				holder.Set(c)
				notify()
				lastConnected = true

				if !browserOpened && !*noBrowser {
					tray.OpenBrowser(c.DashboardURL())
					browserOpened = true
				}
			}

			time.Sleep(5 * time.Second)
		}
	}()

	if !*noTray {
		tray.Run(holder, rebuildCh)
	} else {
		select {}
	}
}
