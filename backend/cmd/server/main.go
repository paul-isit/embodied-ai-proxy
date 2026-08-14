package main

import (
	"context"
	"embodied-ai-proxy/backend/internal/config/constants"
	"embodied-ai-proxy/backend/internal/server"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func getAppArgs() server.ApplicationArgs {
	var (
		dataDir  string
		httpPort int
	)
	flag.StringVar(&dataDir, "dataDir", "data", "The path to the data folder for the application")
	flag.IntVar(&httpPort, "httpPort", constants.DefaultServerPort, "http server port")
	flag.Parse()

	log.Printf("[Main] Parsed flags: dataDir=%s, httpPort=%d", dataDir, httpPort)
	return server.ApplicationArgs{
		DataDir:  dataDir,
		HTTPPort: httpPort,
	}
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(
		context.TODO(),
		syscall.SIGINT,  // signal interrupt, i.e. Ctrl+C
		syscall.SIGTERM, //signal terminate, i.e. kill <pid>, etc.
		syscall.SIGHUP,  // signal hangup, i.e. terminal window or ssh session closes
		syscall.SIGQUIT, // signal quit
	)
	defer stop()

	srv, err := server.New(getAppArgs())
	if err != nil {
		log.Fatalf("Failed to start app server: %v", err)
		return 1
	}

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("App server stopped with error: %v", err)
		return 1
	}
	return 0
}
