package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthAndFailClosedReadiness(t *testing.T) {
	cases := []struct {
		name, method, path string
		ready              func(context.Context) error
		status             int
		body               string
	}{
		{"alive without storage", "GET", "/healthz", nil, 200, "alive"},
		{"unwired dependencies", "GET", "/readyz", nil, 503, "not_ready"},
		{"dependencies ready", "GET", "/readyz", func(context.Context) error { return nil }, 200, "ready"},
		{"redacted dependency error", "GET", "/readyz", func(context.Context) error { return errors.New("secret-db-path") }, 503, "not_ready"},
		{"readiness deadline", "GET", "/readyz", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 503, "not_ready"},
		{"reject mutation", "POST", "/healthz", nil, 405, ""},
		{"unimplemented API", "GET", "/v1/feed-queries", nil, 404, ""},
		{"no accidental health subtree", "GET", "/healthz/extra", nil, 404, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			response := httptest.NewRecorder()
			NewHandler(tt.ready, nil).ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, nil))
			if response.Code != tt.status {
				t.Fatalf("status %d want %d", response.Code, tt.status)
			}
			if strings.Contains(response.Body.String(), "secret-db-path") {
				t.Fatal("readiness leaked private error")
			}
			if tt.body != "" {
				if response.Body.String() != "{\"status\":\""+tt.body+"\"}\n" {
					t.Fatalf("unexpected body: %s", response.Body.String())
				}
				if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json" {
					t.Fatal("health response must be uncached JSON")
				}
			}
		})
	}
}

func TestCanceledReadinessCannotReportReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	NewHandler(func(context.Context) error { return nil }, nil).ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", response.Code)
	}
}

func TestCollectionStatusIsBoundedAndRedacted(t *testing.T) {
	for _, tt := range []struct {
		name, supplied, want string
	}{
		{"disabled by default", "", "disabled"},
		{"idle", "idle", "idle"},
		{"polling", "polling", "polling"},
		{"degraded", "degraded", "degraded"},
		{"unknown is fail closed", "secret-source-error", "degraded"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var handler http.Handler
			if tt.supplied == "" {
				handler = NewHandler(nil, nil)
			} else {
				handler = NewHandler(nil, func(context.Context) string { return tt.supplied })
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/statusz", nil))
			if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\""+tt.want+"\"}\n" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
