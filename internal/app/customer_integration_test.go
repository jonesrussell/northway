package app_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type betaFixture struct {
	s        *sqlite.Store
	path     string
	key      ed25519.PrivateKey
	handler  http.Handler
	sequence atomic.Int64
}

func beta(t *testing.T) *betaFixture {
	t.Helper()
	dir := t.TempDir()
	check(t, os.Chmod(dir, 0700))
	path := filepath.Join(dir, "beta.sqlite")
	check(t, sqlite.Migrate(t.Context(), path))
	s, err := sqlite.Open(t.Context(), path)
	check(t, err)
	_, key, err := ed25519.GenerateKey(rand.Reader)
	check(t, err)
	f := &betaFixture{s: s, path: path, key: key}
	f.bind(t)
	t.Cleanup(func() { _ = f.s.Close() })
	return f
}
func (f *betaFixture) bind(t *testing.T) {
	v, err := identity.NewAssertionVerifier("https://northcloud.one", "northcloud-api", map[string]ed25519.PublicKey{"test": f.key.Public().(ed25519.PublicKey)}, f.s)
	check(t, err)
	f.handler = httpapi.NewCustomerAPI(identity.NewService(f.s), v, f.s, query.NewService(f.s), feedback.NewService(f.s))
}
func (f *betaFixture) sign(tenant, op string, modify func(map[string]any)) string {
	now := time.Now().Unix()
	c := map[string]any{"iss": "https://northcloud.one", "aud": "northcloud-api", "sub": tenant, "tenant": tenant, "iat": now, "nbf": now, "exp": now + 30, "jti": fmt.Sprintf("%08x-1111-4111-8111-111111111111", f.sequence.Add(1)), "op": op, "ver": 1}
	if modify != nil {
		modify(c)
	}
	b, _ := json.Marshal(c)
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"ncl-fpa+jwt","kid":"test"}`))
	message := h + "." + base64.RawURLEncoding.EncodeToString(b)
	return message + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.key, []byte(message)))
}
func (f *betaFixture) request(method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func (f *betaFixture) call(tenant, method, path, body string) *httptest.ResponseRecorder {
	return f.request(method, path, body, f.sign(tenant, method+" "+path, nil))
}
func status(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status %d want %d body %s", w.Code, want, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response cacheable")
	}
}

func TestCustomerHTTPProvisionKeysIsolationAndRevoke(t *testing.T) {
	f := beta(t)
	a, b := string(tenantOne), string(tenantTwo)
	for _, tenant := range []string{a, b, a} {
		status(t, f.call(tenant, "PUT", "/v1/workspace", ""), 200)
	}
	w := f.call(a, "POST", "/v1/keys", `{"scopes":"feeds:read"}`)
	status(t, w, 201)
	var issued struct {
		Key    identity.KeyMetadata `json:"key"`
		Secret string               `json:"secret"`
	}
	check(t, json.Unmarshal(w.Body.Bytes(), &issued))
	if len(issued.Secret) != 80 || issued.Key.ExpiresAt.Sub(issued.Key.CreatedAt) != 30*24*time.Hour {
		t.Fatal("key contract")
	}
	w = f.call(a, "GET", "/v1/keys", "")
	status(t, w, 200)
	if strings.Contains(w.Body.String(), issued.Secret) || !strings.Contains(w.Body.String(), issued.Key.ID) {
		t.Fatal("key metadata leak/absence")
	}
	w = f.call(b, "GET", "/v1/keys", "")
	status(t, w, 200)
	if strings.Contains(w.Body.String(), issued.Key.ID) {
		t.Fatal("cross-tenant list")
	}
	status(t, f.call(b, "DELETE", "/v1/keys/"+issued.Key.ID, ""), 404)
	status(t, f.request("GET", "/v1/feeds", "", issued.Secret), 200)
	status(t, f.request("GET", "/v1/keys", "", issued.Secret), 403)
	status(t, f.request("PUT", "/v1/workspace", "", issued.Secret), 403)
	status(t, f.call(a, "DELETE", "/v1/keys/"+issued.Key.ID, ""), 204)
	status(t, f.request("GET", "/v1/feeds", "", issued.Secret), 401)
	status(t, f.call(a, "POST", "/v1/keys", `{"scopes":"operator"}`), 400)
	status(t, f.call(a, "POST", "/v1/keys", `{"scopes":"feeds:read","tenant":"`+b+`"}`), 400)
	for i := 0; i < 5; i++ {
		status(t, f.call(a, "POST", "/v1/keys", `{"scopes":"feeds:read"}`), 201)
	}
	status(t, f.call(a, "POST", "/v1/keys", `{"scopes":"feeds:read"}`), 403)
}

func TestCustomerReplayConcurrentRestartAndOperation(t *testing.T) {
	f := beta(t)
	a := string(tenantOne)
	token := f.sign(a, "PUT /v1/workspace", nil)
	status(t, f.request("GET", "/v1/keys", "", token), 401) // Wrong op cannot burn legitimate assertion.
	var successes atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			w := f.request("PUT", "/v1/workspace", "", token)
			if w.Code == 200 {
				successes.Add(1)
			} else if w.Code != 401 {
				t.Errorf("unexpected replay status %d", w.Code)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("replay accepted more/less than once")
	}
	check(t, f.s.Close())
	s, err := sqlite.Open(t.Context(), f.path)
	check(t, err)
	f.s = s
	f.bind(t)
	status(t, f.request("PUT", "/v1/workspace", "", token), 401)
	status(t, f.call(a, "PUT", "/v1/workspace", ""), 200)
	check(t, f.s.Close())
	status(t, f.call(a, "GET", "/v1/keys", ""), 503)
}

func TestCustomerProfileAndOperatorIsolation(t *testing.T) {
	f := beta(t)
	a := string(tenantOne)
	for _, change := range []func(map[string]any){func(c map[string]any) { c["iss"] = "https://goformx.com" }, func(c map[string]any) { c["aud"] = "goformx" }, func(c map[string]any) { c["sub"] = string(tenantTwo) }, func(c map[string]any) { c["ver"] = 2 }, func(c map[string]any) { c["exp"] = time.Now().Unix() - 10 }, func(c map[string]any) { c["exp"] = time.Now().Unix() + 120 }, func(c map[string]any) { c["iat"] = int64(9223372036854775807) }, func(c map[string]any) { c["extra"] = "bad" }} {
		status(t, f.request("PUT", "/v1/workspace", "", f.sign(a, "PUT /v1/workspace", change)), 401)
	}
	check(t, f.s.CreateTenant(t.Context(), tenantOne))
	status(t, f.call(a, "PUT", "/v1/workspace", ""), 403) // Never adopt a pre-existing pilot automatically.
	op, err := identity.Operator(tenantTwo)
	check(t, err)
	if _, _, err = identity.GenerateManagedKey(op, identity.FeedsRead); err == nil {
		t.Fatal("operator treated as customer assertion")
	}
}

func TestCustomerOldActiveKeyRemainsVisible(t *testing.T) {
	f := beta(t)
	a := string(tenantOne)
	v, err := identity.NewAssertionVerifier("https://northcloud.one", "northcloud-api", map[string]ed25519.PublicKey{"test": f.key.Public().(ed25519.PublicKey)}, f.s)
	check(t, err)
	p, err := v.Verify(t.Context(), f.sign(a, "test", nil), "test", time.Now().UTC())
	check(t, err)
	check(t, f.s.EnsureWorkspace(t.Context(), p))
	old, _, err := f.s.IssueCustomerKey(t.Context(), p, identity.FeedsRead)
	check(t, err)
	for i := 0; i < 105; i++ {
		k, _, err := f.s.IssueCustomerKey(t.Context(), p, identity.FeedsRead)
		check(t, err)
		check(t, f.s.RevokeCustomerKey(t.Context(), p, k.ID))
	}
	w := f.call(a, "GET", "/v1/keys", "")
	status(t, w, 200)
	if !strings.Contains(w.Body.String(), old.ID) {
		t.Fatal("older active key hidden by inactive history")
	}
	status(t, f.call(a, "DELETE", "/v1/keys/"+old.ID, ""), 204)
}

func TestCustomerConcurrentDistinctProvisioning(t *testing.T) {
	f := beta(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			w := f.call(string(tenantOne), "PUT", "/v1/workspace", "")
			if w.Code != 200 {
				t.Errorf("provision %d: %s", w.Code, w.Body.String())
			}
		})
	}
	wg.Wait()
	status(t, f.call(string(tenantOne), "POST", "/v1/keys", `{"scopes":"feeds:read"}`), 201)
}

func TestCustomerHTTPBudgetSharedByAssertionsAndKeys(t *testing.T) {
	f := beta(t)
	a := string(tenantOne)
	v, err := identity.NewAssertionVerifier("https://northcloud.one", "northcloud-api", map[string]ed25519.PublicKey{"test": f.key.Public().(ed25519.PublicKey)}, f.s)
	check(t, err)
	p, err := v.Verify(t.Context(), f.sign(a, "test", nil), "test", time.Now().UTC())
	check(t, err)
	check(t, f.s.EnsureWorkspace(t.Context(), p))
	_, secret, err := f.s.IssueCustomerKey(t.Context(), p, identity.FeedsRead)
	check(t, err)
	// Precharge both boundary minutes to make the HTTP assertion robust to a
	// minute rollover during the short test, without sleeping or clock changes.
	now := time.Now().UTC()
	for _, when := range []time.Time{now, now.Add(time.Minute)} {
		for i := 0; i < 60; i++ {
			ok, err := f.s.TakeRequestBudget(t.Context(), p, when)
			check(t, err)
			if !ok {
				t.Fatal("early budget failure")
			}
		}
	}
	status(t, f.call(a, "GET", "/v1/feeds", ""), 429)
	status(t, f.request("GET", "/v1/feeds", "", secret.Reveal()), 429)
	status(t, f.call(string(tenantTwo), "PUT", "/v1/workspace", ""), 200)
}
