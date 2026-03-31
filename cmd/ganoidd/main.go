package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/yashau/ganoid/internal/api"
	"github.com/yashau/ganoid/internal/config"
	"github.com/yashau/ganoid/internal/daemon"
	"github.com/yashau/ganoid/internal/manager"
	"github.com/yashau/ganoid/internal/platform"
)

// ui/dist is the SvelteKit build output, placed here by the Makefile.
//
//go:embed ui/dist
var uiFiles embed.FS

// Set via -ldflags at build time.
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	port := flag.Int("port", 57400, "HTTP port for the web UI and API")
	flag.Parse()

	configDir := configDirPath()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("create config dir: %v", err)
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	plat := platform.New()
	mgr := manager.New(cfg, plat, func() {})

	distFS, err := fs.Sub(uiFiles, "ui/dist")
	if err != nil {
		log.Fatalf("embed fs: %v", err)
	}

	srv := api.New(cfg, mgr, http.FS(distFS), version)

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// Write daemon info so ganoid can find us.
	if err := daemon.Write(daemon.Info{Port: *port, Token: cfg.AuthToken()}); err != nil {
		log.Fatalf("write daemon info: %v", err)
	}

	httpServer := &http.Server{Handler: srv.Handler()}

	go func() {
		log.Printf("ganoidd %s (%s, built %s) listening on http://localhost:%d", version, gitCommit, buildTime, *port)
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	// Wait for termination signal then clean up.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("ganoidd shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	_ = daemon.Remove()
}

func configDirPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("APPDATA") + `\ganoid`
	}
	home, _ := os.UserHomeDir()
	return home + "/.config/ganoid"
}
