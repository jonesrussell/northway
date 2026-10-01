package app_test

import (
	"bytes"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/app"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPostgresPopulatedMigrationAPIParity(t *testing.T) {
	file := os.Getenv("NORTHWAY_TEST_POSTGRES_FILE")
	if file == "" {
		t.Skip("requires empty disposable PostgreSQL target")
	}
	raw, e := os.ReadFile(file)
	check(t, e)
	u, e := url.Parse(strings.TrimSpace(string(raw)))
	check(t, e)
	if u.Hostname() != "127.0.0.1" || u.Path != "/northway_test" {
		t.Fatal("requires local disposable northway_test")
	}
	dir := filepath.Join(t.TempDir(), "private-source")
	check(t, os.Mkdir(dir, 0700))
	path := filepath.Join(dir, "source.sqlite")
	check(t, sqlite.Migrate(t.Context(), path))
	src, e := sqlite.Open(t.Context(), path)
	check(t, e)
	defer src.Close()
	one := fixtureTenant(t, src, tenantOne, "Tenant One World")
	two := fixtureTenant(t, src, tenantTwo, "Tenant Two World")
	for _, p := range []identity.Principal{one, two} {
		check(t, src.ConfigureFeedPreferences(t.Context(), p, corpusID, feed.Preferences{Categories: []string{"world"}, Sources: []feed.SourceRule{{SourceID: corpusID, PublisherGroup: "publisher", Categories: []string{"world"}}}, PublisherCap: 2}))
	}
	// Populate a private FETDER-style observation and local synthetic grant.
	// Neither is promoted into shared publisher ownership by the import.
	seedID := "00000009-0000-4000-8000-000000000001"
	privateURL := "https://fixture.invalid/private-creator"
	check(t, src.AddCollectionSeed(t.Context(), one, ingest.CollectionSeed{ID: seedID, URL: privateURL, Title: "Private creator fixture"}))
	now := time.Now().UTC()
	check(t, src.ConfigurePoll(t.Context(), one, ingest.Policy{SourceID: seedID, URL: privateURL, Mode: "html", Approved: true, Enabled: true, RobotsUntil: now.Add(time.Hour), Interval: time.Hour, MaxBytes: 2048}))
	cl, e := src.ClaimCollection(t.Context(), one)
	check(t, e)
	check(t, src.FinishPoll(t.Context(), one, cl.ID, ingest.Result{Status: 200, Bytes: 256, Observations: []ingest.Observation{{Version: 1, AccountURL: privateURL, EvidenceURL: privateURL, OriginalURL: privateURL, Kind: "profile", Title: "Private creator fixture", Display: "link", State: "available", Method: "html-metadata-v1"}}}))
	privateBatch, e := src.CollectionBatch(t.Context(), one, 0)
	check(t, e)
	grant, _, e := identity.GenerateAgentGrant(one, identity.CollectionObservationsRead, "local migration fixture", now.Add(-time.Second), now.Add(time.Hour))
	check(t, e)
	check(t, src.CreateAgentGrant(t.Context(), one, grant))
	_, key := fixtureKey(t, src, one, identity.FeedsRead)
	_, other := fixtureKey(t, src, two, identity.FeedsRead)
	invoke := func(store *sqlite.Store, method, path, body string, secret identity.Secret) (int, map[string]any) {
		t.Helper()
		handler := httpapi.NewAPI(identity.NewService(store), query.NewService(store), feedback.NewService(store))
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+secret.Reveal())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "postgres-migration-fixture-0001")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var data map[string]any
		check(t, json.Unmarshal(rec.Body.Bytes(), &data))
		delete(data, "request_id")
		return rec.Code, data
	}
	code, before := invoke(src, http.MethodPost, "/v1/feed-queries", queryBody(), key)
	if code != 200 {
		t.Fatal("source API failed", code)
	}
	id := before["snapshot_id"].(string)
	code, before = invoke(src, http.MethodGet, "/v1/snapshots/"+id, "", key)
	if code != 200 {
		t.Fatal("source snapshot failed", code)
	}
	check(t, src.Close())
	check(t, sqlite.Migrate(t.Context(), "postgres:"+file))
	var receipt bytes.Buffer
	e = app.Execute(t.Context(), []string{"postgres", "import", "--source", path, "--database", "postgres:" + file}, os.LookupEnv, &receipt, &receipt)
	check(t, e)
	var counts map[string]int64
	check(t, json.Unmarshal(receipt.Bytes(), &counts))
	if counts["tenants"] != 2 || counts["articles"] != 2 || counts["api_keys"] != 2 || counts["query_snapshots"] != 1 || counts["collection_items"] != 1 || counts["collection_events"] != 1 || counts["agent_grants"] != 1 {
		t.Fatal("populated counts", counts)
	}
	dst, e := sqlite.Open(t.Context(), "postgres:"+file)
	check(t, e)
	defer dst.Close()
	code, after := invoke(dst, http.MethodGet, "/v1/snapshots/"+id, "", key)
	if code != 200 || !reflect.DeepEqual(before, after) {
		t.Fatal("representative snapshot API changed", code)
	}
	code, _ = invoke(dst, http.MethodGet, "/v1/snapshots/"+id, "", other)
	if code != 404 {
		t.Fatal("cross-tenant snapshot exposed", code)
	}
	afterBatch, e := dst.CollectionBatch(t.Context(), one, 0)
	check(t, e)
	if !reflect.DeepEqual(privateBatch, afterBatch) {
		t.Fatal("private observation IDs/cursor changed")
	}
	otherBatch, e := dst.CollectionBatch(t.Context(), two, 0)
	check(t, e)
	if len(otherBatch.Events) != 0 {
		t.Fatal("private observation crossed tenant")
	}
	check(t, dst.Close())
	if _, e = sqlite.ImportPostgres(t.Context(), path, file); e == nil {
		t.Fatal("nonempty target import allowed")
	}
	t.Logf("preserved populated counts: tenants=%d articles=%d keys=%d snapshots=%d; private observation/grant preserved; exact API projection and two-tenant isolation passed", counts["tenants"], counts["articles"], counts["api_keys"], counts["query_snapshots"])
}
