package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
)

func testConfig() Config          { return Config{ListenAddress: "127.0.0.1:0", ShutdownTimeout: time.Second} }
func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunRejectsInvalidConfigAndBindFailure(t *testing.T) {
	ctx := context.Background()
	if err := Run(ctx, Config{}, discardLogger()); err == nil {
		t.Fatal("invalid configuration accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := testConfig()
	cfg.ListenAddress = listener.Addr().String()
	if err := Run(ctx, cfg, discardLogger()); err == nil {
		t.Fatal("bind failure was swallowed")
	}
}

func TestRunRejectsUnprovisionedPollingTenant(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "northway.sqlite")
	if err := sqlite.Migrate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	config := testConfig()
	config.DatabasePath = path
	config.PollTenant = identity.TenantID("00000000-0000-4000-8000-000000000001")
	if err := Run(t.Context(), config, discardLogger()); err == nil || !strings.Contains(err.Error(), "not provisioned") {
		t.Fatalf("error=%v", err)
	}
}

func TestRunAcceptsProvisionedPollingTenant(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "northway.sqlite")
	if err := sqlite.Migrate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	tenant := identity.TenantID("00000000-0000-4000-8000-000000000001")
	if err := store.CreateTenant(t.Context(), tenant); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	probe.Close()
	config := testConfig()
	config.ListenAddress = address
	config.DatabasePath = path
	config.PollTenant = tenant
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config, discardLogger()) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		select {
		case err := <-done:
			t.Fatalf("server stopped before listening: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not listen")
		}
		time.Sleep(10 * time.Millisecond)
	}
	statusDeadline := time.Now().Add(3 * time.Second)
	for {
		response, err := http.Get("http://" + address + "/statusz")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.StatusCode, body)
		}
		if string(body) == "{\"status\":\"idle\"}\n" {
			break
		}
		if string(body) != "{\"status\":\"polling\"}\n" {
			t.Fatalf("unexpected collection status: %s", body)
		}
		if time.Now().After(statusDeadline) {
			t.Fatal("publisher did not settle to idle")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestGracefulShutdownDrainsAnActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); cancel(); listener.Close() })
	handlerCanceled := make(chan bool, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			handlerCanceled <- false
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
			handlerCanceled <- true
		}
	})
	stopped := make(chan error, 1)
	go func() { stopped <- serve(ctx, listener, testConfig(), handler, discardLogger()) }()
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	response := make(chan error, 1)
	go func() {
		r, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			r.Body.Close()
			if r.StatusCode != http.StatusNoContent {
				err = errors.New("request failed during drain")
			}
		}
		response <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-stopped:
		t.Fatalf("shutdown did not wait for request: %v", err)
	case canceled := <-handlerCanceled:
		t.Fatalf("handler finished before release (canceled=%v)", canceled)
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-response:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not complete")
	}
	if <-handlerCanceled {
		t.Fatal("active request context canceled during graceful drain")
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestShutdownDeadlineCancelsRequestsAndReturnsFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); listener.Close() })
	entered, handlerDone := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(handlerDone) })
	cfg := testConfig()
	cfg.ShutdownTimeout = 30 * time.Millisecond // internal lifecycle test, not accepted public configuration
	stopped := make(chan error, 1)
	go func() { stopped <- serve(ctx, listener, cfg, handler, discardLogger()) }()
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		r, _ := client.Get("http://" + listener.Addr().String())
		if r != nil {
			r.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-stopped:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want deadline error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown hung")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("forced close did not cancel request")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("client connection leaked")
	}
}

func TestServeReportsListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
	err = serve(context.Background(), listener, testConfig(), http.NotFoundHandler(), discardLogger())
	if err == nil {
		t.Fatal("listener failure was swallowed")
	}
}

type backgroundFunc func(context.Context) error

func (f backgroundFunc) Run(ctx context.Context) error { return f(ctx) }

func TestBackgroundFailureStopsHTTPService(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("collector failed")
	err = runServices(t.Context(), listener, testConfig(), http.NotFoundHandler(), discardLogger(), backgroundFunc(func(context.Context) error { return want }))
	if !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestServiceShutdownWaitsForBackgroundDrain(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	entered, release := make(chan struct{}), make(chan struct{})
	background := backgroundFunc(func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() {
		done <- runServices(ctx, listener, testConfig(), http.NotFoundHandler(), discardLogger(), background)
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		t.Fatalf("returned before background drain: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("services did not stop")
	}
}
