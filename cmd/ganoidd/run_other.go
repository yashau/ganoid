//go:build !windows

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := flag.Int("port", 57400, "HTTP port for the web UI and API")
	flag.Parse()

	shutdown, err := startServer(*port)
	if err != nil {
		log.Fatalf("start server: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdown()
}
