package server

import (
	"context"
	"embodied-ai-proxy/backend/internal/config"
	"embodied-ai-proxy/backend/internal/monitoring"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type ApplicationArgs struct {
	DataDir  string
	HTTPPort int
}

type AppServer struct {
	args       ApplicationArgs
	httpServer *http.Server
}

func New(args ApplicationArgs) (*AppServer, error) {
	mux := http.NewServeMux()
	server := &AppServer{
		args: args,
	}

	if err := server.initialize(mux); err != nil {
		return nil, err
	}

	server.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", args.HTTPPort),
		Handler: mux,
	}

	return server, nil
}

func (server *AppServer) initialize(mux *http.ServeMux) error {
	dataDir := server.args.DataDir
	log.Printf("[Server] Initializing configuration with data directory: %s", dataDir)

	appConfig, err := config.Initialise(dataDir)
	if err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}

	log.Printf("[Server] Registering route: GET /health")
	mux.HandleFunc("/health", monitoring.HealthHandler(appConfig, err))

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
	errCh := make(chan error, 1)
	go func() {
		log.Printf("[Server] HTTP server listening on %s", server.httpServer.Addr)
		if err := server.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Printf("[Server] Shutdown signal received")
		return server.Shutdown()
	}
}

func (server *AppServer) Shutdown() error {
	log.Printf("[Server] Initiating graceful shutdown (timeout 10s)...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.httpServer.Shutdown(ctx); err != nil {
		return err
	}
	log.Printf("[Server] Server stopped successfully")
	return nil
}
