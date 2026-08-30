package server

import (
	"context"
	"embodied-ai-proxy/backend/internal/api"
	"embodied-ai-proxy/backend/internal/pipeline"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	sharedconfig "embodied-ai-proxy/shared/config"
	"embodied-ai-proxy/shared/httpserver"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// jsonSchemaFileName and systemPromptFileName are the two backend-specific
// files that live alongside config.json under <dataDir>/config - naming them
// here keeps the actual filename in one place instead of as an inline
// string literal wherever a path gets built.
const (
	jsonSchemaFileName   = "json_schema.json"
	systemPromptFileName = "system_prompt.md"
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
	log.Printf("[Server] Initializing configuration with data directory: %s", dataDir)

	appConfig, err := sharedconfig.Initialise(dataDir)
	if err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}

	configDir := filepath.Join(server.args.DataDir, sharedconfig.ConfigDirName)

	schemaPath := filepath.Join(configDir, jsonSchemaFileName)
	schemaRaw, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read json schema %s: %w", schemaPath, err)
	}

	validtr, err := validator.New(schemaPath, schemaRaw)
	if err != nil {
		return fmt.Errorf("compile json schema %s: %w", schemaPath, err)
	}

	systemPrompt := pipeline.DefaultSystemPrompt
	systemPromptPath := filepath.Join(configDir, systemPromptFileName)
	if raw, err := os.ReadFile(systemPromptPath); err != nil {
		log.Printf("[Server] %s not found at %s, falling back to default system prompt", systemPromptFileName, systemPromptPath)
	} else {
		systemPrompt = string(raw)
	}

	hub := websocket.NewHub()
	p := pipeline.New(hub, validtr, appConfig.Server.ProxyURL, systemPrompt, schemaRaw)
	hub.SetPromptHandler(p)
	hub.SetStatusHandler(p)

	log.Printf("[Server] Registering route: GET /ws/client")
	server.mux.HandleFunc("/ws/client", hub.ServeClient)
	log.Printf("[Server] Registering route: GET /ws/bridge")
	server.mux.HandleFunc("/ws/bridge", hub.ServeBridge)
	log.Printf("[Server] Registering route: POST /api/prompt")
	server.mux.HandleFunc("/api/prompt", api.PromptHandler(p))
	log.Printf("[Server] Registering route: GET /api/info")
	server.mux.HandleFunc("/api/info", api.InfoHandler(appConfig, hub, p))

	return nil
}

// TODO implement this
//func verifyDataDir(dir string) error {
//	absDataDir, err := filepath.Abs(dir)
//	if err != nil {
//		return fmt.Errorf("get absolute path: %w", err)
//	}
//
//	executablePath, err := os.Executable()
//	if err != nil {
//		return fmt.Errorf("get path to executable file: %w", err)
//	}
//
//	return nil
//}

func (server *AppServer) Start(ctx context.Context) error {
	return httpserver.Run(ctx, "[Server]", fmt.Sprintf(":%d", server.args.HTTPPort), server.mux)
}
