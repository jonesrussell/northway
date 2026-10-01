package sqlite

import (
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/query"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"os"
	"testing"
	"time"
)

func TestPostgresSharedKeyRevocationAndExpiryFenceCacheWrites(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("local PostgreSQL only")
	}
	for _, mode := range []string{"revoked", "expired"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := fresh(t)
			s.clock = func() time.Time { return queryEpoch }
			src, e := s.ConfigurePublicSource(t.Context(), publicDefinition(1))
			must(t, e)
			_, e = s.RunPublicPollOnce(t.Context(), &publicSyntheticFetcher{})
			must(t, e)
			publicWorkspace(t, s, tenantA, src)
			key, secret := newKey(t, s, tenantA, identity.FeedsRead)
			must(t, s.queries(s.writer).CreateCustomerKeyExpiry(t.Context(), sqlc.CreateCustomerKeyExpiryParams{KeyID: key.ID, ExpiresAt: time.Now().Add(time.Hour).UnixMicro()}))
			p, e := identity.NewService(s).Authenticate(t.Context(), secret.Reveal())
			must(t, e)
			pending, e := s.BeginQuery(t.Context(), p, "public-key-revoke-race", request(), policy())
			must(t, e)
			if mode == "revoked" {
				must(t, s.RevokeAPIKey(t.Context(), operator(tenantA), key.ID))
			} else {
				_, e = s.writer.ExecContext(t.Context(), `UPDATE customer_key_expiry SET expires_at=$1 WHERE key_id=$2`, time.Now().Add(-time.Second).UnixMicro(), key.ID)
				must(t, e)
			}
			_, e = s.CompleteQuery(t.Context(), p, pending.WorkID, "deterministic_fallback", nil, query.Settlement{Known: true})
			if !errors.Is(e, identity.ErrUnauthorized) {
				t.Fatal("stale key wrote cache", mode, e)
			}
			if pgCount(t, s, "SELECT count(*) FROM query_snapshots") != 0 {
				t.Fatal("stale key published snapshot")
			}
			if _, e = s.BeginQuery(t.Context(), p, "public-key-revoke-later", request(), policy()); !errors.Is(e, identity.ErrUnauthorized) {
				t.Fatal("stale principal admitted again", e)
			}
		})
	}
}
