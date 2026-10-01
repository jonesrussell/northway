package app_test

import (
	"bytes"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
	"testing"
)

func TestOfflineCustomerSupportPreservesOtherWorkspaceAndTombstone(t *testing.T) {
	f := beta(t)
	a := "00000000-0000-4000-8000-000000000001"
	b := "00000000-0000-4000-8000-000000000002"
	for _, tenant := range []string{a, b} {
		status(t, f.call(tenant, "PUT", "/v1/workspace", ""), 200)
	}
	issued := f.call(a, "POST", "/v1/keys", `{"scopes":"feeds:read"}`)
	status(t, issued, 201)
	var key struct {
		Secret string `json:"secret"`
	}
	check(t, json.Unmarshal(issued.Body.Bytes(), &key))
	if err := sqlite.CustomerSupport(t.Context(), f.path, identity.TenantID(a), "delete", &bytes.Buffer{}); err == nil {
		t.Fatal("support path accepted live database")
	}
	check(t, f.s.Close())
	var export bytes.Buffer
	check(t, sqlite.CustomerSupport(t.Context(), f.path, identity.TenantID(a), "export", &export))
	if bytes.Contains(export.Bytes(), []byte(key.Secret)) || bytes.Contains(export.Bytes(), []byte(b)) {
		t.Fatal("export leaked credential/other tenant")
	}
	check(t, sqlite.CustomerSupport(t.Context(), f.path, identity.TenantID(a), "suspend", &bytes.Buffer{}))
	s, err := sqlite.Open(t.Context(), f.path)
	check(t, err)
	f.s = s
	f.bind(t)
	status(t, f.call(a, "GET", "/v1/keys", ""), 403)
	status(t, f.call(b, "GET", "/v1/keys", ""), 200)
	tenants, err := f.s.CustomerTenants(t.Context())
	check(t, err)
	if len(tenants) != 1 || string(tenants[0]) != b {
		t.Fatal("suspended workspace remained in polling inventory")
	}
	check(t, f.s.Close())
	check(t, sqlite.CustomerSupport(t.Context(), f.path, identity.TenantID(a), "delete", &bytes.Buffer{}))
	check(t, sqlite.CustomerSupport(t.Context(), f.path, identity.TenantID(a), "delete", &bytes.Buffer{}))
	s, err = sqlite.Open(t.Context(), f.path)
	check(t, err)
	f.s = s
	f.bind(t)
	status(t, f.call(a, "PUT", "/v1/workspace", ""), 403)
	status(t, f.request("GET", "/v1/feeds", "", key.Secret), 401)
	status(t, f.call(b, "GET", "/v1/keys", ""), 200)
}
