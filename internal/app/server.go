package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/sqlite"
)

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	if err := config.Validate(); err != nil {
		return err
	}
	var checkReady func(context.Context) error
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
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", config.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("server listening", "address", listener.Addr().String(), "version", Version, "revision", Revision)
	// Readiness covers configured storage, not the still-unimplemented feed API.
	return serve(ctx, listener, config, httpapi.NewHandler(checkReady), logger)
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
