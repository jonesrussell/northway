// Package ingest coordinates bounded, metadata-only publisher feed collection.
package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
)

const (
	MaxResponseBytes  int64 = 2 * 1024 * 1024
	DailyBytes        int64 = 64 * 1024 * 1024
	DailyAttempts           = 180
	MaxItems                = 1000
	MaxSources              = 100
	MaxSourceItems          = 5000
	MaxSourceVersions       = 10000
	FetchTimeout            = 15 * time.Second
	LeaseDuration           = 2 * time.Minute
)

var (
	ErrIdle       = errors.New("no eligible feed due")
	ErrBudget     = errors.New("acquisition budget exhausted")
	ErrCorpusFull = errors.New("corpus admission limit reached")
	ErrBusy       = errors.New("acquisition already in progress")
	ErrLease      = errors.New("poll claim expired or unavailable")
	ErrInvalid    = errors.New("invalid ingestion input")
	ErrFetch      = errors.New("feed fetch failed")
	ErrParse      = errors.New("feed parse failed")
)

// Policy is operator-only. Approval and Enabled are deliberately separate.
// Neither repository catalogues nor existing sources enable collection.
type Policy struct {
	SourceID          string
	URL               string
	Approved, Enabled bool
	Interval          time.Duration
	MaxBytes          int64
}

type Claim struct {
	ID, SourceID, URL, ETag, LastModified string
	MaxBytes                              int64
	Until                                 time.Time
}

// Item contains no publisher body, excerpt, enclosure or remotely chosen source.
type Item struct {
	OriginID, URL, Title string
	PublishedAt          *time.Time
}

type Result struct {
	Status             int
	ETag, LastModified string
	NotBefore          time.Time
	Bytes              int64
	Items              []Item
	Failure            string // fixed category only; never raw publisher/transport errors
}

type Store interface {
	ClaimPoll(context.Context, identity.Principal) (Claim, error)
	FinishPoll(context.Context, identity.Principal, string, Result) error
}

type Fetcher interface {
	Fetch(context.Context, Claim) Result
}

type Service struct {
	store   Store
	fetcher Fetcher
}

func New(store Store, fetcher Fetcher) *Service { return &Service{store: store, fetcher: fetcher} }

// RunOnce makes at most one request. There are no retries or startup schedules.
// The database owns global admission; network I/O is outside its transaction.
func (s *Service) RunOnce(ctx context.Context, p identity.Principal) (Result, error) {
	if _, err := p.RequireOperator(); err != nil {
		return Result{}, err
	}
	claim, err := s.store.ClaimPoll(ctx, p)
	if err != nil {
		return Result{}, err
	}
	leaseCtx, leaseCancel := context.WithDeadline(ctx, claim.Until)
	defer leaseCancel()
	fetchCtx, cancel := context.WithTimeout(leaseCtx, FetchTimeout)
	result := s.fetcher.Fetch(fetchCtx, claim)
	cancel()
	if ctx.Err() != nil {
		// Preserve conservative accounting while recording an accurate fixed
		// category for an operator-requested drain.
		result = Result{Status: result.Status, Bytes: result.Bytes, NotBefore: result.NotBefore, Failure: "cancelled"}
	}
	// Cancellation must not erase consumed work. A crash leaves the full byte
	// reservation charged; a bounded detached settlement records known failures.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if err := s.store.FinishPoll(finishCtx, p, claim.ID, result); err != nil {
		if errors.Is(err, ErrCorpusFull) {
			// The batch rolled back. Record failure without publishing parsed
			// items or new validators; stop reserving the active slot.
			failed := Result{Status: result.Status, Bytes: result.Bytes, NotBefore: result.NotBefore, Failure: "corpus_full"}
			return failed, errors.Join(err, s.store.FinishPoll(finishCtx, p, claim.ID, failed))
		}
		return Result{}, err
	}
	if result.Failure != "" {
		return result, ErrFetch
	}
	return result, nil
}
