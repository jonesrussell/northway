package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	assets "github.com/jonesrussell/northway/db"
	"regexp"
	"strings"
)

// ImportPostgres is an offline, empty-target cutover primitive. It never creates
// a snapshot, rewrites the source, overwrites target records, or runs on startup.
// Operators must pause writers and obtain separate production cutover approval.
func ImportPostgres(ctx context.Context, sourcePath, connectionFile string) (map[string]int64, error) {
	if strings.HasPrefix(sourcePath, "postgres:") {
		return nil, errors.New("import requires a SQLite source")
	}
	src, e := Open(ctx, sourcePath)
	if e != nil {
		return nil, e
	}
	defer src.Close()
	dst, e := openPostgres(ctx, connectionFile)
	if e != nil {
		return nil, e
	}
	defer dst.Close()
	var exclusive bool
	if e = dst.guard.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(762314201)").Scan(&exclusive); e != nil || !exclusive {
		return nil, errors.New("import requires stopped PostgreSQL application owners")
	}
	defer dst.guard.ExecContext(context.Background(), "SELECT pg_advisory_unlock(762314201)")
	names, e := assets.Migrations.ReadDir("migrations")
	if e != nil {
		return nil, e
	}
	tables := []string{}
	pattern := regexp.MustCompile(`(?i)CREATE TABLE\s+(\w+)\s*\(`)
	for _, name := range names {
		raw, e := assets.Migrations.ReadFile("migrations/" + name.Name())
		if e != nil {
			return nil, e
		}
		for _, m := range pattern.FindAllStringSubmatch(string(raw), -1) {
			tables = append(tables, m[1])
		}
	}
	// query_work references completed snapshots defined later in SQLite DDL.

	for i, n := range tables {
		if n == "query_snapshots" {
			for j, m := range tables {
				if m == "query_work" && j < i {
					copy(tables[j+1:i+1], tables[j:i])
					tables[j] = n
					break
				}
			}
			break
		}
	}
	tx, e := dst.writer.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(762314202)"); e != nil {
		return nil, e
	}
	counts := map[string]int64{}
	for _, table := range tables {
		var count int64
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); e != nil {
			return nil, e
		}
		if count != 0 {
			return nil, errors.New("PostgreSQL import target must be empty")
		}
	}
	for _, table := range []string{"public_register_exclusions", "public_sources", "public_poll_sources", "public_poll_attempts", "public_articles", "public_article_versions", "workspace_public_subscriptions", "public_query_scopes"} {
		var count int64
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); e != nil {
			return nil, e
		}
		if count != 0 {
			return nil, errors.New("PostgreSQL import target must be empty, including public ownership")
		}
	}
	// USER triggers only: foreign keys stay enforced. Preserve imported revisions
	// instead of incrementing them once per imported row.
	for _, table := range tables {
		if _, e = tx.ExecContext(ctx, "ALTER TABLE "+table+" DISABLE TRIGGER USER"); e != nil {
			return nil, e
		}
	}
	for _, table := range tables {
		counts[table] = 0
		rows, e := src.readers.QueryContext(ctx, "SELECT * FROM "+table)
		if e != nil {
			return nil, e
		}
		cols, e := rows.Columns()
		if e != nil {
			rows.Close()
			return nil, e
		}
		target := make([]string, len(cols))
		params := make([]string, len(cols))
		for i, c := range cols {
			if !regexp.MustCompile(`^[a-z_]+$`).MatchString(c) {
				rows.Close()
				return nil, errors.New("unexpected source column")
			}
			if table == "request_budgets" && c == "window" {
				c = "budget_window"
			}
			target[i] = `"` + c + `"`
			params[i] = fmt.Sprintf("$%d", i+1)
		}
		stmt, e := tx.PrepareContext(ctx, "INSERT INTO "+table+" ("+strings.Join(target, ",")+") VALUES ("+strings.Join(params, ",")+")")
		if e != nil {
			rows.Close()
			return nil, e
		}
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if e = rows.Scan(ptrs...); e != nil {
				stmt.Close()
				rows.Close()
				return nil, e
			}
			if _, e = stmt.ExecContext(ctx, values...); e != nil {
				stmt.Close()
				rows.Close()
				return nil, e
			}
			counts[table]++
		}
		e = errors.Join(rows.Err(), rows.Close(), stmt.Close())
		if e != nil {
			return nil, e
		}
		var copied int64
		if e = tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&copied); e != nil {
			return nil, e
		}
		if copied != counts[table] {
			return nil, errors.New("import count mismatch")
		}
	}
	for _, table := range tables {
		if _, e = tx.ExecContext(ctx, "ALTER TABLE "+table+" ENABLE TRIGGER USER"); e != nil {
			return nil, e
		}
	}
	for _, v := range []struct{ table, column string }{{"articles", "rowid"}, {"collection_events", "sequence"}} {
		var next sql.NullInt64
		if e = tx.QueryRowContext(ctx, "SELECT max("+v.column+") FROM "+v.table).Scan(&next); e != nil {
			return nil, e
		}
		if next.Valid {
			if _, e = tx.ExecContext(ctx, "SELECT setval(pg_get_serial_sequence($1,$2),$3,true)", v.table, v.column, next.Int64); e != nil {
				return nil, e
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return counts, nil
}
