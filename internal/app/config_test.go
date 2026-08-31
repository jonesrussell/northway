package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, ok := values[key]; return value, ok }
}

func TestConfigurationPrecedenceAndValidation(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		address string
		timeout time.Duration
		level   slog.Level
		invalid bool
	}{
		{name: "defaults bind privately", address: "127.0.0.1:8080", timeout: 10 * time.Second},
		{name: "environment", env: map[string]string{"NORTHWAY_LISTEN_ADDR": "[::1]:8090", "NORTHWAY_SHUTDOWN_TIMEOUT": "3s", "NORTHWAY_LOG_LEVEL": "warn"}, address: "[::1]:8090", timeout: 3 * time.Second, level: slog.LevelWarn},
		{name: "flags override even invalid environment", env: map[string]string{"NORTHWAY_LISTEN_ADDR": "bad", "NORTHWAY_SHUTDOWN_TIMEOUT": "bad"}, args: []string{"--listen=127.0.0.1:0", "--shutdown-timeout=2s"}, address: "127.0.0.1:0", timeout: 2 * time.Second},
		{name: "explicit empty address", env: map[string]string{"NORTHWAY_LISTEN_ADDR": ""}, invalid: true},
		{name: "zero timeout", args: []string{"--shutdown-timeout=0s"}, invalid: true},
		{name: "negative timeout", args: []string{"--shutdown-timeout=-1s"}, invalid: true},
		{name: "excess timeout", args: []string{"--shutdown-timeout=61s"}, invalid: true},
		{name: "bad duration", args: []string{"--shutdown-timeout=tomorrow"}, invalid: true},
		{name: "invalid level", env: map[string]string{"NORTHWAY_LOG_LEVEL": ""}, invalid: true},
		{name: "bad port", args: []string{"--listen=127.0.0.1:65536"}, invalid: true},
		{name: "unbracketed IPv6", args: []string{"--listen=::1:8080"}, invalid: true},
		{name: "hostname is not an IP", args: []string{"--listen=example.com:80"}, invalid: true},
		{name: "unexpected argument", args: []string{"something"}, invalid: true},
		{name: "unknown option", args: []string{"--token=do-not-print"}, invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			got, err := ParseConfig(tt.args, environment(tt.env), &output)
			if tt.invalid {
				if err == nil {
					t.Fatal("accepted invalid configuration")
				}
				if strings.Contains(err.Error()+output.String(), "do-not-print") {
					t.Fatal("echoed invalid argument value")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ListenAddress != tt.address || got.ShutdownTimeout != tt.timeout || got.LogLevel != tt.level {
				t.Fatalf("unexpected config: %+v", got)
			}
		})
	}
}

func TestCLIHelpVersionAndUnsupportedWork(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"serve", "--help"}, {"migrate", "--help"}, {"version"}} {
		var out bytes.Buffer
		if err := Execute(context.Background(), args, environment(nil), &out, io.Discard); err != nil {
			t.Fatal(err)
		}
		if out.Len() == 0 {
			t.Fatal("missing command output")
		}
		if args[0] == "version" {
			var version map[string]string
			if err := json.Unmarshal(out.Bytes(), &version); err != nil {
				t.Fatal(err)
			}
			if version["version"] == "" || version["go"] == "" || version["arch"] == "" {
				t.Fatalf("missing version metadata: %v", version)
			}
		}
	}
	for _, args := range [][]string{nil, {"ingest"}, {"migrate"}, {"version", "extra"}, {"help", "extra"}, {"serve", "--bad"}} {
		if err := Execute(context.Background(), args, environment(nil), io.Discard, io.Discard); err == nil {
			t.Fatalf("unimplemented/invalid command succeeded: %v", args)
		}
	}
}

func TestDatabaseConfigurationAndMigrationFlags(t *testing.T) {
	config, err := ParseConfig([]string{"--database=chosen.sqlite"}, environment(map[string]string{"NORTHWAY_DATABASE_PATH": "other.sqlite"}), io.Discard)
	if err != nil || config.DatabasePath != "chosen.sqlite" {
		t.Fatalf("database precedence: %+v %v", config, err)
	}
	path, err := ParseMigrationPath(nil, environment(map[string]string{"NORTHWAY_DATABASE_PATH": "configured.sqlite"}), io.Discard)
	if err != nil || path != "configured.sqlite" {
		t.Fatalf("migration environment: %s %v", path, err)
	}
	for _, args := range [][]string{nil, {"--database="}, {"--database=x", "extra"}, {"--unknown=private"}} {
		_, err := ParseMigrationPath(args, environment(nil), io.Discard)
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("invalid migration flags: %v", err)
		}
	}
}
