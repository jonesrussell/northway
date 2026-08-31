package app

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/netip"
	"time"
)

// Config is validated before any listener is opened. It contains no secrets.
type Config struct {
	ListenAddress   string
	ShutdownTimeout time.Duration
	LogLevel        slog.Level
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
	shutdown := env("NORTHWAY_SHUTDOWN_TIMEOUT", "10s")
	level := env("NORTHWAY_LOG_LEVEL", "info")
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&listen, "listen", listen, "IP:port to listen on")
	fs.StringVar(&shutdown, "shutdown-timeout", shutdown, "maximum drain time")
	fs.StringVar(&level, "log-level", level, "debug, info, warn or error")
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
	config := Config{ListenAddress: listen, ShutdownTimeout: timeout, LogLevel: logLevel}
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
	return nil
}

const serveHelp = `Usage: northway serve [flags]
  --listen IP:port          default 127.0.0.1:8080; NORTHWAY_LISTEN_ADDR
  --shutdown-timeout 10s    allowed 1s..1m; NORTHWAY_SHUTDOWN_TIMEOUT
  --log-level info          debug|info|warn|error; NORTHWAY_LOG_LEVEL
Flags override explicitly present environment values. Port 0 is allowed for local tests.
The foundation exposes /healthz and /readyz only; /readyz stays 503 until storage is wired.
`
