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
	"github.com/jonesrussell/northway/internal/fetch"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/schedule"
	"github.com/jonesrussell/northway/internal/sqlite"
)

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	if err := config.Validate(); err != nil {
		return err
	}
	var checkReady func(context.Context) error
	collectionStatus := func() string { return "disabled" }
	var publisher *schedule.Publisher
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
		if config.PollTenant != "" {
			principal, err := identity.Operator(config.PollTenant)
			if err != nil {
				return errors.New("invalid publisher polling identity")
			}
			pollCtx, pollCancel := context.WithTimeout(ctx, 5*time.Second)
			err = store.RequireTenant(pollCtx, principal)
			pollCancel()
			if errors.Is(err, sqlite.ErrNotFound) {
				return errors.New("publisher polling tenant is not provisioned")
			}
			if err != nil {
				return fmt.Errorf("verify publisher polling tenant: %w", err)
			}
			publisher = schedule.NewPublisher(ingest.New(store, fetch.New()), principal, logger)
			collectionStatus = publisher.Status
		}
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", config.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("server listening", "address", listener.Addr().String(), "version", Version, "revision", Revision)
	// Storage readiness is not source freshness or pilot readiness.
	health := httpapi.NewHandler(checkReady, collectionStatus)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			api.ServeHTTP(w, r)
		} else {
			health.ServeHTTP(w, r)
		}
	})
	if publisher == nil {
		return serve(ctx, listener, config, handler, logger)
	}
	return runServices(ctx, listener, config, handler, logger, publisher)
}

type backgroundService interface {
	Run(context.Context) error
}

// runServices gives HTTP and collection one cancellation owner. A configured
// collector failure stops the process instead of silently leaving stale feeds.
func runServices(ctx context.Context, listener net.Listener, config Config, handler http.Handler, logger *slog.Logger, background backgroundService) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	serverResult := make(chan error, 1)
	backgroundResult := make(chan error, 1)
	go func() { serverResult <- serve(runCtx, listener, config, handler, logger) }()
	go func() { backgroundResult <- background.Run(runCtx) }()

	select {
	case serverErr := <-serverResult:
		cancel()
		return errors.Join(serverErr, <-backgroundResult)
	case backgroundErr := <-backgroundResult:
		if backgroundErr == nil && ctx.Err() == nil {
			backgroundErr = errors.New("publisher scheduler stopped unexpectedly")
		}
		cancel()
		return errors.Join(backgroundErr, <-serverResult)
	}
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
