package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
)

type cancellationStore struct{ settled Result }

func (s *cancellationStore) ClaimPoll(context.Context, identity.Principal) (Claim, error) {
	return Claim{ID: "00000000-0000-4000-8000-000000000002", Until: time.Now().Add(time.Minute)}, nil
}

func (s *cancellationStore) FinishPoll(_ context.Context, _ identity.Principal, _ string, result Result) error {
	s.settled = result
	return nil
}

type cancelFetcher struct{ cancel context.CancelFunc }

func (f cancelFetcher) Fetch(context.Context, Claim) Result {
	f.cancel()
	return Result{Failure: "transport", NotBefore: time.Now().Add(time.Hour)}
}

func TestRunOnceRecordsOperatorCancellationAccurately(t *testing.T) {
	principal, err := identity.Operator("00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	store := &cancellationStore{}
	_, err = New(store, cancelFetcher{cancel}).RunOnce(ctx, principal)
	if !errors.Is(err, ErrFetch) {
		t.Fatalf("error = %v, want ErrFetch", err)
	}
	if store.settled.Failure != "cancelled" || store.settled.NotBefore.IsZero() {
		t.Fatalf("settled failure = %q hold = %v", store.settled.Failure, store.settled.NotBefore)
	}
}
