package sqlite

import (
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"testing"
	"time"
)

func TestAgentGrantSeedSettlementRejectsRevocationAndExpiry(t *testing.T) {
	for _, mode := range []string{"revoked", "expired"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fresh(t)
			defer s.Close()
			seed(t, s, tenantA)
			p := operator(tenantA)
			now := time.Now().UTC().Add(-time.Minute)
			grant, key, e := identity.GenerateAgentGrant(p, identity.CollectionSeed, "fixture", now, now.Add(time.Hour))
			must(t, e)
			must(t, s.CreateAgentGrant(t.Context(), p, grant))
			authenticated, e := identity.NewAgentService(s).Authenticate(t.Context(), key.Reveal())
			must(t, e)
			if mode == "revoked" {
				must(t, s.RevokeAgentGrant(t.Context(), p, grant.ID))
			} else {
				_, e = s.writer.ExecContext(t.Context(), "UPDATE agent_grants SET expires_at=? WHERE id=?", time.Now().Add(-time.Second).UnixMicro(), grant.ID)
				must(t, e)
			}
			candidate := ingest.CollectionSeed{ID: "00000009-0000-4000-8000-000000000000", URL: "https://fixture.example/creator", Title: "Fixture"}
			if e = s.AddCollectionSeed(t.Context(), authenticated, candidate); !errors.Is(e, identity.ErrUnauthorized) {
				t.Fatal("stale principal created seed", e)
			}
			status, e := s.CollectionStatus(t.Context(), p)
			must(t, e)
			if status.Seeds != 0 {
				t.Fatal("stale write persisted", status)
			}
			must(t, s.AddCollectionSeed(t.Context(), p, candidate))
		})
	}
}
