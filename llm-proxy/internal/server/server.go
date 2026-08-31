package server

import (
	"context"
	"embodied-ai-proxy/llm-proxy/internal/api"
	"embodied-ai-proxy/llm-proxy/internal/router"
	sharedconfig "embodied-ai-proxy/shared/config"
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

	appConfig, err := sharedconfig.Initialise(dataDir)
	if err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}

	rt, err := router.New(appConfig.Proxy.LLMConfig, http.DefaultClient)
	if err != nil {
		return fmt.Errorf("initialize provider router: %w", err)
	}
	llm := appConfig.Proxy.LLMConfig
	log.Printf("[LLMProxy] Registering route: POST /generate (provider=%s model=%s base_url=%s max_tokens=%d temperature=%v timeout=%ds)",
		llm.Provider, llm.Model, llm.BaseURL, llm.MaxTokens, llm.Temperature, llm.TimeoutSeconds)
	server.mux.HandleFunc("/generate", api.GenerateHandler(rt))

	return nil
}

func (server *AppServer) Start(ctx context.Context) error {
	return httpserver.Run(ctx, "[LLMProxy]", fmt.Sprintf(":%d", server.args.HTTPPort), server.mux)
}
