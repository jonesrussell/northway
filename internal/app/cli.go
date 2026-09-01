package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"runtime"
	"time"

	"github.com/jonesrussell/northway/internal/sqlite"
)

// Version and Revision can be supplied by the reproducible release build.
var Version = "dev"
var Revision = "unknown"

func Execute(ctx context.Context, args []string, lookup func(string) (string, bool), stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected a command; use 'northway --help'")
	}
	switch args[0] {
	case "help", "--help", "-h":
		if len(args) != 1 {
			return errors.New("help takes no arguments")
		}
		_, err := io.WriteString(stdout, "Usage: northway <serve|migrate|backup|healthcheck|tenant|key|ingest|pilot|version>\nUse northway serve --help, northway migrate --help, northway backup --help or northway key --help for configuration.\nIngestion is bounded; unattended polling requires an explicit serve --poll-tenant UUID.\n")
		return err
	case "version":
		if len(args) != 1 {
			return errors.New("version takes no arguments")
		}
		return json.NewEncoder(stdout).Encode(struct {
			Version  string `json:"version"`
			Revision string `json:"revision"`
			Go       string `json:"go"`
			OS       string `json:"os"`
			Arch     string `json:"arch"`
		}{Version, Revision, runtime.Version(), runtime.GOOS, runtime.GOARCH})
	case "migrate":
		path, err := ParseMigrationPath(args[1:], lookup, stdout)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		if err != nil {
			return err
		}
		migrationCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := sqlite.Migrate(migrationCtx, path); err != nil {
			return err
		}
		slog.New(slog.NewJSONHandler(stderr, nil)).Info("migrations complete")
		return nil
	case "backup":
		source, output, err := ParseBackupPaths(args[1:], lookup, stdout)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		if err != nil {
			return err
		}
		backupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := sqlite.Backup(backupCtx, source, output); err != nil {
			return err
		}
		slog.New(slog.NewJSONHandler(stderr, nil)).Info("backup complete")
		return nil
	case "healthcheck":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			_, err := io.WriteString(stdout, "Usage: northway healthcheck\nChecks the fixed storage-backed container readiness endpoint at 127.0.0.1:8080.\n")
			return err
		}
		if len(args) != 1 {
			return errors.New("healthcheck takes no arguments")
		}
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return checkLocalReadiness(probeCtx, localReadinessURL)
	case "serve":
		config, err := ParseConfig(args[1:], lookup, stdout)
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		if err != nil {
			return err
		}
		logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: config.LogLevel}))
		return Run(ctx, config, logger)
	case "ingest":
		return executeIngest(ctx, args[1:], lookup, stdout)
	case "pilot":
		return executePilot(ctx, args[1:], lookup, stdout)
	case "tenant", "key":
		return executeIdentity(ctx, args, lookup, stdout)
	default:
		return errors.New("unknown command; use 'northway --help'")
	}
}
