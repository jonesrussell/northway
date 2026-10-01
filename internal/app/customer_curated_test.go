package app_test

import (
	"context"
	"crypto/ed25519"
	"github.com/jonesrussell/northway/internal/catalogue"
	"github.com/jonesrussell/northway/internal/feedback"
	"github.com/jonesrussell/northway/internal/httpapi"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"testing"
)

func TestCustomerCuratedCatalogueUsesReviewedPolicyAndIsRetrySafe(t *testing.T) {
	f := beta(t)
	verifier, e := identity.NewAssertionVerifier("https://northcloud.one", "northcloud-api", map[string]ed25519.PublicKey{"test": f.key.Public().(ed25519.PublicKey)}, f.s)
	check(t, e)
	m := catalogue.Manifest{SchemaVersion: 1, Profile: "curated-v1", ResearchSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", IntervalSeconds: 86400, MaxBytes: 2048,
		Entries: []catalogue.Entry{{Candidate: catalogue.Candidate{URL: "https://science.example/feed", Title: "Science", Publisher: "Fixture publisher", Topic: "science", Region: "Global", Language: "en", Rights: "not_assessed", Notes: "No rehosting assumed"}, Selected: true}}}
	f.handler = httpapi.NewCustomerAPI(identity.NewService(f.s), verifier, f.s, query.NewService(f.s), feedback.NewService(f.s), func(ctx context.Context, p identity.Principal) error {
		return f.s.ProvisionCustomerCuratedCatalogue(ctx, p, m)
	})
	for _, tenant := range []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"} {
		for retry := 0; retry < 2; retry++ {
			status(t, f.call(tenant, "PUT", "/v1/workspace", ""), 200)
		}
		result := f.call(tenant, "GET", "/v1/feeds", "")
		status(t, result, 200)
		principal, e := identity.Operator(identity.TenantID(tenant))
		check(t, e)
		inv, e := f.s.CatalogueInventory(t.Context(), principal)
		check(t, e)
		if len(inv.TenantSources) != 1 || inv.TenantSources[0].PollEnabled != 0 || inv.TenantSources[0].Approved != 0 {
			t.Fatalf("unexpected policy: %+v", inv)
		}
	}
}
