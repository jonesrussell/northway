package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/fetch"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
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
