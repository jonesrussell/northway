package schedule

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
)

type runnerStep struct {
	result ingest.Result
	err    error
}

type scriptedRunner struct {
	mu     sync.Mutex
	steps  []runnerStep
	calls  int
	cancel context.CancelFunc
}

func (r *scriptedRunner) RunOnce(context.Context, identity.Principal) (ingest.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	step := r.steps[r.calls]
	r.calls++
	if r.calls == len(r.steps) && r.cancel != nil {
		r.cancel()
	}
	return step.result, step.err
}

func schedulerPrincipal(t *testing.T) identity.Principal {
	t.Helper()
	p, err := identity.Operator("00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type overlapRunner struct {
	mu      sync.Mutex
	calls   int
	entered chan int
	release chan struct{}
	cancel  context.CancelFunc
}

func (r *overlapRunner) RunOnce(context.Context, identity.Principal) (ingest.Result, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	r.entered <- call
	<-r.release
	if call == 2 {
		r.cancel()
	}
	return ingest.Result{}, nil
}

func TestPublisherDoesNotStartAnotherPollBeforeSettlement(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runner := &overlapRunner{entered: make(chan int, 2), release: make(chan struct{}), cancel: cancel}
	p := NewPublisher(runner, schedulerPrincipal(t), testLogger())
	p.wait = func(context.Context, time.Duration) bool { return true }
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	if call := <-runner.entered; call != 1 {
		t.Fatalf("first call = %d", call)
	}
	select {
	case call := <-runner.entered:
		t.Fatalf("poll %d overlapped the unsettled first poll", call)
	case <-time.After(20 * time.Millisecond):
	}
	runner.release <- struct{}{}
	if call := <-runner.entered; call != 2 {
		t.Fatalf("second call = %d", call)
	}
	runner.release <- struct{}{}
	if err := <-done; err != nil || p.Status() != "idle" {
		t.Fatalf("error=%v status=%s", err, p.Status())
	}
}

func TestPublisherUsesBoundedOutcomeWaits(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   time.Duration
		status string
	}{
		{"idle", ingest.ErrIdle, idleWait, "idle"},
		{"busy recovery", ingest.ErrBusy, recoveryWait, "degraded"},
		{"budget", ingest.ErrBudget, budgetWait, "degraded"},
		{"corpus full", ingest.ErrCorpusFull, budgetWait, "degraded"},
		{"fetch failure", ingest.ErrFetch, settledWait, "degraded"},
		{"expired lease", ingest.ErrLease, recoveryWait, "degraded"},
		{"rejected result", ingest.ErrInvalid, recoveryWait, "degraded"},
		{"settlement deadline", context.DeadlineExceeded, recoveryWait, "degraded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &scriptedRunner{steps: []runnerStep{{err: tt.err}}}
			p := NewPublisher(runner, schedulerPrincipal(t), testLogger())
			var got time.Duration
			var state string
			p.wait = func(_ context.Context, delay time.Duration) bool {
				got, state = delay, p.Status()
				return false
			}
			if err := p.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if got != tt.want || state != tt.status {
				t.Fatalf("delay=%s status=%s", got, state)
			}
		})
	}
}

func TestPublisherRetainsDegradedStateUntilASettledSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runner := &scriptedRunner{steps: []runnerStep{{err: ingest.ErrFetch}, {err: ingest.ErrIdle}, {}}, cancel: cancel}
	p := NewPublisher(runner, schedulerPrincipal(t), testLogger())
	var states []string
	p.wait = func(context.Context, time.Duration) bool {
		states = append(states, p.Status())
		return true
	}
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0] != "degraded" || states[1] != "degraded" || p.Status() != "idle" {
		t.Fatalf("states=%v final=%s", states, p.Status())
	}
}

func TestPublisherReturnsUnknownFailure(t *testing.T) {
	p := NewPublisher(&scriptedRunner{steps: []runnerStep{{err: errors.New("storage")}}}, schedulerPrincipal(t), testLogger())
	if err := p.Run(t.Context()); err == nil || p.Status() != "degraded" {
		t.Fatalf("error=%v status=%s", err, p.Status())
	}
}

type cancelRunner struct{ entered chan struct{} }

func (r cancelRunner) RunOnce(ctx context.Context, _ identity.Principal) (ingest.Result, error) {
	close(r.entered)
	<-ctx.Done()
	return ingest.Result{}, ctx.Err()
}

func TestPublisherCancellationDrainsCurrentRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	entered := make(chan struct{})
	p := NewPublisher(cancelRunner{entered}, schedulerPrincipal(t), testLogger())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if err != nil || p.Status() != "idle" {
			t.Fatalf("error=%v status=%s", err, p.Status())
		}
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
}
