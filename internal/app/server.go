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
)

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	if err := config.Validate(); err != nil {
		return err
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", config.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("server listening", "address", listener.Addr().String(), "version", Version, "revision", Revision)
	// There is no storage/query implementation in this work package. Do not
	// signal readiness to an infra deployment that requires a usable service.
	return serve(ctx, listener, config, httpapi.NewHandler(nil), logger)
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
