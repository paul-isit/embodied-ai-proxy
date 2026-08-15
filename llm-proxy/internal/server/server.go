package server

import (
	"context"
	"embodied-ai-proxy/llm-proxy/internal/config"
	"embodied-ai-proxy/shared/health"
	"embodied-ai-proxy/shared/httpserver"
	"fmt"
	"log"
	"net/http"
)

type ApplicationArgs struct {
	DataDir  string
	HTTPPort int
}

type AppServer struct {
	args ApplicationArgs
	mux  *http.ServeMux
}

func New(args ApplicationArgs) (*AppServer, error) {
	server := &AppServer{
		args: args,
		mux:  http.NewServeMux(),
	}

	if err := server.initialize(); err != nil {
		return nil, err
	}

	return server, nil
}

func (server *AppServer) initialize() error {
	dataDir := server.args.DataDir
	log.Printf("[LLMProxy] Initializing configuration with data directory: %s", dataDir)

	appConfig, err := config.Initialise(dataDir)
	if err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}

	log.Printf("[LLMProxy] Registering route: GET /health")
	server.mux.HandleFunc("/health", health.Handler("embodied-ai-proxy-llm-proxy", appConfig.Port, err))

	return nil
}

func (server *AppServer) Start(ctx context.Context) error {
	return httpserver.Run(ctx, "[LLMProxy]", fmt.Sprintf(":%d", server.args.HTTPPort), server.mux)
}
