// Package sqlite owns all database handles, transactions and generated bindings.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

const schemaVersion = 4
const busyMilliseconds = 50

// Store is a single-process SQLite owner. Close only after all users have stopped.
// No database handles or transaction callbacks escape this package.
type Store struct {
	writer, readers *sql.DB
	file            *os.File
	writeGate       chan struct{}
	closeOnce       sync.Once
	closeErr        error
	clock           func() time.Time // test clock; immutable while the store is in use
}

func lockFile(path string, create bool) (*os.File, string, error) {
	if path == "" || path == ":memory:" {
		return nil, "", errors.New("a local database file path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(abs)
	if create {
		if err := os.MkdirAll(parent, 0700); err != nil {
			return nil, "", err
		}
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, "", errors.New("database directory must be private (0700) and not a symlink")
	}
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	// O_NOFOLLOW prevents a final-component symlink substitution. The private
	// directory and operator-controlled ancestors are part of the file boundary.
	file, err := os.OpenFile(abs, flags|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, "", fmt.Errorf("open database file: %w", err)
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		file.Close()
		return nil, "", errors.New("database file must be private (0600) and regular")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, "", fmt.Errorf("database already owned by another process: %w", err)
	}
	return file, abs, nil
}

func openPool(path string, readOnly bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rw")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(50)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "cache_size(-2048)")
	if readOnly {
		q.Add("_pragma", "query_only(1)")
	} else {
		q.Set("_txlock", "immediate")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	size := 1
	if readOnly {
		size = 2
	}
	db.SetMaxOpenConns(size)
	db.SetMaxIdleConns(size)
	return db, nil
}

func provider(db *sql.DB, migrations fs.FS) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectSQLite3, db, migrations,
		goose.WithSlog(slog.New(slog.NewJSONHandler(io.Discard, nil))), goose.WithDisableGlobalRegistry(true))
}

// Migrate runs forward-only, embedded Goose migrations with exclusive ownership.
// It must complete before serve. It never runs as a side effect of Open.
func Migrate(ctx context.Context, path string) error {
	file, abs, err := lockFile(path, true)
	if err != nil {
		return err
	}
	defer file.Close()
	db, err := openPool(abs, false)
	if err != nil {
		return err
	}
	defer db.Close()
	var hasVersions int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'").Scan(&hasVersions); err != nil {
		return err
	}
	if hasVersions != 0 {
		var version int
		if err := db.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied=1").Scan(&version); err != nil {
			return err
		}
		if version > schemaVersion {
			return errors.New("database schema is newer than this binary")
		}
	}
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journal); err != nil {
		return err
	}
	if journal != "wal" {
		return errors.New("WAL mode unavailable")
	}
	migrations, err := fs.Sub(assets.Migrations, "migrations")
	if err != nil {
		return err
	}
	p, err := provider(db, migrations)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	var check string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return errors.New("database integrity check failed")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("database foreign key integrity check failed")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO article_fts(article_fts,rank) VALUES('integrity-check',1)")
	return err
}

// Open requires an existing, fully migrated local file and locks its ownership.
func Open(ctx context.Context, path string) (*Store, error) {
	file, abs, err := lockFile(path, false)
	if err != nil {
		return nil, err
	}
	s := &Store{file: file, writeGate: make(chan struct{}, 1)}
	s.writer, err = openPool(abs, false)
	if err == nil {
		err = s.writer.PingContext(ctx)
	}
	if err == nil {
		s.readers, err = openPool(abs, true)
	}
	if err == nil {
		err = s.Ready(ctx)
	}
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("open storage: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		if s.readers != nil {
			s.closeErr = errors.Join(s.closeErr, s.readers.Close())
		}
		if s.writer != nil {
			s.closeErr = errors.Join(s.closeErr, s.writer.Close())
		}
		if s.file != nil {
			s.closeErr = errors.Join(s.closeErr, s.file.Close())
		}
	})
	return s.closeErr
}

// Ready checks real storage and schema capability, not end-to-end feed readiness.
func (s *Store) Ready(ctx context.Context) error {
	conn, err := s.readers.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var foreignKeys, busy, synchronous, fts, version int
	var journal string
	for _, probe := range []struct {
		sql  string
		dest any
	}{
		{"PRAGMA foreign_keys", &foreignKeys}, {"PRAGMA busy_timeout", &busy},
		{"PRAGMA synchronous", &synchronous}, {"PRAGMA journal_mode", &journal},
		{"SELECT sqlite_compileoption_used('ENABLE_FTS5')", &fts},
		{"SELECT max(version_id) FROM goose_db_version WHERE is_applied=1", &version},
	} {
		if err := conn.QueryRowContext(ctx, probe.sql).Scan(probe.dest); err != nil {
			return err
		}
	}
	if foreignKeys != 1 || busy != busyMilliseconds || synchronous != 2 || journal != "wal" || fts != 1 || version != schemaVersion {
		return errors.New("storage settings or schema do not match this binary; run migrate before serve")
	}
	rows, err := conn.QueryContext(ctx, `SELECT rowid FROM article_fts WHERE article_fts MATCH '"northway readiness"' LIMIT 1`)
	if err != nil {
		return err
	}
	rows.Next()
	return errors.Join(rows.Err(), rows.Close())
}

// write serializes writers before BEGIN IMMEDIATE; waiting callers can cancel.
// The driver's bounded busy handler deals with other SQLite clients without an
// application retry loop. Any failure rolls back both corpus and FTS changes.
func (s *Store) write(ctx context.Context, fn func(*sqlc.Queries) error) error {
	select {
	case s.writeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.writeGate }()
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer tx.Rollback()
	if err := fn(sqlc.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

// Diagnostics contains capability metadata only, never paths, SQL data or context.
func (s *Store) Diagnostics(ctx context.Context) (string, []string, error) {
	var version string
	if err := s.readers.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return "", nil, err
	}
	rows, err := s.readers.QueryContext(ctx, "PRAGMA compile_options")
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var options []string
	for rows.Next() {
		var option string
		if err := rows.Scan(&option); err != nil {
			return "", nil, err
		}
		options = append(options, option)
	}
	return version, options, rows.Err()
}
