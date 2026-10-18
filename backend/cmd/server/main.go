package main

import (
	"context"
	"embodied-ai-proxy/backend/internal/eval"
	"embodied-ai-proxy/backend/internal/server"
	sharedconfig "embodied-ai-proxy/shared/config"
	"embodied-ai-proxy/shared/logging"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// getAppArgs parses the flags, returning the test suite files to run when
// -eval is given and the optional -evalJSON results file.
func getAppArgs() (server.ApplicationArgs, []string, string) {
	var (
		dataDir  string
		httpPort int
		evalMode bool
		evalJSON string
	)
	flag.StringVar(&dataDir, "dataDir", "data", "The path to the data folder for the application (config.json, json_schema.json, and system_prompt.md all live under <dataDir>/config)")
	flag.IntVar(&httpPort, "httpPort", sharedconfig.DefaultServerPort, "http server port")
	flag.BoolVar(&evalMode, "eval", false, "Run the given JSON test suites (e.g. -eval tests/*.json) through the pipeline and exit, instead of serving")
	flag.StringVar(&evalJSON, "evalJSON", "", "With -eval, also write each case's raw result to this file as JSON lines")
	flag.Parse()

	var evalFiles []string
	if evalMode {
		evalFiles = flag.Args()
		if len(evalFiles) == 0 {
			log.Fatalf("[Main] -eval needs at least one test suite file, e.g. -eval tests/basic.json")
		}
	}

	return server.ApplicationArgs{
		DataDir:  dataDir,
		HTTPPort: httpPort,
	}, evalFiles, evalJSON
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

	args, evalFiles, evalJSON := getAppArgs()
	if evalFiles != nil {
		return runEval(ctx, args, evalFiles, evalJSON)
	}

	if closer, err := logging.Setup(args.DataDir, "server"); err != nil {
		log.Printf("[Main] Failed to set up file logging: %v (continuing with stdout only)", err)
	} else {
		defer closer.Close()
	}
	log.Printf("[Main] Parsed flags: dataDir=%s, httpPort=%d", args.DataDir, args.HTTPPort)

	srv, err := server.New(args)
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

// runEval runs the test suites and returns the exit code. Logs go only to
// the eval log file, so stdout shows just the report.
func runEval(ctx context.Context, args server.ApplicationArgs, suiteFiles []string, resultsPath string) int {
	if logFile, err := logging.Open(args.DataDir, "eval"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: no eval log (%v)\n", err)
		log.SetOutput(io.Discard)
	} else {
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	srv, err := server.New(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set up the pipeline: %v\n", err)
		return 1
	}

	var results io.Writer
	if resultsPath != "" {
		resultsFile, err := os.Create(resultsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create %s: %v\n", resultsPath, err)
			return 1
		}
		defer resultsFile.Close()
		results = resultsFile
	}

	llm := srv.LLMConfig()
	if failed := eval.Run(ctx, srv.Pipeline(), llm.Provider+" · "+llm.Model, suiteFiles, os.Stdout, results); failed > 0 {
		return 1
	}
	return 0
}
