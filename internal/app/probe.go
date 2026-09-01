package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

const localReadinessURL = "http://127.0.0.1:8080/readyz"

func checkLocalReadiness(ctx context.Context, address string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return errors.New("invalid local health endpoint")
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("health redirect refused")
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("local readiness request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 65))
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "{\"status\":\"ready\"}\n" {
		return errors.New("local readiness check failed")
	}
	return nil
}
