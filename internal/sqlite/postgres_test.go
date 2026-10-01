package sqlite

import (
	"errors"
	"fmt"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/source"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// This destructive fixture is restricted to the explicitly selected disposable
// local northway_test database. It cannot target a production DSN.
func postgresFresh(t *testing.T) (*Store, string) {
	t.Helper()
	file := os.Getenv("NORTHWAY_TEST_POSTGRES_FILE")
	raw, e := os.ReadFile(file)
	must(t, e)
	u, e := url.Parse(strings.TrimSpace(string(raw)))
	must(t, e)
	if u.Hostname() != "127.0.0.1" || u.Path != "/northway_test" {
		t.Fatal("PostgreSQL tests require disposable loopback northway_test")
	}
	db, e := postgresPool(file, 1)
	must(t, e)
	_, e = db.ExecContext(t.Context(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	must(t, e)
	must(t, db.Close())
	path := "postgres:" + file
	must(t, Migrate(t.Context(), path))
	s, e := Open(t.Context(), path)
	must(t, e)
	t.Cleanup(func() { must(t, s.Close()) })
	return s, path
}

func TestPostgresIndependentClaimsSkipLocksAndReservations(t *testing.T) {
	if os.Getenv("NORTHWAY_TEST_POSTGRES_FILE") == "" {
		t.Skip("requires disposable PostgreSQL fixture")
	}
	s, path := fresh(t)
	now := queryEpoch
	s.clock = func() time.Time { return now }
	for _, tenant := range []identity.TenantID{tenantA, tenantB} {
		must(t, s.CreateTenant(t.Context(), tenant))
		for n := 0; n < 4; n++ {
			id := fmt.Sprintf("00000003-0000-4000-8000-%012d", n)
			u := fmt.Sprintf("https://host%d.invalid/feed", n)
			must(t, s.CreateSource(t.Context(), operator(tenant), source.Source{ID: id, URL: u, Title: "Synthetic"}))
			must(t, s.ConfigurePoll(t.Context(), operator(tenant), ingest.Policy{SourceID: id, URL: u, Mode: "html", Enabled: true, Approved: true, RobotsUntil: now.Add(time.Hour), Interval: time.Hour, MaxBytes: ingest.MaxResponseBytes}))
		}
	}
	second, e := Open(t.Context(), path)
	must(t, e)
	defer second.Close()
	second.clock = s.clock
	// An unrelated transaction owns the first source. Claiming must skip it.
	tx, e := s.writer.BeginTx(t.Context(), nil)
	must(t, e)
	_, e = tx.ExecContext(t.Context(), "SELECT source_id FROM poll_sources WHERE tenant_id=$1 AND source_id=$2 FOR UPDATE", string(tenantA), "00000003-0000-4000-8000-000000000000")
	must(t, e)
	cl, e := second.ClaimCollection(t.Context(), operator(tenantA))
	must(t, e)
	if cl.SourceID == "00000003-0000-4000-8000-000000000000" {
		t.Fatal("locked source claimed")
	}
	must(t, tx.Rollback())
	var wg sync.WaitGroup
	var mu sync.Mutex
	claims := []ingest.Claim{cl}
	failures := []error{}
	for n := 0; n < 12; n++ {
		wg.Go(func() {
			v, e := second.ClaimCollection(t.Context(), operator(tenantB))
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				claims = append(claims, v)
			} else if !errors.Is(e, ingest.ErrBusy) && !errors.Is(e, ingest.ErrIdle) {
				failures = append(failures, e)
			}
		})
	}
	wg.Wait()
	if len(failures) > 0 {
		t.Fatal(failures)
	}
	if len(claims) != 4 {
		t.Fatalf("global active cap admitted %d claims", len(claims))
	}
	seen := map[string]bool{}
	for _, v := range claims {
		if seen[v.ID] {
			t.Fatal("duplicate claim")
		}
		seen[v.ID] = true
	}
	var reserved int64
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT sum(reserved_bytes) FROM poll_attempts").Scan(&reserved))
	if reserved != 4*ingest.MaxResponseBytes {
		t.Fatal("reservation accounting", reserved)
	}
	// Completion is fenced across tenants and after owner restart/lease expiry.
	if e = second.FinishPoll(t.Context(), operator(tenantB), cl.ID, ingest.Result{Status: 404}); !errors.Is(e, ingest.ErrLease) {
		t.Fatal("cross-tenant settlement", e)
	}
	now = now.Add(ingest.LeaseDuration)
	if e = second.FinishPoll(t.Context(), operator(tenantA), cl.ID, ingest.Result{Status: 404}); !errors.Is(e, ingest.ErrLease) {
		t.Fatal("stale completion", e)
	}
	must(t, second.Close())
	reopened, e := Open(t.Context(), path)
	must(t, e)
	defer reopened.Close()
	reopened.clock = s.clock
	if e = reopened.FinishPoll(t.Context(), operator(tenantA), cl.ID, ingest.Result{Status: 404}); !errors.Is(e, ingest.ErrLease) {
		t.Fatal("restart accepted stale completion", e)
	}
	var used int64
	must(t, reopened.readers.QueryRowContext(t.Context(), "SELECT sum(charged_bytes) FROM poll_attempts").Scan(&used))
	if used != reserved {
		t.Fatal("abandoned reservations refunded")
	}
}
