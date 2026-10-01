package sqlite

import (
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCustomerExpiryAndDurableSharedBudget(t *testing.T) {
	s, path := fresh(t)
	seed(t, s, tenantA)
	seed(t, s, tenantB)
	key, secret := newKey(t, s, tenantA, identity.FeedsRead)
	must(t, s.queries(s.writer).CreateCustomerKeyExpiry(t.Context(), sqlc.CreateCustomerKeyExpiryParams{KeyID: key.ID, ExpiresAt: time.Now().Add(-time.Minute).UnixMicro()}))
	if _, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal()); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatal("expired customer key authenticated")
	}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 70; i++ {
		wg.Go(func() {
			ok, err := s.TakeRequestBudget(t.Context(), operator(tenantA), now)
			if err != nil {
				t.Error(err)
			}
			if ok {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 60 {
		t.Fatalf("accepted %d", accepted.Load())
	}
	ok, err := s.TakeRequestBudget(t.Context(), operator(tenantB), now)
	must(t, err)
	if !ok {
		t.Fatal("tenant budget crossed boundary")
	}
	must(t, s.Close())
	s, err = Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	ok, err = s.TakeRequestBudget(t.Context(), operator(tenantA), now)
	must(t, err)
	if ok {
		t.Fatal("restart reset quota")
	}
	ok, err = s.TakeRequestBudget(t.Context(), operator(tenantA), now.Add(time.Minute))
	must(t, err)
	if !ok {
		t.Fatal("budget did not replenish")
	}
}
