package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/jonesrussell/northway/internal/fetch"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/source"
	"github.com/jonesrussell/northway/internal/sqlite"
	"io"
	"os"
	"path/filepath"
	"time"
)

const fixtureTenant identity.TenantID = "00000001-0000-4000-8000-000000000000"
const fixtureSource = "00000003-0000-4000-8000-000000000000"
const fixturePage = "https://fixture.example/creator"

type fixtureCollector struct{ body []byte }

func (f fixtureCollector) FetchCollection(ctx context.Context, c ingest.Claim) ingest.Result {
	o, e := fetch.ExtractCreator(ctx, c.URL, f.body, c.PreviewAllowed)
	r := ingest.Result{Status: 200, Bytes: int64(len(f.body)), Observations: o}
	if e != nil {
		r.Observations = nil
		r.Failure = "parse"
	}
	return r
}

// executeCollection provides local fixture qualification and operator cursor export.
// It deliberately exposes no network run, schedule, approval or provisioning API.
func executeCollection(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected collection fixture|export")
	}
	fs := flag.NewFlagSet("collection", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var path, input, tenant, sourceID, url, title string
	var after int64
	fs.StringVar(&path, "database", "", "local database")
	fs.StringVar(&input, "input", "", "controlled HTML fixture")
	fs.StringVar(&tenant, "tenant", string(fixtureTenant), "operator tenant UUID")
	fs.StringVar(&sourceID, "source", "", "explicit source UUID")
	fs.StringVar(&url, "url", "", "approved seed URL")
	fs.StringVar(&title, "title", "", "seed title")
	fs.Int64Var(&after, "after", 0, "committed cursor")
	if e := fs.Parse(args[1:]); e != nil || fs.NArg() != 0 || path == "" {
		return errors.New("collection requires --database and valid flags")
	}
	if args[0] == "fixture" {
		if filepath.Base(path) != "northway-collection.fixture.sqlite" || input == "" || tenant != string(fixtureTenant) || after != 0 {
			return errors.New("fixture requires new northway-collection.fixture.sqlite and controlled --input")
		}
		if _, e := os.Lstat(path); !errors.Is(e, os.ErrNotExist) {
			return errors.New("fixture database must not exist")
		}
		f, e := os.Open(input)
		if e != nil {
			return errors.New("fixture input unavailable")
		}
		defer f.Close()
		data, e := io.ReadAll(io.LimitReader(f, ingest.MaxResponseBytes))
		if e != nil || len(data) == 0 || int64(len(data)) >= ingest.MaxResponseBytes {
			return errors.New("fixture byte limit")
		}
		if e = sqlite.Migrate(ctx, path); e != nil {
			return e
		}
		s, e := sqlite.Open(ctx, path)
		if e != nil {
			return e
		}
		defer s.Close()
		p, _ := identity.Operator(fixtureTenant)
		if e = s.CreateTenant(ctx, fixtureTenant); e != nil {
			return e
		}
		if e = s.CreateSource(ctx, p, source.Source{ID: fixtureSource, URL: fixturePage, Title: "Controlled creator fixture"}); e != nil {
			return e
		}
		policy := ingest.Policy{SourceID: fixtureSource, URL: fixturePage, Mode: "html", Approved: true, Enabled: true, PreviewAllowed: true, Interval: time.Hour, MaxBytes: ingest.MaxResponseBytes, RobotsUntil: time.Now().Add(time.Hour)}
		if e = s.ConfigurePoll(ctx, p, policy); e != nil {
			return e
		}
		if _, e = ingest.NewCollector(s, fixtureCollector{data}).RunOnce(ctx, p); e != nil {
			return e
		}
		policy.Enabled = false
		if e = s.ConfigurePoll(ctx, p, policy); e != nil {
			return e
		}
		batch, e := s.CollectionBatch(ctx, p, 0)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(batch)
	}
	if (args[0] != "export" && args[0] != "seed" && args[0] != "status") || input != "" {
		return errors.New("expected collection fixture|export")
	}
	p, e := identity.Operator(identity.TenantID(tenant))
	if e != nil {
		return e
	}
	s, e := sqlite.Open(ctx, path)
	if e != nil {
		return e
	}
	defer s.Close()
	if args[0] == "seed" {
		if after != 0 {
			return errors.New("seed takes no cursor")
		}
		if e = s.AddCollectionSeed(ctx, p, ingest.CollectionSeed{ID: sourceID, URL: url, Title: title}); e != nil {
			return e
		}
		v, e := s.CollectionStatus(ctx, p)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	}
	if sourceID != "" || url != "" || title != "" {
		return errors.New("unexpected seed fields")
	}
	if args[0] == "status" {
		if after != 0 {
			return errors.New("status takes no cursor")
		}
		v, e := s.CollectionStatus(ctx, p)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	}
	b, e := s.CollectionBatch(ctx, p, after)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(b)
}
