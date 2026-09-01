package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/httpapi"
)

func TestCheckLocalReadinessMatchesProductionHandler(t *testing.T) {
	server := httptest.NewServer(httpapi.NewHandler(func(context.Context) error { return nil }, nil))
	defer server.Close()
	if err := checkLocalReadiness(t.Context(), server.URL+"/readyz"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckLocalReadinessAcceptsOnlyExactBoundedResponse(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
		ok   bool
	}{
		{"ready", http.StatusOK, "{\"status\":\"ready\"}\n", true},
		{"not ready", http.StatusServiceUnavailable, "{\"status\":\"not_ready\"}\n", false},
		{"wrong body", http.StatusOK, "private details", false},
		{"oversized", http.StatusOK, string(make([]byte, 100)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.code)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			err := checkLocalReadiness(t.Context(), server.URL)
			if (err == nil) != test.ok {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCheckLocalReadinessRefusesRedirectAndHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/readyz", http.StatusFound)
	}))
	defer server.Close()
	if err := checkLocalReadiness(t.Context(), server.URL); err == nil {
		t.Fatal("redirect accepted")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()
	if err := checkLocalReadiness(ctx, server.URL); err == nil {
		t.Fatal("canceled readiness accepted")
	}
}
