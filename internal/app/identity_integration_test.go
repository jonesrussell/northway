package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/app"
	"github.com/jonesrussell/northway/internal/article"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/source"
	"github.com/jonesrussell/northway/internal/sqlite"
)

const tenantOne identity.TenantID = "00000001-0000-4000-8000-000000000000"
const tenantTwo identity.TenantID = "00000002-0000-4000-8000-000000000000"
const corpusID = "00000003-0000-4000-8000-000000000000"
const privateID = "00000004-0000-4000-8000-000000000000"

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func privateDirectory(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	check(t, os.Chmod(p, 0700))
	return p
}
func testStore(t *testing.T) *sqlite.Store {
	t.Helper()
	path := filepath.Join(privateDirectory(t), "test.sqlite")
	check(t, sqlite.Migrate(t.Context(), path))
	s, err := sqlite.Open(t.Context(), path)
	check(t, err)
	t.Cleanup(func() { check(t, s.Close()) })
	return s
}
func fixtureTenant(t *testing.T, s *sqlite.Store, tenant identity.TenantID, title string) identity.Principal {
	t.Helper()
	p, err := identity.Operator(tenant)
	check(t, err)
	check(t, s.CreateTenant(t.Context(), tenant))
	check(t, s.CreateSource(t.Context(), p, source.Source{ID: corpusID, Title: title, URL: "https://example.invalid/feed"}))
	check(t, s.CreateFeed(t.Context(), p, feed.Feed{ID: corpusID, Title: title}))
	check(t, s.AttachSource(t.Context(), p, corpusID, corpusID))
	check(t, s.PutArticle(t.Context(), p, article.Article{ID: corpusID, SourceID: corpusID, OriginID: "fixture", URL: "https://example.invalid/item", Title: title, Body: "PHP fixture", ObservedAt: time.Now()}))
	return p
}
func fixtureKey(t *testing.T, s *sqlite.Store, p identity.Principal, scopes identity.Scopes) (identity.KeyRecord, identity.Secret) {
	t.Helper()
	key, secret, err := identity.GenerateKey(p, scopes)
	check(t, err)
	check(t, s.CreateAPIKey(t.Context(), p, key))
	return key, secret
}

func TestHTTPAuthorizationWithRealSQLite(t *testing.T) {
	s := testStore(t)
	one := fixtureTenant(t, s, tenantOne, "Tenant One")
	two := fixtureTenant(t, s, tenantTwo, "Tenant Two")
	check(t, s.CreateFeed(t.Context(), one, feed.Feed{ID: privateID, Title: "Private One"}))
	key, secret := fixtureKey(t, s, one, identity.FeedsRead)
	_, other := fixtureKey(t, s, two, identity.FeedsRead)
	_, feedback := fixtureKey(t, s, one, identity.FeedbackWrite)
	auth := identity.NewService(s)
	mux := http.NewServeMux()
	// Probe endpoints exist only in this test, using real transport/auth/storage.
	mux.Handle("GET /probe/{id}", httpapi.Require(auth, identity.FeedsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		value, err := s.GetFeed(r.Context(), p, r.PathValue("id"))
		if errors.Is(err, sqlite.ErrNotFound) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		check(t, json.NewEncoder(w).Encode(value))
	}))
	mux.Handle("GET /search", httpapi.Require(auth, identity.FeedsRead, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		value, err := s.Search(r.Context(), p, corpusID, "PHP", time.Unix(0, 0), 50)
		if err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		check(t, json.NewEncoder(w).Encode(value))
	}))
	mux.Handle("POST /mutate", httpapi.Require(auth, identity.FeedbackWrite, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		if err := s.DeleteArticle(r.Context(), p, corpusID); errors.Is(err, identity.ErrForbidden) {
			http.Error(w, "forbidden", 403)
			return
		}
		t.Error("service key mutated operator corpus")
		w.WriteHeader(500)
	}))
	server := httptest.NewServer(mux)
	defer server.Close()
	request := func(path string, headers []string, method string) (int, string, http.Header) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, nil)
		check(t, err)
		for _, header := range headers {
			req.Header.Add("Authorization", header)
		}
		response, err := server.Client().Do(req)
		check(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		check(t, err)
		for _, key := range []identity.Secret{secret, other, feedback} {
			if bytes.Contains(body, []byte(key.Reveal())) {
				t.Fatal("HTTP exposed a service key")
			}
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("authenticated response can be cached")
		}
		return response.StatusCode, string(body), response.Header
	}
	for _, headers := range [][]string{nil, {"Bearer invalid"}, {"Basic " + secret.Reveal()}, {"Bearer " + secret.Reveal(), "Bearer " + secret.Reveal()}, {"Bearer " + secret.Reveal() + ", Bearer " + other.Reveal()}, {"Bearer  " + secret.Reveal()}} {
		status, body, headers := request("/probe/"+corpusID, headers, "GET")
		if status != 401 || headers.Get("WWW-Authenticate") != "Bearer" {
			t.Fatal("invalid HTTP credential accepted")
		}
		var problem struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Retryable bool   `json:"retryable"`
		}
		check(t, json.Unmarshal([]byte(body), &problem))
		if problem.Code != "unauthorized" || problem.Retryable || identity.ValidateID(problem.RequestID) != nil {
			t.Fatal("invalid auth problem shape")
		}
	}
	if status, _, _ := request("/probe/"+corpusID, []string{"Bearer " + feedback.Reveal()}, "GET"); status != 403 {
		t.Fatal("missing read scope was accepted")
	}
	for _, tc := range []struct {
		secret         identity.Secret
		want, excluded string
	}{{secret, "Tenant One", "Tenant Two"}, {other, "Tenant Two", "Tenant One"}} {
		for _, path := range []string{"/probe/" + corpusID + "?tenant_id=" + string(tenantTwo), "/search"} {
			status, body, _ := request(path, []string{"Bearer " + tc.secret.Reveal()}, "GET")
			if status != 200 || !strings.Contains(body, tc.want) || strings.Contains(body, tc.excluded) {
				t.Fatal("HTTP tenant scope changed by guessed ID or query field")
			}
		}
	}
	status, body, _ := request("/probe/"+privateID, []string{"Bearer " + other.Reveal()}, "GET")
	missing, missingBody, _ := request("/probe/00000009-0000-4000-8000-000000000000", []string{"Bearer " + other.Reveal()}, "GET")
	if status != 404 || missing != 404 || body != missingBody {
		t.Fatal("private object existence was disclosed")
	}
	if status, _, _ := request("/mutate", []string{"Bearer " + feedback.Reveal()}, "POST"); status != 403 {
		t.Fatal("feedback scope granted corpus writes")
	}
	check(t, s.RevokeAPIKey(t.Context(), one, key.ID))
	if status, _, _ := request("/probe/"+corpusID, []string{"Bearer " + secret.Reveal()}, "GET"); status != 401 {
		t.Fatal("HTTP used cached revoked identity")
	}
	check(t, s.Close())
	if status, body, _ := request("/probe/"+corpusID, []string{"Bearer " + other.Reveal()}, "GET"); status != 503 || strings.Contains(body, "database") {
		t.Fatal("HTTP storage failure leaked or authenticated")
	}
}

func TestOperatorCommandsPrivateKeyHandoff(t *testing.T) {
	dir := privateDirectory(t)
	path := filepath.Join(dir, "db.sqlite")
	check(t, sqlite.Migrate(t.Context(), path))
	lookup := func(name string) (string, bool) { return path, name == "NORTHWAY_DATABASE_PATH" }
	var stdout, stderr bytes.Buffer
	run := func(args ...string) error {
		stdout.Reset()
		stderr.Reset()
		return app.Execute(t.Context(), args, lookup, &stdout, &stderr)
	}
	check(t, run("tenant", "create", "--tenant", string(tenantOne)))
	output := filepath.Join(dir, "client.key")
	check(t, run("key", "create", "--tenant", string(tenantOne), "--scopes", "feeds:read", "--output", output))
	raw, err := os.ReadFile(output)
	check(t, err)
	secret := strings.TrimSpace(string(raw))
	if bytes.Contains(stdout.Bytes(), []byte(secret)) || bytes.Contains(stderr.Bytes(), []byte(secret)) {
		t.Fatal("operator command exposed key")
	}
	var result struct {
		KeyID string `json:"key_id"`
	}
	check(t, json.Unmarshal(stdout.Bytes(), &result))
	if !identity.ValidKeyID(result.KeyID) {
		t.Fatal("nonsecret key ID missing")
	}
	info, err := os.Stat(output)
	check(t, err)
	if info.Mode().Perm() != 0600 {
		t.Fatal("key file is not 0600")
	}
	s, err := sqlite.Open(t.Context(), path)
	check(t, err)
	if _, err := identity.NewService(s).Authenticate(t.Context(), secret); err != nil {
		t.Fatal("provisioned key unusable")
	}
	check(t, s.Close())
	if err := run("key", "create", "--tenant", string(tenantOne), "--scopes", "feeds:read", "--output", output); err == nil {
		t.Fatal("overwrote credential file")
	}
	unchanged, err := os.ReadFile(output)
	check(t, err)
	if !bytes.Equal(raw, unchanged) {
		t.Fatal("existing key was changed")
	}
	link := filepath.Join(dir, "link.key")
	check(t, os.Symlink(output, link))
	if err := run("key", "create", "--tenant", string(tenantOne), "--scopes", "feeds:read", "--output", link); err == nil {
		t.Fatal("symlink key destination accepted")
	}
	public := filepath.Join(dir, "public")
	check(t, os.Mkdir(public, 0755))
	if err := run("key", "create", "--tenant", string(tenantOne), "--scopes", "feeds:read", "--output", filepath.Join(public, "key")); err == nil {
		t.Fatal("public key directory accepted")
	}
	orphan := filepath.Join(dir, "orphan.key")
	if err := run("key", "create", "--tenant", string(tenantTwo), "--scopes", "feeds:read", "--output", orphan); err == nil {
		t.Fatal("key for absent tenant created")
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("failed provisioning left secret file")
	}
	if err := run("key", "revoke", "--tenant", string(tenantTwo), "--key-id", result.KeyID); err == nil {
		t.Fatal("wrong tenant revoked key")
	}
	check(t, run("key", "revoke", "--tenant", string(tenantOne), "--key-id", result.KeyID))
	s, err = sqlite.Open(t.Context(), path)
	check(t, err)
	defer s.Close()
	if _, err := identity.NewService(s).Authenticate(t.Context(), secret); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatal("CLI revocation did not persist")
	}
	err = run("key", "create", "--tenant", string(tenantOne), "--scopes", "do-not-echo-sensitive-value", "--output", orphan)
	if err == nil || strings.Contains(err.Error()+stdout.String()+stderr.String(), "do-not-echo-sensitive-value") {
		t.Fatal("invalid argument was accepted or echoed")
	}
}
