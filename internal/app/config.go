package app

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
)

// Config is validated before any listener is opened. It contains no secrets.
type Config struct {
	DatabasePath    string
	ListenAddress   string
	ShutdownTimeout time.Duration
	LogLevel        slog.Level
	PollTenant      identity.TenantID
}

// ParseConfig applies defaults, explicitly present environment values, then flags.
// Empty and zero values are validated rather than silently replaced by defaults.
func ParseConfig(args []string, lookup func(string) (string, bool), output io.Writer) (Config, error) {
	env := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			return value
		}
		return fallback
	}
	listen := env("NORTHWAY_LISTEN_ADDR", "127.0.0.1:8080")
	database := env("NORTHWAY_DATABASE_PATH", "")
	shutdown := env("NORTHWAY_SHUTDOWN_TIMEOUT", "10s")
	level := env("NORTHWAY_LOG_LEVEL", "info")
	pollTenant := env("NORTHWAY_POLL_TENANT", "")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&listen, "listen", listen, "IP:port to listen on")
	fs.StringVar(&database, "database", database, "existing migrated SQLite file; empty disables storage")
	fs.StringVar(&shutdown, "shutdown-timeout", shutdown, "maximum drain time")
	fs.StringVar(&level, "log-level", level, "debug, info, warn or error")
	fs.StringVar(&pollTenant, "poll-tenant", pollTenant, "explicit tenant UUID whose approved sources may be polled")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err := io.WriteString(output, serveHelp)
			if err != nil {
				return Config{}, err
			}
			return Config{}, flag.ErrHelp
		}
		return Config{}, errors.New("invalid serve flags; use 'northway serve --help'")
	}
	if fs.NArg() != 0 {
		return Config{}, errors.New("serve takes no positional arguments")
	}
	timeout, err := time.ParseDuration(shutdown)
	if err != nil {
		return Config{}, errors.New("shutdown timeout must be a duration")
	}
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}
	logLevel, ok := levels[level]
	if !ok {
		return Config{}, errors.New("log level must be debug, info, warn or error")
	}
	config := Config{DatabasePath: database, ListenAddress: listen, ShutdownTimeout: timeout, LogLevel: logLevel, PollTenant: identity.TenantID(pollTenant)}
	return config, config.Validate()
}

func (c Config) Validate() error {
	if _, err := netip.ParseAddrPort(c.ListenAddress); err != nil {
		return errors.New("listen address must be a literal IP:port (IPv6 in brackets)")
	}
	if c.ShutdownTimeout < time.Second || c.ShutdownTimeout > time.Minute {
		return errors.New("shutdown timeout must be between 1s and 1m")
	}
	switch c.LogLevel {
	case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError:
	default:
		return errors.New("unsupported log level")
	}
	if c.PollTenant != "" {
		if c.DatabasePath == "" {
			return errors.New("poll tenant requires configured storage")
		}
		if _, err := identity.Operator(c.PollTenant); err != nil {
			return errors.New("poll tenant must be a canonical tenant UUID")
		}
	}
	return nil
}

const serveHelp = `Usage: northway serve [flags]
  --database PATH          existing migrated SQLite file; NORTHWAY_DATABASE_PATH
  --listen IP:port          default 127.0.0.1:8080; NORTHWAY_LISTEN_ADDR
  --shutdown-timeout 10s    allowed 1s..1m; NORTHWAY_SHUTDOWN_TIMEOUT
  --log-level info          debug|info|warn|error; NORTHWAY_LOG_LEVEL
  --poll-tenant UUID        enable serial polling for one provisioned tenant; NORTHWAY_POLL_TENANT
Flags override explicitly present environment values. Port 0 is allowed for local tests.
Polling is disabled when poll-tenant is empty. It never enables a source or bypasses stored policy.
Readiness requires configured, usable storage.
`

func ParseMigrationPath(args []string, lookup func(string) (string, bool), output io.Writer) (string, error) {
	path, _ := lookup("NORTHWAY_DATABASE_PATH")
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&path, "database", path, "local SQLite file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, writeErr := io.WriteString(output, "Usage: northway migrate --database PATH\nUses NORTHWAY_DATABASE_PATH when the flag is absent. Stop serve before migrating.\n"); writeErr != nil {
				return "", writeErr
			}
			return "", flag.ErrHelp
		}
		return "", errors.New("invalid migrate flags; use 'northway migrate --help'")
	}
	if fs.NArg() != 0 || path == "" {
		return "", errors.New("migrate requires a database path and no positional arguments")
	}
	return path, nil
}
