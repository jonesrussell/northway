package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
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
