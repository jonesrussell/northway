package schedule

import (
	"context"
	"github.com/jonesrussell/northway/internal/ingest"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestPublicPublisherWithoutTenantRunsSeriallyAndDrains(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	p := NewPublicPublisher(func(context.Context) (ingest.Result, error) { calls++; return ingest.Result{Status: 200}, nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.wait = func(context.Context, time.Duration) bool { cancel(); return false }
	if e := p.Run(ctx); e != nil || calls != 1 || p.Status() != "idle" {
		t.Fatal(e, calls, p.Status())
	}
}
