// Package schedule owns bounded in-process background work.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
)

const (
	idleWait     = 30 * time.Second
	settledWait  = time.Second
	recoveryWait = 30 * time.Second
	budgetWait   = 5 * time.Minute
)

type pollRunner interface {
	RunOnce(context.Context, identity.Principal) (ingest.Result, error)
}

type State uint32

const (
	StateIdle State = iota
	StatePolling
	StateDegraded
)

func (s State) String() string {
	switch s {
	case StatePolling:
		return "polling"
	case StateDegraded:
		return "degraded"
	default:
		return "idle"
	}
}

// Publisher runs the existing one-request ingestion transaction serially.
// Poll policy, leases, budgets and next-eligible times remain database-owned.
type Publisher struct {
	runner    pollRunner
	principal identity.Principal
	logger    *slog.Logger
	state     atomic.Uint32
	wait      func(context.Context, time.Duration) bool
}

func NewPublisher(runner pollRunner, principal identity.Principal, logger *slog.Logger) *Publisher {
	p := &Publisher{runner: runner, principal: principal, logger: logger, wait: waitContext}
	p.state.Store(uint32(StateIdle))
	return p
}

func (p *Publisher) Status() string { return State(p.state.Load()).String() }

// Run owns one serial loop. It never enables sources, retries a fetch, or
// derives future work from missed intervals; ClaimPoll decides what is due.
func (p *Publisher) Run(ctx context.Context) error {
	degraded := false
	for ctx.Err() == nil {
		p.state.Store(uint32(StatePolling))
		result, err := p.runner.RunOnce(ctx, p.principal)
		if ctx.Err() != nil {
			p.state.Store(uint32(StateIdle))
			return nil
		}

		state, delay, reason := StateIdle, settledWait, "complete"
		switch {
		case err == nil:
			degraded = false
			p.logger.Info("publisher poll settled", "outcome", reason, "http_status", result.Status, "items", len(result.Items), "bytes", result.Bytes)
		case errors.Is(err, ingest.ErrIdle):
			delay, reason = idleWait, "idle"
			if degraded {
				state = StateDegraded
			}
		case errors.Is(err, ingest.ErrBusy):
			degraded, state, delay, reason = true, StateDegraded, recoveryWait, "busy"
		case errors.Is(err, ingest.ErrBudget):
			degraded, state, delay, reason = true, StateDegraded, budgetWait, "budget_exhausted"
		case errors.Is(err, ingest.ErrCorpusFull):
			degraded, state, delay, reason = true, StateDegraded, budgetWait, "corpus_full"
		case errors.Is(err, ingest.ErrFetch):
			degraded, state, delay, reason = true, StateDegraded, settledWait, "fetch_failed"
		case errors.Is(err, ingest.ErrLease):
			degraded, state, delay, reason = true, StateDegraded, recoveryWait, "lease_expired"
		case errors.Is(err, ingest.ErrInvalid):
			degraded, state, delay, reason = true, StateDegraded, recoveryWait, "result_rejected"
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			degraded, state, delay, reason = true, StateDegraded, recoveryWait, "operation_interrupted"
		default:
			p.state.Store(uint32(StateDegraded))
			return fmt.Errorf("publisher scheduler: %w", err)
		}
		p.state.Store(uint32(state))
		if reason != "complete" && reason != "idle" {
			p.logger.Warn("publisher poll deferred", "outcome", reason, "http_status", result.Status, "bytes", result.Bytes)
		}
		if !p.wait(ctx, delay) {
			p.state.Store(uint32(StateIdle))
			return nil
		}
	}
	p.state.Store(uint32(StateIdle))
	return nil
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
