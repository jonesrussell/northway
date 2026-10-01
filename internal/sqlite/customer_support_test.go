package sqlite

import (
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"io"
	"testing"
	"time"
)

func TestCustomerDeletionPreservesNonIdentifyingAcquisitionCharges(t *testing.T) {
	s, path, now := pollSetup(t)
	must(t, sqlc.New(s.writer).EnsureCustomerWorkspace(t.Context(), sqlc.EnsureCustomerWorkspaceParams{TenantID: string(tenantA), CreatedAt: now.UnixMicro()}))
	claim, err := s.ClaimPoll(t.Context(), operator(tenantA))
	must(t, err)
	must(t, s.FinishPoll(t.Context(), operator(tenantA), claim.ID, pollResult()))
	before, err := sqlc.New(s.readers).PollWindow(t.Context())
	must(t, err)
	must(t, s.Close())
	must(t, CustomerSupport(t.Context(), path, tenantA, "delete", io.Discard))
	s, err = Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	after, err := sqlc.New(s.readers).PollWindow(t.Context())
	must(t, err)
	if before != after {
		t.Fatalf("customer deletion reset acquisition charges: before %v after %v", before, after)
	}
	var personal int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM articles WHERE tenant_id=?", tenantA).Scan(&personal))
	if personal != 0 {
		t.Fatal("customer article remained")
	}
	must(t, sqlc.New(s.writer).ExpireErasedAcquisitionUsage(t.Context(), now.Add(48*time.Hour).UnixMicro()))
	expired, err := sqlc.New(s.readers).PollWindow(t.Context())
	must(t, err)
	if expired.Attempts != 0 || expired.Used != 0 {
		t.Fatal("expired anonymous charge remained")
	}
}
