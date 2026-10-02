package app

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
)

// Config is validated before any listener is opened. It contains no secrets.
type Config struct {
	DatabasePath      string
	ListenAddress     string
	ShutdownTimeout   time.Duration
	LogLevel          slog.Level
	PollTenant        identity.TenantID
	AssertionIssuer   string
	AssertionAudience string
	AssertionKeys     string // Public verification material only, never private seeds.
	CustomerCatalogue string
	CollectionAPI     bool
	PublicPolling     bool
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
	collectionAPI := false
	fs.BoolVar(&collectionAPI, "collection-api", false, "enable tenant-scoped agent collection adapter; no grant issuance")
	publicPolling := false
	fs.BoolVar(&publicPolling, "public-polling", false, "run approved shared public feeds serially; PostgreSQL shared-v1 only")
	fs.StringVar(&listen, "listen", listen, "IP:port to listen on")
	fs.StringVar(&database, "database", database, "migrated SQLite file or postgres: private connection-file path; empty disables storage")
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
	config.CollectionAPI = collectionAPI
	config.PublicPolling = publicPolling
	config.AssertionIssuer = env("NORTHCLOUD_ASSERTION_ISSUER", "")
	config.AssertionAudience = env("NORTHCLOUD_ASSERTION_AUDIENCE", "")
	config.AssertionKeys = env("NORTHCLOUD_ASSERTION_KEYS", "")
	config.CustomerCatalogue = env("NORTHCLOUD_CATALOGUE", "")
	return config, config.Validate()
}

func (c Config) Validate() error {
	if c.PublicPolling && (c.CustomerCatalogue != "shared-v1" || !strings.HasPrefix(c.DatabasePath, "postgres:") || c.PollTenant != "") {
		return errors.New("public polling requires shared-v1 PostgreSQL and no tenant polling")
	}
	if c.CollectionAPI && c.DatabasePath == "" {
		return errors.New("collection API requires migrated storage")
	}
	if c.CustomerCatalogue != "" && ((c.CustomerCatalogue != "developer-v1" && c.CustomerCatalogue != "shared-v1") || c.AssertionKeys == "" || c.PollTenant != "") {
		return errors.New("customer catalogue requires assertion keys and exclusive customer polling mode")
	}
	if c.CustomerCatalogue == "shared-v1" && !strings.HasPrefix(c.DatabasePath, "postgres:") {
		return errors.New("shared catalogue requires PostgreSQL storage")
	}
	if c.CustomerCatalogue == "developer-v1" && strings.HasPrefix(c.DatabasePath, "postgres:") {
		return errors.New("PostgreSQL uses shared-v1 catalogue")
	}
	if _, err := c.verificationKeys(); err != nil {
		return err
	}
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

func (c Config) verificationKeys() (map[string]ed25519.PublicKey, error) {
	if c.AssertionIssuer == "" && c.AssertionAudience == "" && c.AssertionKeys == "" {
		return nil, nil
	}
	if c.DatabasePath == "" || c.AssertionIssuer == "" || c.AssertionAudience == "" || len(c.AssertionIssuer) > 256 || len(c.AssertionAudience) > 256 || len(c.AssertionKeys) > 1024 {
		return nil, errors.New("complete customer assertion configuration and storage are required")
	}
	var raw map[string]string
	if json.Unmarshal([]byte(c.AssertionKeys), &raw) != nil || len(raw) == 0 || len(raw) > 4 {
		return nil, errors.New("invalid customer public-key set")
	}
	keys := map[string]ed25519.PublicKey{}
	for id, value := range raw {
		key, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil || id == "" || len(id) > 64 || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("invalid customer public key")
		}
		keys[id] = key
	}
	return keys, nil
}

const serveHelp = `Usage: northway serve [flags]
  --database PATH          existing migrated SQLite file; NORTHWAY_DATABASE_PATH
  --listen IP:port          default 127.0.0.1:8080; NORTHWAY_LISTEN_ADDR
  --shutdown-timeout 10s    allowed 1s..1m; NORTHWAY_SHUTDOWN_TIMEOUT
  --log-level info          debug|info|warn|error; NORTHWAY_LOG_LEVEL
  --poll-tenant UUID        enable serial polling for one provisioned tenant; NORTHWAY_POLL_TENANT
  --public-polling          enable serial shared PostgreSQL polling; no tenant or grant
Flags override explicitly present environment values. Port 0 is allowed for local tests.
Polling is disabled when poll-tenant is empty and public-polling is absent. It never enables a source or bypasses stored policy.
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

func ParseBackupPaths(args []string, lookup func(string) (string, bool), output io.Writer) (string, string, error) {
	database, _ := lookup("NORTHWAY_DATABASE_PATH")
	var destination string
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&database, "database", database, "offline SQLite source file")
	fs.StringVar(&destination, "output", "", "new coherent SQLite snapshot file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if _, writeErr := io.WriteString(output, "Usage: northway backup --database PATH --output PATH\nUses NORTHWAY_DATABASE_PATH when --database is absent. Stop serve before backup. Output must not exist.\n"); writeErr != nil {
				return "", "", writeErr
			}
			return "", "", flag.ErrHelp
		}
		return "", "", errors.New("invalid backup flags; use 'northway backup --help'")
	}
	if fs.NArg() != 0 || database == "" || destination == "" {
		return "", "", errors.New("backup requires database and output paths and no positional arguments")
	}
	return database, destination, nil
}
