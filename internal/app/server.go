package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite"
)

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	if err := config.Validate(); err != nil {
		return err
	}
	var checkReady func(context.Context) error
	api := httpapi.NewAPI(nil, nil, nil)
	if config.DatabasePath != "" {
		startupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		store, err := sqlite.Open(startupCtx, config.DatabasePath)
		if err != nil {
			cancel()
			return err
		}
		defer store.Close()
		version, options, err := store.Diagnostics(startupCtx)
		cancel()
		if err != nil {
			return err
		}
		logger.Info("storage ready", "sqlite_version", version, "compile_options", options)
		checkReady = store.Ready
		api = httpapi.NewAPI(identity.NewService(store), query.NewService(store), feedback.NewService(store))
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", config.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("server listening", "address", listener.Addr().String(), "version", Version, "revision", Revision)
	// Storage readiness is not source freshness or pilot readiness.
	health := httpapi.NewHandler(checkReady)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			api.ServeHTTP(w, r)
		} else {
			health.ServeHTTP(w, r)
		}
	})
	return serve(ctx, listener, config, handler, logger)
}

// serve owns the listener and waits for the HTTP serving goroutine to terminate.
func serve(ctx context.Context, listener net.Listener, config Config, handler http.Handler, logger *slog.Logger) error {
	requestCtx, cancelRequests := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRequests()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		// Serve may fail before it takes ownership of all open connections.
		_ = server.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		logger.Info("server draining")
		// Request contexts retain their values but are not canceled on the
		// first signal: active requests have the configured window to finish.
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.ShutdownTimeout)
		defer cancel()
		err := server.Shutdown(drainCtx)
		if err != nil {
			cancelRequests()
			closeErr := server.Close()
			<-result
			return fmt.Errorf("graceful shutdown: %w", errors.Join(err, closeErr))
		}
		serveErr := <-result
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve during shutdown: %w", serveErr)
		}
		logger.Info("server stopped")
		return nil
	}
}
