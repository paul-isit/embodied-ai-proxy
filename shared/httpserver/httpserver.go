package httpserver

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

const (
	shutdownTimeout = 10 * time.Second

	// readHeaderTimeout bounds how long a client has to finish sending
	// request headers - the classic Slowloris mitigation. It's the one
	// timeout safe to set unconditionally here: it only applies before a
	// handler runs, so it can't interfere with a handler (e.g. the backend's
	// WebSocket hub) that hijacks the connection for a long-lived session.
	readHeaderTimeout = 5 * time.Second

	// idleTimeout bounds how long a keep-alive connection may sit idle
	// between requests. Also safe unconditionally: a hijacked connection is
	// no longer managed by the idle-connection machinery this governs.
	idleTimeout = 120 * time.Second
)

// Run starts an HTTP server on addr and blocks until ctx is cancelled or the
// server fails to start, then performs a graceful shutdown.
//
// Deliberately not set: ReadTimeout/WriteTimeout. Both are enforced by
// setting a deadline on the underlying connection before the handler runs,
// and that deadline is NOT cleared when a handler hijacks the connection
// (as the backend's WebSocket hub does for /ws/client and /ws/bridge) - so
// setting either here would silently force-close every TUI/bridge
// connection once the deadline elapsed, regardless of whether it was still
// in active use. Revisit alongside adding read/write deadlines and
// ping/pong keepalives to the hub itself, not in isolation here.
func Run(ctx context.Context, logPrefix, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

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
