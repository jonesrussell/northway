package app

import (
	"context"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/sqlite"
	"log/slog"
	"sync/atomic"
	"time"
)

// customerPublisher visits at most five persisted customer workspaces serially.
// Identity comes from the trusted job inventory, never from HTTP input. Existing
// SQLite claims enforce global attempts/bytes, one acquisition and source timing.
type customerPublisher struct {
	store    *sqlite.Store
	runner   *ingest.Service
	logger   *slog.Logger
	degraded atomic.Bool
}

func (p *customerPublisher) Status() string {
	if p.degraded.Load() {
		return "degraded"
	}
	return "idle"
}
func (p *customerPublisher) Run(ctx context.Context) error {
	lastMaintenance := time.Time{}
	for ctx.Err() == nil {
		tenants, err := p.store.CustomerTenants(ctx)
		if err != nil {
			return err
		}
		maintenanceDue := time.Since(lastMaintenance) >= time.Hour
		degraded := false
		for _, tenant := range tenants {
			principal, err := identity.Operator(tenant)
			if err != nil {
				return err
			}
			if maintenanceDue {
				work, cancel := context.WithTimeout(ctx, 30*time.Second)
				err := maintainStorage(work, p.store, principal, p.logger)
				cancel()
				if err != nil {
					degraded = true
				}
			}
			work, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, err = p.runner.RunOnce(work, principal)
			cancel()
			if ctx.Err() != nil {
				return nil
			}
			switch {
			case err == nil, errors.Is(err, ingest.ErrIdle):
			case errors.Is(err, ingest.ErrFetch), errors.Is(err, ingest.ErrBusy), errors.Is(err, ingest.ErrBudget), errors.Is(err, ingest.ErrCorpusFull), errors.Is(err, ingest.ErrInvalid), errors.Is(err, ingest.ErrLease), errors.Is(err, context.DeadlineExceeded):
				degraded = true
			default:
				return err
			}
		}
		if maintenanceDue {
			lastMaintenance = time.Now()
		}
		p.degraded.Store(degraded)
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
