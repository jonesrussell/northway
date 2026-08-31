package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"runtime"
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
		_, err := io.WriteString(stdout, "Usage: northway <serve|version>\nUse northway serve --help for configuration.\nIngestion and migrations are not implemented yet.\n")
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
	default:
		return errors.New("unknown command; use 'northway --help'")
	}
}
