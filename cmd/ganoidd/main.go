package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/yashau/ganoid/internal/api"
	"github.com/yashau/ganoid/internal/config"
	"github.com/yashau/ganoid/internal/daemon"
	"github.com/yashau/ganoid/internal/manager"
	"github.com/yashau/ganoid/internal/platform"
)

// ui/dist is the SvelteKit build output, placed here by the Makefile.
//
//go:embed all:ui/dist
var uiFiles embed.FS

// Set via -ldflags at build time.
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// startServer initialises and starts the HTTP server on the given port.
// It returns a shutdown function that gracefully stops the server and
// cleans up daemon.json. The caller is responsible for invoking it.
func startServer(port int) (shutdown func(), err error) {
	configDir := configDirPath()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	plat := platform.New()
	mgr := manager.New(cfg, plat, func() {})

	// Reconcile: if Tailscale's actual ControlURL doesn't match the profile
	// Ganoid thinks is active, find or create the matching profile.
	if actualURL, err := mgr.ActualControlURL(context.Background()); err == nil {
		activeProfile, _ := cfg.ActiveProfile()
		if activeProfile.LoginServer != actualURL {
			store := cfg.GetStore()
			matched := false
			for _, p := range store.Profiles {
				if p.LoginServer == actualURL {
					_ = cfg.SetActiveProfile(p.ID)
					matched = true
					break
				}
			}
			if !matched && actualURL != "" {
				// No profile exists for this login server — create one.
				host := actualURL
				if u, err := url.Parse(actualURL); err == nil && u.Host != "" {
					host = u.Host
				}
				id := fmt.Sprintf("auto-%d", time.Now().UnixMilli())
				p := config.Profile{
					ID:          id,
					Name:        host,
					LoginServer: actualURL,
					CreatedAt:   time.Now().UTC(),
					LastUsed:    time.Now().UTC(),
				}
				if err := cfg.AddProfile(p); err == nil {
					_ = cfg.SetActiveProfile(id)
				}
			}
		}
	}

	distFS, err := fs.Sub(uiFiles, "ui/dist")
	if err != nil {
		return nil, fmt.Errorf("embed fs: %w", err)
	}

	srv := api.New(cfg, mgr, http.FS(distFS), version)

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on port %d: %w", port, err)
	}

	if err := daemon.Write(daemon.Info{Port: port, Token: cfg.AuthToken()}); err != nil {
		listener.Close()
		return nil, fmt.Errorf("write daemon info: %w", err)
	}

	httpServer := &http.Server{Handler: srv.Handler()}

	go func() {
		log.Printf("ganoidd %s (%s, built %s) listening on http://localhost:%d",
			version, gitCommit, buildTime, port)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
		}
	}()

	shutdown = func() {
		log.Println("ganoidd shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
		_ = daemon.Remove()
	}

	return shutdown, nil
}

func configDirPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("APPDATA") + `\ganoid`
	}
	home, _ := os.UserHomeDir()
	return home + "/.config/ganoid"
}
