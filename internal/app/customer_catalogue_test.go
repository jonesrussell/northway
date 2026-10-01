package app_test

import (
	"crypto/ed25519"
	"fmt"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"testing"
)

func TestCustomerCatalogueRetryAndWorkspaceCeiling(t *testing.T) {
	f := beta(t)
	v, err := identity.NewAssertionVerifier("https://northcloud.one", "northcloud-api", map[string]ed25519.PublicKey{"test": f.key.Public().(ed25519.PublicKey)}, f.s)
	check(t, err)
	f.handler = httpapi.NewCustomerAPI(identity.NewService(f.s), v, f.s, query.NewService(f.s), feedback.NewService(f.s), f.s.ProvisionCustomerCatalogue)
	for i := 1; i <= 5; i++ {
		tenant := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		for retry := 0; retry < 2; retry++ {
			status(t, f.call(tenant, "PUT", "/v1/workspace", ""), 200)
		}
		result := f.call(tenant, "GET", "/v1/feeds", "")
		status(t, result, 200)
		if result.Body.String() != `{"feeds":[{"id":"bca10000-0000-4000-8000-000000000001","title":"Official developer news"}]}`+"\n" {
			t.Fatalf("unexpected catalogue: %s", result.Body.String())
		}
	}
	status(t, f.call("00000000-0000-4000-8000-000000000006", "PUT", "/v1/workspace", ""), 403)
	tenants, err := f.s.CustomerTenants(t.Context())
	check(t, err)
	if len(tenants) != 5 {
		t.Fatalf("workspace inventory: %d", len(tenants))
	}
}
