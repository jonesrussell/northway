package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"github.com/pressly/goose/v3"
)

// A postgres: path names a private DSN file, never a DSN in a flag or log.
func postgresPool(path string, size int) (*sql.DB, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("PostgreSQL connection file must be private and regular")
	}
	dir, e := os.Lstat(filepath.Dir(path))
	if e != nil || !dir.IsDir() || dir.Mode().Perm()&0077 != 0 {
		return nil, errors.New("PostgreSQL connection directory must be private")
	}
	raw, e := os.ReadFile(path)
	if e != nil || len(raw) > 8192 {
		return nil, errors.New("cannot read PostgreSQL connection configuration")
	}
	cfg, e := pgx.ParseConfig(strings.TrimSpace(string(raw)))
	if e != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	cfg.RuntimeParams["statement_timeout"] = "5000"
	cfg.RuntimeParams["lock_timeout"] = "2000"
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = "5000"
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(size)
	db.SetMaxIdleConns(size)
	db.SetConnMaxIdleTime(time.Minute)
	return db, nil
}

func migratePostgres(ctx context.Context, path string) error {
	db, e := postgresPool(path, 1)
	if e != nil {
		return e
	}
	defer db.Close()
	conn, e := db.Conn(ctx)
	if e != nil {
		return errors.New("PostgreSQL unavailable")
	}
	defer conn.Close()
	var locked bool
	if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(762314201)").Scan(&locked); e != nil || !locked {
		return errors.New("PostgreSQL migration requires stopped application owners")
	}
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(762314201)")
	// Goose uses a second connection while the dedicated session owns the lock.
	db.SetMaxOpenConns(2)
	var exists bool
	if e = conn.QueryRowContext(ctx, "SELECT to_regclass('public.goose_db_version') IS NOT NULL").Scan(&exists); e != nil {
		return e
	}
	if exists {
		var v int
		if e = conn.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&v); e != nil {
			return e
		}
		if v > schemaVersion {
			return errors.New("database schema is newer than this binary")
		}
	}
	files, e := fs.Sub(assets.PostgresMigrations, "postgres")
	if e != nil {
		return e
	}
	provider, e := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithDisableGlobalRegistry(true))
	if e != nil {
		return e
	}
	_, e = provider.Up(ctx)
	return e
}

func openPostgres(ctx context.Context, path string) (*Store, error) {
	s := &Store{postgres: true, writeGate: make(chan struct{}, 1)}
	var e error
	s.writer, e = postgresPool(path, 4)
	if e == nil {
		s.readers, e = postgresPool(path, 2)
	}
	if e == nil {
		s.guard, e = s.writer.Conn(ctx)
	}
	if e == nil {
		var locked bool
		e = s.guard.QueryRowContext(ctx, "SELECT pg_try_advisory_lock_shared(762314201)").Scan(&locked)
		if e == nil && !locked {
			e = errors.New("PostgreSQL migration owns storage")
		}
	}
	if e == nil {
		e = s.Ready(ctx)
	}
	if e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}

func (s *Store) postgresReady(ctx context.Context) error {
	var v int
	if _, e := s.guard.ExecContext(ctx, "SELECT 1"); e != nil {
		return errors.New("PostgreSQL ownership connection lost")
	}
	if e := s.readers.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&v); e != nil {
		return e
	}
	if v != schemaVersion {
		return errors.New("storage schema does not match this binary; run migrate before serve")
	}
	var valid bool
	if e := s.readers.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_ts_config WHERE cfgname='northway_search')").Scan(&valid); e != nil {
		return e
	}
	if !valid {
		return errors.New("PostgreSQL search configuration unavailable")
	}
	return nil
}

// Feature services and their generated result types remain identical. This
// narrow DBTX boundary is private to this adapter, not a storage framework.
func (s *Store) database(db sqlc.DBTX) sqlc.DBTX {
	if s.postgres {
		return postgresDB{db}
	}
	return db
}
func (s *Store) queries(db sqlc.DBTX) *sqlc.Queries { return sqlc.New(s.database(db)) }

type postgresDB struct{ sqlc.DBTX }

func (d postgresDB) ExecContext(c context.Context, q string, a ...interface{}) (sql.Result, error) {
	return d.DBTX.ExecContext(c, postgresStatement(q), a...)
}
func (d postgresDB) PrepareContext(c context.Context, q string) (*sql.Stmt, error) {
	return d.DBTX.PrepareContext(c, postgresStatement(q))
}
func (d postgresDB) QueryContext(c context.Context, q string, a ...interface{}) (*sql.Rows, error) {
	return d.DBTX.QueryContext(c, postgresStatement(q), a...)
}
func (d postgresDB) QueryRowContext(c context.Context, q string, a ...interface{}) *sql.Row {
	return d.DBTX.QueryRowContext(c, postgresStatement(q), a...)
}

var postgresParameter = regexp.MustCompile(`\?([0-9]*)`)

func postgresStatement(q string) string {
	// Only repository-owned SQL reaches this boundary; values remain parameters.
	q = strings.ReplaceAll(q, "FROM article_fts JOIN articles a ON a.rowid=article_fts.rowid", "FROM articles a")
	q = strings.ReplaceAll(q, "a.rowid NOT IN (SELECT rowid FROM article_fts WHERE article_fts MATCH ?)", "NOT (a.title_vector @@ northway_match(?))")
	q = strings.ReplaceAll(q, "a.rowid IN (SELECT rowid FROM article_fts WHERE article_fts MATCH ?)", "a.title_vector @@ northway_match(?)")
	q = strings.ReplaceAll(q, "article_fts MATCH ?", "a.search_vector @@ northway_match(?)")
	q = strings.ReplaceAll(q, "SELECT value FROM json_each(?)", "SELECT jsonb_array_elements_text(?::jsonb)")
	q = strings.ReplaceAll(q, "JOIN json_each(fd.preferences,'$.sources') r", "CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN fd.preferences='' THEN '{}'::jsonb ELSE fd.preferences::jsonb END -> 'sources') r(value)")
	q = strings.ReplaceAll(q, "json_extract(r.value,'$.source_id')", "(r.value->>'source_id')")
	q = strings.ReplaceAll(q, " AS INTEGER)", " AS BIGINT)")
	q = strings.ReplaceAll(q, "FROM (SELECT charged_bytes FROM poll_attempts UNION ALL SELECT charged_bytes FROM erased_acquisition_usage)", "FROM (SELECT charged_bytes FROM poll_attempts UNION ALL SELECT charged_bytes FROM erased_acquisition_usage) accounting")
	if strings.Contains(q, "-- name: NextPollSources ") {
		q = strings.ReplaceAll(q, "ORDER BY ps.source_id LIMIT 100", "ORDER BY ps.source_id LIMIT 100 FOR UPDATE OF ps SKIP LOCKED")
	}
	q = strings.ReplaceAll(q, "WHERE window <", "WHERE budget_window <")
	q = strings.ReplaceAll(q, "tenant_id,window", "tenant_id,budget_window")
	q = strings.ReplaceAll(q, "used=used+1 WHERE used<60", "used=request_budgets.used+1 WHERE request_budgets.used<60")
	if strings.HasPrefix(q, "-- name:") {
		q = strings.ReplaceAll(q, "max(", "greatest(")
	}
	q = strings.ReplaceAll(q, "next_at=greatest(next_at,excluded.next_at)", "next_at=greatest(collection_hosts.next_at,excluded.next_at)")
	n := 0
	q = postgresParameter.ReplaceAllStringFunc(q, func(v string) string {
		if len(v) > 1 {
			return "$" + v[1:]
		}
		n++
		return "$" + strconv.Itoa(n)
	})
	return q
}
func (s *Store) hostHold(at int64, until time.Time) int64 {
	if s.postgres {
		return until.UnixMicro()
	}
	return at + int64(10*time.Second/time.Microsecond)
}
