//go:build windows

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"golang.org/x/sys/windows/svc"

	"github.com/yashau/ganoid/internal/logger"
)

func main() {
	port     := flag.Int("port", 57400, "HTTP port for the web UI and API")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	flag.Parse()

	initLogger(*logLevel)

	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("cannot determine run context: %v", err)
	}

	if isService {
		if err := svc.Run("ganoidd", &ganoidSvc{port: *port}); err != nil {
			log.Fatalf("service run failed: %v", err)
		}
		return
	}

	// Interactive / debug mode: use signal handling.
	runInteractive(*port)
}

func initLogger(level string) {
	logPath := filepath.Join(logDirPath(), "ganoidd.log")
	if err := logger.Init(logPath, logger.ParseLevel(level)); err != nil {
		log.Printf("warning: could not open log file %s: %v", logPath, err)
	}
}

func logDirPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "Ganoid")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ganoid")
}

// ganoidSvc implements the Windows Service Control Manager protocol.
type ganoidSvc struct{ port int }

func (s *ganoidSvc) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	shutdown, err := startServer(s.port)
	if err != nil {
		log.Printf("ganoidd: failed to start server: %v", err)
		return false, 1
	}

	status <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	for c := range req {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			shutdown()
			return false, 0
		default:
			log.Printf("ganoidd: unexpected service control request %d", c.Cmd)
		}
	}

	return false, 0
}

func runInteractive(port int) {
	shutdown, err := startServer(port)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdown()
}
