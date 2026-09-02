package main

import (
	"context"
	"embodied-ai-proxy/llm-proxy/internal/server"
	sharedconfig "embodied-ai-proxy/shared/config"
	"embodied-ai-proxy/shared/logging"
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
	flag.IntVar(&httpPort, "httpPort", sharedconfig.DefaultProxyPort, "http server port")
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

	args := getAppArgs()
	if closer, err := logging.Setup(args.DataDir, "proxy"); err != nil {
		log.Printf("[Main] Failed to set up file logging: %v (continuing with stdout only)", err)
	} else {
		defer closer.Close()
	}

	srv, err := server.New(args)
	if err != nil {
		log.Fatalf("Failed to start LLM proxy server: %v", err)
		return 1
	}

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("LLM proxy server stopped with error: %v", err)
		return 1
	}
	return 0
}
