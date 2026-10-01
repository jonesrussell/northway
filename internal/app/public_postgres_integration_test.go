package app_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/sqlite"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type sharedHTTPFetcher struct{}

func (sharedHTTPFetcher) Fetch(context.Context, ingest.Claim) ingest.Result {
	return ingest.Result{Status: 200, Bytes: 128, Items: []ingest.Item{{OriginID: "http-shared", URL: "https://synthetic.invalid/article", Title: "Synthetic shared headline"}}}
}

func TestPostgresPublicHTTPWorkspaceKeyAndSnapshotIsolation(t *testing.T) {
	file := os.Getenv("NORTHWAY_TEST_POSTGRES_FILE")
	if file == "" {
		t.Skip("requires disposable PostgreSQL fixture")
	}
	raw, e := os.ReadFile(file)
	check(t, e)
	u, e := url.Parse(strings.TrimSpace(string(raw)))
	check(t, e)
	if u.Hostname() != "127.0.0.1" || u.Path != "/northway_test" {
		t.Fatal("requires local disposable northway_test")
	}
	check(t, sqlite.Migrate(t.Context(), "postgres:"+file))
	s, e := sqlite.Open(t.Context(), "postgres:"+file)
	check(t, e)
	defer s.Close()
	_, e = s.ConfigurePublicSource(t.Context(), sqlite.PublicDefinition{URL: "https://synthetic.invalid/feed", Title: "HTTP Publisher", Publisher: "HTTP Publisher", Topic: "world", Language: "en", Provenance: "synthetic local HTTP fixture", ReviewState: "approved", RightsBasis: "reviewed_metadata", Enabled: true, Onboarding: true, Interval: 24 * time.Hour, MaxBytes: 2048})
	check(t, e)
	_, e = s.RunPublicPollOnce(t.Context(), sharedHTTPFetcher{})
	check(t, e)
	_, key, e := ed25519.GenerateKey(nil)
	check(t, e)
	f := &betaFixture{s: s, key: key}
	f.bind(t)
	tenants := []string{"00000000-0000-4000-8000-000000000031", "00000000-0000-4000-8000-000000000032"}
	secrets := []string{}
	keyIDs := []string{}
	snapshots := []string{}
	articles := []string{}
	for n, tenant := range tenants {
		for range 2 {
			status(t, f.call(tenant, "PUT", "/v1/workspace", ""), 200)
		}
		feeds := f.call(tenant, "GET", "/v1/feeds", "")
		status(t, feeds, 200)
		if !strings.Contains(feeds.Body.String(), sqlite.PublicFeedID("world")) {
			t.Fatal("shared feed missing")
		}
		issued := f.call(tenant, "POST", "/v1/keys", `{"scopes":"feeds:read"}`)
		status(t, issued, 201)
		var result struct {
			Key    identity.KeyMetadata `json:"key"`
			Secret string               `json:"secret"`
		}
		check(t, json.Unmarshal(issued.Body.Bytes(), &result))
		secrets = append(secrets, result.Secret)
		keyIDs = append(keyIDs, result.Key.ID)
		req := httptest.NewRequest("POST", "/v1/feed-queries", strings.NewReader(fmt.Sprintf(`{"feed_id":%q,"context":{"intent":"recent news","technologies":[]},"max_age_hours":24,"limit":5}`, sqlite.PublicFeedID("world"))))
		req.Header.Set("Authorization", "Bearer "+result.Secret)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("shared-http-tenant-key-%d", n))
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		status(t, rec, 200)
		var snapshot struct {
			ID    string `json:"snapshot_id"`
			Items []struct {
				ID string `json:"article_id"`
			} `json:"items"`
		}
		check(t, json.Unmarshal(rec.Body.Bytes(), &snapshot))
		if len(snapshot.Items) != 1 {
			t.Fatal("shared article missing")
		}
		snapshots = append(snapshots, snapshot.ID)
		articles = append(articles, snapshot.Items[0].ID)
	}
	if snapshots[0] == snapshots[1] || articles[0] != articles[1] {
		t.Fatal("HTTP shared/private identity failure")
	}
	status(t, f.request("GET", "/v1/snapshots/"+snapshots[0], "", secrets[1]), 404)
	status(t, f.call(tenants[0], "DELETE", "/v1/keys/"+keyIDs[0], ""), 204)
	status(t, f.request("GET", "/v1/snapshots/"+snapshots[0], "", secrets[0]), 401)
	status(t, f.request("GET", "/v1/snapshots/"+snapshots[1], "", secrets[1]), 200)
	t.Log("HTTP onboarding retries, distinct keys/snapshots, shared article, cross-tenant denial and key revocation passed")
}
