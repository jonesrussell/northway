package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/fetch"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/schedule"
	"github.com/jonesrussell/northway/internal/source"
	"github.com/jonesrussell/northway/internal/sqlite"
)

type fixtureFeed struct{}

func (fixtureFeed) Fetch(ctx context.Context, c ingest.Claim) ingest.Result {
	data := []byte(`<rss version="2.0"><channel><title>Synthetic</title><item><guid>world-1</guid><title>World news fixture</title><link>https://example.invalid/world</link></item></channel></rss>`)
	items, err := fetch.Parse(ctx, data)
	if err != nil {
		return ingest.Result{Failure: "parse"}
	}
	return ingest.Result{Status: 200, Bytes: int64(len(data)), Items: items}
}

type blockingFeed struct {
	entered chan struct{}
	release chan struct{}
	once    *sync.Once
}

func (f blockingFeed) Fetch(ctx context.Context, claim ingest.Claim) ingest.Result {
	f.once.Do(func() { close(f.entered) })
	select {
	case <-f.release:
		return fixtureFeed{}.Fetch(ctx, claim)
	case <-ctx.Done():
		return ingest.Result{Failure: "transport"}
	}
}

func TestIngestionAssemblyAndIdleCLI(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ingest.sqlite")
	// Migrate enforces a private parent directory on all platforms supported here.
	if err := sqlite.Migrate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tenant := identity.TenantID("00000001-0000-4000-8000-000000000000")
	p, err := identity.Operator(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateTenant(t.Context(), tenant); err != nil {
		t.Fatal(err)
	}
	id := "00000003-0000-4000-8000-000000000000"
	if err = s.CreateSource(t.Context(), p, source.Source{ID: id, URL: "https://example.invalid/feed", Title: "World"}); err != nil {
		t.Fatal(err)
	}
	if err = s.ConfigurePoll(t.Context(), p, ingest.Policy{SourceID: id, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}); err != nil {
		t.Fatal(err)
	}
	result, err := ingest.New(s, fixtureFeed{}).RunOnce(t.Context(), p)
	if err != nil || len(result.Items) != 1 {
		t.Fatal(result, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	lookup := func(string) (string, bool) { return "", false }
	if err = Execute(t.Context(), []string{"ingest", "once", "--database", path, "--tenant", string(tenant)}, lookup, &output, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status":"idle"`) || strings.Contains(output.String(), "example.invalid") {
		t.Fatal(output.String())
	}
}

func TestIngestCLIRejectsActivationAndLeaks(t *testing.T) {
	lookup := func(string) (string, bool) { return "", false }
	for _, args := range [][]string{{"ingest", "once", "--url", "https://private.invalid/secret"}, {"ingest", "once", "--tenant", "bad"}, {"ingest", "enable"}} {
		var out bytes.Buffer
		err := Execute(t.Context(), args, lookup, &out, &out)
		if err == nil {
			t.Fatal("invalid flags accepted")
		}
		if strings.Contains(err.Error(), "private.invalid") {
			t.Fatal("sensitive flag echoed")
		}
	}
	var out bytes.Buffer
	if err := Execute(t.Context(), []string{"ingest", "--help"}, lookup, &out, &out); err != nil || !strings.Contains(out.String(), "cannot add sources") {
		t.Fatal(out.String(), err)
	}
}

func TestPublisherSchedulerUsesLiveSQLiteOwnerAndPersistedDueState(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "scheduled.sqlite")
	if err := sqlite.Migrate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tenant := identity.TenantID("00000001-0000-4000-8000-000000000000")
	principal, err := identity.Operator(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CreateTenant(t.Context(), tenant); err != nil {
		t.Fatal(err)
	}
	sourceID := "00000003-0000-4000-8000-000000000000"
	if err = store.CreateSource(t.Context(), principal, source.Source{ID: sourceID, URL: "https://example.invalid/feed", Title: "World"}); err != nil {
		t.Fatal(err)
	}
	if err = store.ConfigurePoll(t.Context(), principal, ingest.Policy{SourceID: sourceID, URL: "https://example.invalid/feed", Approved: true, Enabled: true, Interval: time.Hour, MaxBytes: 2048}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	feed := blockingFeed{entered: make(chan struct{}), release: make(chan struct{}), once: new(sync.Once)}
	publisher := schedule.NewPublisher(ingest.New(store, feed), principal, discardLogger())
	done := make(chan error, 1)
	go func() { done <- publisher.Run(ctx) }()
	select {
	case <-feed.entered:
		if publisher.Status() != "polling" {
			t.Fatalf("status=%s", publisher.Status())
		}
	case <-time.After(time.Second):
		t.Fatal("scheduled poll did not start")
	}
	close(feed.release)
	deadline := time.Now().Add(time.Second)
	for publisher.Status() == "polling" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if publisher.Status() != "idle" {
		t.Fatalf("status=%s", publisher.Status())
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := ingest.New(store, fixtureFeed{}).RunOnce(t.Context(), principal); !errors.Is(err, ingest.ErrIdle) {
		t.Fatalf("persisted schedule was not advanced: %v", err)
	}
}

func TestCorpusFailureIsNotSuccessfulDeferral(t *testing.T) {
	var out bytes.Buffer
	err := reportIngestion(&out, ingest.Result{Status: 200, Items: []ingest.Item{{Title: "Discarded"}}}, ingest.ErrCorpusFull)
	if err == nil || out.Len() != 0 || !strings.Contains(err.Error(), "retention review") {
		t.Fatal("false success", out.String(), err)
	}
}
