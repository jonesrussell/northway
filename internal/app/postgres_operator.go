package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/sqlite"
)

// Offline release operations never activate acquisition or create identities.
func executePostgresOperator(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected postgres import or install-register")
	}
	f := flag.NewFlagSet("postgres "+args[0], flag.ContinueOnError)
	f.SetOutput(out)
	source := f.String("source", "", "paused SQLite source (import only)")
	database := f.String("database", "", "postgres:/absolute/private/connection-file")
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 || !strings.HasPrefix(*database, "postgres:/") {
		return errors.New("explicit private PostgreSQL connection file required")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	switch args[0] {
	case "import":
		if *source == "" {
			return errors.New("paused SQLite source required")
		}
		counts, err := sqlite.ImportPostgres(ctx, *source, strings.TrimPrefix(*database, "postgres:"))
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(counts)
	case "install-register":
		if *source != "" {
			return errors.New("install-register takes no source")
		}
		store, err := sqlite.Open(ctx, *database)
		if err != nil {
			return err
		}
		defer store.Close()
		count, err := store.InstallReviewedPublicRegister(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]any{"installed": count, "enabled": false, "broad_sha256": sqlite.PublicBroadRegisterSHA256, "canary_sha256": sqlite.PublicCanaryRegisterSHA256})
	default:
		return errors.New("unknown postgres operation")
	}
}
