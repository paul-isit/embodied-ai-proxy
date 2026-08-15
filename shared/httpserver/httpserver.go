package httpserver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

const shutdownTimeout = 10 * time.Second

// Run starts an HTTP server on addr and blocks until ctx is cancelled or the
// server fails to start, then performs a graceful shutdown.
func Run(ctx context.Context, logPrefix, addr string, handler http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: handler}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("%s HTTP server listening on %s", logPrefix, addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Printf("%s Shutdown signal received", logPrefix)
		return shutdown(srv, logPrefix)
	}
}

func shutdown(srv *http.Server, logPrefix string) error {
	log.Printf("%s Initiating graceful shutdown (timeout %s)...", logPrefix, shutdownTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	log.Printf("%s Server stopped successfully", logPrefix)
	return nil
}
