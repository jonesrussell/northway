package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/jonesrussell/northway/internal/fetch"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/sqlite"
)

const ingestHelp = `Usage: northway ingest once --database PATH --tenant UUID
Attempt at most one due, individually approved feed. No retries or background timer.
Database defaults to NORTHWAY_DATABASE_PATH. Stop serve first; migrations must be current.
This command cannot add sources, approve URLs, load catalogues or enable collection.
An operator must provision an approved poll policy separately before any work is due.
`

func executeIngest(ctx context.Context, args []string, lookup func(string) (string, bool), out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		_, err := io.WriteString(out, ingestHelp)
		return err
	}
	if len(args) == 0 || args[0] != "once" {
		return errors.New("expected ingest once; use northway ingest --help")
	}
	path, _ := lookup("NORTHWAY_DATABASE_PATH")
	var tenant string
	fs := flag.NewFlagSet("ingest once", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&path, "database", path, "local database")
	fs.StringVar(&tenant, "tenant", "", "tenant UUID")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err := io.WriteString(out, ingestHelp)
			return err
		}
		return errors.New("invalid ingestion flags")
	}
	if path == "" || fs.NArg() != 0 {
		return errors.New("ingestion requires database and no positional arguments")
	}
	principal, err := identity.Operator(identity.TenantID(tenant))
	if err != nil {
		return errors.New("ingestion requires canonical tenant UUID")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		return errors.New("cannot open ingestion storage; migrate first and stop serve")
	}
	defer store.Close()
	result, err := ingest.New(store, fetch.New()).RunOnce(ctx, principal)
	return reportIngestion(out, result, err)
}

func reportIngestion(out io.Writer, result ingest.Result, err error) error {
	status := "complete"
	switch {
	case errors.Is(err, ingest.ErrCorpusFull):
		return errors.New("corpus admission limit reached; operator retention review required")
	case errors.Is(err, ingest.ErrIdle):
		status = "idle"
	case errors.Is(err, ingest.ErrBudget):
		status = "budget_exhausted"
	case errors.Is(err, ingest.ErrBusy):
		status = "busy"
	case err != nil:
		return errors.New("ingestion failed; inspect bounded poll status before retrying")
	}
	// Do not expose publisher content, URLs, validators, raw errors or private paths.
	return json.NewEncoder(out).Encode(struct {
		Status     string `json:"status"`
		HTTPStatus int    `json:"http_status,omitempty"`
		Items      int    `json:"items"`
		Bytes      int64  `json:"bytes"`
	}{status, result.Status, len(result.Items), result.Bytes})
}
