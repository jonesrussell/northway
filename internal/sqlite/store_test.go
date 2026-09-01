package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/article"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/source"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	modern "modernc.org/sqlite"
)

const tenantA identity.TenantID = "00000001-0000-4000-8000-000000000000"
const tenantB identity.TenantID = "00000002-0000-4000-8000-000000000000"
const sourceID = "00000003-0000-4000-8000-000000000000"
const feedID = "00000004-0000-4000-8000-000000000000"
const itemID = "00000005-0000-4000-8000-000000000000"

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func fresh(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(privateDir(t), "northway.sqlite")
	must(t, Migrate(t.Context(), path))
	s, err := Open(t.Context(), path)
	must(t, err)
	t.Cleanup(func() { must(t, s.Close()) })
	return s, path
}
func seed(t *testing.T, s *Store, tenant identity.TenantID) {
	t.Helper()
	must(t, s.CreateTenant(t.Context(), tenant))
	must(t, s.CreateSource(t.Context(), operator(tenant), source.Source{ID: sourceID, URL: "https://example.invalid/feed", Title: "Fixture"}))
	must(t, s.CreateFeed(t.Context(), operator(tenant), feed.Feed{ID: feedID, Title: "Fixture"}))
	must(t, s.AttachSource(t.Context(), operator(tenant), feedID, sourceID))
}
func item() article.Article {
	return article.Article{ID: itemID, SourceID: sourceID, OriginID: "origin-1", URL: "https://example.invalid/item", Title: "PHP release", Body: "literal content", ObservedAt: time.Date(2026, 8, 30, 1, 2, 3, 456789000, time.UTC)}
}
func search(t *testing.T, s *Store, tenant identity.TenantID, term string) []article.Article {
	t.Helper()
	rows, err := s.Search(t.Context(), operator(tenant), feedID, term, time.Unix(0, 0), 10)
	must(t, err)
	return rows
}

func TestCreateTenantIsIdempotentAndPreservesState(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	must(t, s.PutArticle(t.Context(), operator(tenantA), item()))
	type state struct{ createdAt, corpusRevision, entitlementRevision int64 }
	read := func() (state, int64) {
		t.Helper()
		var got state
		must(t, s.readers.QueryRowContext(t.Context(), `
			SELECT created_at, corpus_revision, entitlement_revision FROM tenants WHERE id=?`, tenantA).
			Scan(&got.createdAt, &got.corpusRevision, &got.entitlementRevision))
		var count int64
		must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM tenants WHERE id=?", tenantA).Scan(&count))
		return got, count
	}
	before, beforeCount := read()
	if beforeCount != 1 {
		t.Fatalf("initial tenant count = %d, want 1", beforeCount)
	}
	must(t, s.CreateTenant(t.Context(), tenantA))
	after, count := read()
	if after != before || count != 1 {
		t.Fatalf("repeated tenant creation changed state: before=%+v after=%+v count=%d", before, after, count)
	}
	if _, err := s.GetArticle(t.Context(), operator(tenantA), itemID); err != nil {
		t.Fatalf("repeated tenant creation removed tenant data: %v", err)
	}
}

func TestTenantConstraintsAndFTSLifecycle(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	seed(t, s, tenantB)
	a := item()
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	got, err := s.GetArticle(t.Context(), operator(tenantA), a.ID)
	must(t, err)
	if got.PublishedAt != nil || !got.ObservedAt.Equal(a.ObservedAt) {
		t.Fatal("timestamp semantics changed", got)
	}
	if _, err := s.GetArticle(t.Context(), operator(tenantB), a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant read: %v", err)
	}
	if err := s.DeleteArticle(t.Context(), operator(tenantB), a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant delete: %v", err)
	}
	if len(search(t, s, tenantB, "PHP")) != 0 {
		t.Fatal("FTS crossed tenant")
	}
	if len(search(t, s, tenantA, "PHP")) != 1 {
		t.Fatal("FTS insert missing")
	}
	// Same IDs may exist in separate tenant scopes without contaminating results.
	b := item()
	b.Title = "Rust release"
	must(t, s.PutArticle(t.Context(), operator(tenantB), b))
	if len(search(t, s, tenantA, "Rust")) != 0 || len(search(t, s, tenantB, "PHP")) != 0 {
		t.Fatal("same-ID tenant isolation failed")
	}
	if err := s.AttachSource(t.Context(), operator(tenantA), feedID, "00000006-0000-4000-8000-000000000000"); err == nil {
		t.Fatal("missing source accepted")
	}
	must(t, s.CreateSource(t.Context(), operator(tenantB), source.Source{ID: "00000006-0000-4000-8000-000000000000", URL: "https://example.invalid/private", Title: "Private"}))
	bad := item()
	bad.ID = "00000008-0000-4000-8000-000000000000"
	bad.SourceID = "00000006-0000-4000-8000-000000000000"
	if err := s.PutArticle(t.Context(), operator(tenantA), bad); err == nil {
		t.Fatal("cross-tenant article/source relation accepted")
	}
	if err := s.AttachSource(t.Context(), operator(tenantA), feedID, "00000006-0000-4000-8000-000000000000"); err == nil {
		t.Fatal("cross-tenant relation accepted")
	}
	for _, missing := range []identity.TenantID{"", "invalid"} {
		if _, err := s.GetArticle(t.Context(), operator(missing), itemID); err == nil {
			t.Fatal("missing scope read accepted")
		}
		if err := s.PutArticle(t.Context(), operator(missing), a); err == nil {
			t.Fatal("missing scope write accepted")
		}
		if err := s.DeleteArticle(t.Context(), operator(missing), itemID); err == nil {
			t.Fatal("missing scope delete accepted")
		}
		if _, err := s.Search(t.Context(), operator(missing), feedID, "PHP", time.Unix(0, 0), 1); err == nil {
			t.Fatal("missing scope search accepted")
		}
	}
	a.Title = "Go release"
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	if len(search(t, s, tenantA, "PHP")) != 0 || len(search(t, s, tenantA, "Go")) != 1 {
		t.Fatal("FTS update inconsistent")
	}
	var versions int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions WHERE tenant_id=?", tenantA).Scan(&versions))
	if versions != 2 {
		t.Fatalf("versions=%d, want 2", versions)
	}
	a.OriginID = "changed"
	if err := s.PutArticle(t.Context(), operator(tenantA), a); err == nil {
		t.Fatal("identity mutation accepted")
	}
	must(t, s.DeleteArticle(t.Context(), operator(tenantA), itemID))
	if len(search(t, s, tenantA, "Go")) != 0 || len(search(t, s, tenantB, "Rust")) != 1 {
		t.Fatal("FTS delete crossed tenant or left stale index")
	}
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions WHERE tenant_id=?", tenantA).Scan(&versions))
	if versions != 0 {
		t.Fatal("versions were orphaned")
	}
	_, err = s.writer.ExecContext(t.Context(), "INSERT INTO article_fts(article_fts,rank) VALUES('integrity-check',1)")
	must(t, err)
}

func TestSearchBoundsAndMembership(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	a := item()
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	for _, term := range []string{"PHP OR Rust", `title:PHP`, `PHP* NOT Rust`, `" OR "`} {
		if len(search(t, s, tenantA, term)) != 0 {
			t.Fatalf("interpreted FTS operators: %q", term)
		}
	}
	for _, term := range []string{"", strings.Repeat("x", 257), "one two three four five six seven eight nine", "***"} {
		if _, err := s.Search(t.Context(), operator(tenantA), feedID, term, time.Unix(0, 0), 10); err == nil {
			t.Fatalf("accepted unbounded terms %q", term)
		}
	}
	if _, err := s.Search(t.Context(), operator(tenantA), feedID, "PHP", time.Unix(0, 0), 51); err == nil {
		t.Fatal("unbounded result count")
	}
	other := "00000007-0000-4000-8000-000000000000"
	must(t, s.CreateFeed(t.Context(), operator(tenantA), feed.Feed{ID: other, Title: "Empty"}))
	rows, err := s.Search(t.Context(), operator(tenantA), other, "PHP", time.Unix(0, 0), 1)
	must(t, err)
	if len(rows) != 0 {
		t.Fatal("feed membership omitted")
	}
	rows, err = s.Search(t.Context(), operator(tenantA), feedID, "PHP", a.ObservedAt.Add(time.Second), 1)
	must(t, err)
	if len(rows) != 0 {
		t.Fatal("age predicate omitted")
	}
}

func TestPragmasOnEveryConnectionAndReplacement(t *testing.T) {
	s, _ := fresh(t)
	var pageSize, maxPages, checkpointPages int64
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA page_size").Scan(&pageSize))
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA max_page_count").Scan(&maxPages))
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA wal_autocheckpoint").Scan(&checkpointPages))
	if pageSize != storagePageSize || maxPages != storageMaxPages || checkpointPages != walAutoCheckpointPages {
		t.Fatalf("writer bounds page_size=%d max_pages=%d checkpoint_pages=%d", pageSize, maxPages, checkpointPages)
	}
	for _, pool := range []*sql.DB{s.writer, s.readers} {
		count := pool.Stats().MaxOpenConnections
		for round := 0; round < 2; round++ {
			var conns []*sql.Conn
			for i := 0; i < count; i++ {
				c, err := pool.Conn(t.Context())
				must(t, err)
				conns = append(conns, c)
				for pragma, want := range map[string]int{"foreign_keys": 1, "busy_timeout": busyMilliseconds, "synchronous": 2} {
					var got int
					must(t, c.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got))
					if got != want {
						t.Fatalf("%s=%d", pragma, got)
					}
				}
				if pool == s.writer {
					for pragma, want := range map[string]int64{"max_page_count": storageMaxPages, "wal_autocheckpoint": walAutoCheckpointPages} {
						var got int64
						must(t, c.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got))
						if got != want {
							t.Fatalf("replacement %s=%d, want %d", pragma, got, want)
						}
					}
				}
				if pool == s.readers {
					if _, err := c.ExecContext(t.Context(), "CREATE TABLE forbidden(id INTEGER)"); err == nil {
						t.Fatal("read pool wrote")
					}
				}
			}
			pool.SetMaxIdleConns(0)
			for _, c := range conns {
				must(t, c.Close())
			}
			pool.SetMaxIdleConns(count)
		}
	}
	version, options, err := s.Diagnostics(t.Context())
	must(t, err)
	t.Logf("SQLite %s; compile options %v", version, options)
}

func TestWriteSerializationCancellationAndExternalLock(t *testing.T) {
	s, path := fresh(t)
	seed(t, s, tenantA)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- s.write(t.Context(), func(*sqlc.Queries) error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.PutArticle(ctx, operator(tenantA), item()); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting writer did not cancel: %v", err)
	}
	close(release)
	must(t, <-done)
	var wg sync.WaitGroup
	errorsCh := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Go(func() { errorsCh <- s.PutArticle(t.Context(), operator(tenantA), item()) })
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		must(t, err)
	}
	external, err := openPool(path, false)
	must(t, err)
	defer external.Close()
	tx, err := external.BeginTx(t.Context(), nil)
	must(t, err)
	defer tx.Rollback()
	start := time.Now()
	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err = s.PutArticle(ctx, operator(tenantA), item())
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("lock cancellation: %v after %s", err, time.Since(start))
	}
	must(t, tx.Rollback())
	must(t, s.PutArticle(t.Context(), operator(tenantA), item()))
	var replacementMax, replacementCheckpoint int64
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA max_page_count").Scan(&replacementMax))
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA wal_autocheckpoint").Scan(&replacementCheckpoint))
	if replacementMax != storageMaxPages || replacementCheckpoint != walAutoCheckpointPages {
		t.Fatalf("canceled writer replacement lost limits: max=%d checkpoint=%d", replacementMax, replacementCheckpoint)
	}
	// Saturated read pool also respects the caller's deadline.
	c1, err := s.readers.Conn(t.Context())
	must(t, err)
	defer c1.Close()
	c2, err := s.readers.Conn(t.Context())
	must(t, err)
	defer c2.Close()
	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := s.Ready(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read pool cancellation: %v", err)
	}
}

func TestReadyDoesNotWaitForWriter(t *testing.T) {
	s, _ := fresh(t)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.writeOperational(t.Context(), func(*sqlc.Queries) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	if err := s.Ready(ctx); err != nil {
		t.Fatalf("readiness contended with writer: %v", err)
	}
	close(release)
	must(t, <-done)
}

func TestSQLiteFullRollsBackCorpusVersionAndFTS(t *testing.T) {
	s, _ := fresh(t)
	s.reservePages = 0
	seed(t, s, tenantA)
	a := item()
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	var pages int
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA page_count").Scan(&pages))
	var maximum int
	must(t, s.writer.QueryRowContext(t.Context(), fmt.Sprintf("PRAGMA max_page_count=%d", pages+1)).Scan(&maximum))
	a.Body = strings.Repeat("oversized ", 6500)
	err := s.PutArticle(t.Context(), operator(tenantA), a)
	var sqliteErr *modern.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code()&255 != 13 {
		t.Fatalf("want real SQLITE_FULL, got %v", err)
	}
	got, err := s.GetArticle(t.Context(), operator(tenantA), itemID)
	must(t, err)
	if got.Body != "literal content" || len(search(t, s, tenantA, "oversized")) != 0 {
		t.Fatal("disk-full transaction partially committed")
	}
	var versions int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions").Scan(&versions))
	if versions != 1 {
		t.Fatal("failed version persisted")
	}
	must(t, s.writer.QueryRowContext(t.Context(), "PRAGMA max_page_count=2147483646").Scan(&maximum))
	a.Body = "recovered"
	must(t, s.PutArticle(t.Context(), operator(tenantA), a))
	_, err = s.writer.ExecContext(t.Context(), "INSERT INTO article_fts(article_fts,rank) VALUES('integrity-check',1)")
	must(t, err)
}

func TestUpgradeRebuildRestartAndExclusiveOwnership(t *testing.T) {
	for _, version := range []int64{1, 2, 3, 4, 5, 6, 7} {
		t.Run(fmt.Sprintf("schema-%d", version), func(t *testing.T) { testUpgradeRebuildRestart(t, version) })
	}
}
func testUpgradeRebuildRestart(t *testing.T, version int64) {
	path := filepath.Join(privateDir(t), "upgrade.sqlite")
	file, abs, err := lockFile(path, true)
	must(t, err)
	db, err := openPool(abs, false)
	must(t, err)
	_, err = db.ExecContext(t.Context(), "PRAGMA journal_mode=WAL")
	must(t, err)
	migrations, err := fs.Sub(assets.Migrations, "migrations")
	must(t, err)
	p, err := provider(db, migrations)
	must(t, err)
	_, err = p.UpTo(t.Context(), version)
	must(t, err)
	q := sqlc.New(db)
	must(t, q.CreateTenant(t.Context(), sqlc.CreateTenantParams{ID: string(tenantA), CreatedAt: 1}))
	must(t, q.CreateSource(t.Context(), sqlc.CreateSourceParams{TenantID: string(tenantA), ID: sourceID, Url: "https://example.invalid/feed", Title: "Before upgrade"}))
	_, err = q.PutArticle(t.Context(), sqlc.PutArticleParams{TenantID: string(tenantA), ID: itemID, SourceID: sourceID, OriginID: "before", Url: "https://example.invalid/item", Title: "PHP upgrade", Body: "preserved", ContentHash: strings.Repeat("a", 64), ObservedAt: 1})
	must(t, err)
	must(t, db.Close())
	must(t, file.Close())
	if old, err := Open(t.Context(), path); err == nil {
		old.Close()
		t.Fatal("unmigrated schema accepted")
	}
	must(t, Migrate(t.Context(), path))
	must(t, Migrate(t.Context(), path))
	s, err := Open(t.Context(), path)
	must(t, err)
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("concurrent serve accepted")
	}
	if err := Migrate(t.Context(), path); err == nil {
		t.Fatal("migration while serving accepted")
	}
	got, err := s.GetArticle(t.Context(), operator(tenantA), itemID)
	must(t, err)
	if got.Body != "preserved" {
		t.Fatal("upgrade lost data")
	}
	var n int
	must(t, s.readers.QueryRowContext(t.Context(), `SELECT count(*) FROM article_fts WHERE article_fts MATCH 'PHP'`).Scan(&n))
	if n != 1 {
		t.Fatal("upgrade failed to rebuild FTS")
	}
	must(t, s.Close())
	must(t, s.Close())
	s, err = Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	must(t, s.Ready(t.Context()))
	got, err = s.GetArticle(t.Context(), operator(tenantA), itemID)
	must(t, err)
	if got.Body != "preserved" {
		t.Fatal("restart lost data")
	}
}

func TestFailedMigrationRollsBackAndPathsFailClosed(t *testing.T) {
	path := filepath.Join(privateDir(t), "failed.sqlite")
	if s, err := Open(t.Context(), path); err == nil {
		s.Close()
		t.Fatal("serve created missing DB")
	}
	file, abs, err := lockFile(path, true)
	must(t, err)
	defer file.Close()
	db, err := openPool(abs, false)
	must(t, err)
	defer db.Close()
	p, err := provider(db, fstest.MapFS{"00001_broken.sql": {Data: []byte("-- +goose Up\nCREATE TABLE should_rollback(id INTEGER);\nINSERT INTO absent VALUES(1);\n")}})
	must(t, err)
	if _, err := p.Up(t.Context()); err == nil {
		t.Fatal("broken migration accepted")
	}
	var count int
	must(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name='should_rollback'").Scan(&count))
	if count != 0 {
		t.Fatal("partial migration persisted")
	}
	if err := Migrate(t.Context(), ":memory:"); err == nil {
		t.Fatal("memory database accepted")
	}
	link := filepath.Join(privateDir(t), "link.sqlite")
	must(t, os.Symlink(path, link))
	if _, _, err := lockFile(link, false); err == nil {
		t.Fatal("symlink database accepted")
	}
	public := privateDir(t)
	must(t, os.Chmod(public, 0755))
	t.Cleanup(func() { os.Chmod(public, 0700) })
	if err := Migrate(t.Context(), filepath.Join(public, "db.sqlite")); err == nil {
		t.Fatal("public database directory accepted")
	}
}

func privateDir(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	must(t, os.Chmod(p, 0700))
	return p
}

func TestNewerSchemaAndMalformedIDsFailClosed(t *testing.T) {
	s, path := fresh(t)
	_, err := s.writer.ExecContext(t.Context(), "INSERT INTO tenants(id,created_at) VALUES('not-a-uuid',1)")
	if err == nil {
		t.Fatal("database did not validate UUID")
	}
	_, err = s.writer.ExecContext(t.Context(), "INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,1)")
	must(t, err)
	if err := s.Ready(t.Context()); err == nil {
		t.Fatal("readiness accepted future schema")
	}
	must(t, s.Close())
	if err := Migrate(t.Context(), path); err == nil {
		t.Fatal("older migrator accepted newer schema")
	}
	if old, err := Open(t.Context(), path); err == nil {
		old.Close()
		t.Fatal("older server accepted newer schema")
	}
}

func TestRequireTenantChecksPersistedProvisioning(t *testing.T) {
	s, _ := fresh(t)
	defer s.Close()
	seed(t, s, tenantA)

	if err := s.RequireTenant(t.Context(), operator(tenantA)); err != nil {
		t.Fatalf("provisioned tenant rejected: %v", err)
	}
	if err := s.RequireTenant(t.Context(), operator(tenantB)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unprovisioned tenant error = %v, want ErrNotFound", err)
	}
	if err := s.RequireTenant(t.Context(), identity.Principal{}); err == nil {
		t.Fatal("invalid principal accepted")
	}
}

// Invalid fixture tenants deliberately yield the zero, unauthorized principal.
func operator(tenant identity.TenantID) identity.Principal {
	p, _ := identity.Operator(tenant)
	return p
}
