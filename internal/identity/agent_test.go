package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

type grantFixture struct {
	g     AgentGrant
	touch bool
}

func (f *grantFixture) LookupAgentGrant(context.Context, string) (AgentGrant, error) { return f.g, nil }
func (f *grantFixture) TouchAgentGrant(context.Context, TenantID, string, time.Time) (bool, error) {
	return f.touch, nil
}
func TestAgentGrantExpiryRevocationAndClassSeparation(t *testing.T) {
	p, e := Operator(TenantID("00000001-0000-4000-8000-000000000000"))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Add(-time.Second)
	g, key, e := GenerateAgentGrant(p, CollectionStatus, "fixture", now, now.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	store := &grantFixture{g: g, touch: true}
	service := NewAgentService(store)
	principal, e := service.Authenticate(t.Context(), key.Reveal())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = principal.RequireCollection(CollectionSeed); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
	for _, change := range []func(*AgentGrant){
		func(g *AgentGrant) { g.ExpiresAt = time.Now().Add(-time.Second) },
		func(g *AgentGrant) { at := time.Now(); g.RevokedAt = &at },
		func(g *AgentGrant) { g.CreatedAt = time.Now().Add(time.Hour) },
		func(g *AgentGrant) { g.Scopes = 0 },
		func(g *AgentGrant) { g.TenantID = "invalid" },
		func(g *AgentGrant) { g.Digest = [32]byte{} },
	} {
		store.g = g
		change(&store.g)
		if _, e = service.Authenticate(t.Context(), key.Reveal()); !errors.Is(e, ErrUnauthorized) {
			t.Fatal("invalid grant accepted", e)
		}
	}
	store.g = g
	store.touch = false
	if _, e = service.Authenticate(t.Context(), key.Reveal()); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("revocation race", e)
	}
	_, feedKey, e := GenerateKey(p, FeedsRead)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.Authenticate(t.Context(), feedKey.Reveal()); !errors.Is(e, ErrUnauthorized) {
		t.Fatal(e)
	}
	if FeedsRead.Valid() != true || (FeedsRead|FeedbackWrite).Valid() != true || Scopes(4).Valid() {
		t.Fatal("external scopes broadened")
	}
}
