//go:build !windows

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/yashau/ganoid/internal/logger"
)

func main() {
	port     := flag.Int("port", 57400, "HTTP port for the web UI and API")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	flag.Parse()

	initLogger(*logLevel)

	shutdown, err := startServer(*port)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdown()
}

func initLogger(level string) {
	logPath := filepath.Join(logDirPath(), "ganoidd.log")
	if err := logger.Init(logPath, logger.ParseLevel(level)); err != nil {
		log.Printf("warning: could not open log file %s: %v", logPath, err)
	}
}

func logDirPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ganoid")
}
