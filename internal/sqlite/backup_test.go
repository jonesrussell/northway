package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/identity"
)

func TestBackupIncludesUncheckpointedWALAndRestores(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "source.sqlite")
	if err := Migrate(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	tenant := identity.TenantID("00000000-0000-4000-8000-000000000001")
	command := exec.Command(os.Args[0], "-test.run=TestBackupCrashWriter", "--")
	command.Env = append(os.Environ(), "NORTHWAY_CRASH_WRITER="+source, "NORTHWAY_CRASH_TENANT="+string(tenant))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash writer: %v: %s", err, output)
	}
	if info, err := os.Stat(source + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("fixture did not leave WAL content: %v", err)
	}

	backupDir := filepath.Join(dir, "backups")
	if err := os.Mkdir(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(backupDir, "northway.sqlite")
	if err := Backup(t.Context(), source, backup); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode: %v %v", info, err)
	}
	sourceStore, err := Open(t.Context(), source)
	if err != nil {
		t.Fatalf("source did not reopen after backup: %v", err)
	}
	sourcePrincipal, err := identity.Operator(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := sourceStore.RequireTenant(t.Context(), sourcePrincipal); err != nil {
		t.Fatalf("source data changed during backup: %v", err)
	}
	if err := sourceStore.Close(); err != nil {
		t.Fatal(err)
	}
	if matches, err := filepath.Glob(filepath.Join(backupDir, ".northway-backup-*")); err != nil || len(matches) != 0 {
		t.Fatalf("temporary backup files remain: %v %v", matches, err)
	}

	restoreDir := filepath.Join(dir, "restore")
	if err := os.Mkdir(restoreDir, 0700); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(restoreDir, "northway.sqlite")
	copyPrivateFile(t, backup, restored)
	if err := Migrate(t.Context(), restored); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), restored)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	principal, err := identity.Operator(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RequireTenant(t.Context(), principal); err != nil {
		t.Fatalf("restored WAL data missing: %v", err)
	}
}

func TestBackupSupportsPreUpgradeSchema(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "schema-six.sqlite")
	createBackupSchemaFixture(t, source, 6)
	backup := filepath.Join(dir, "schema-six-backup.sqlite")
	if err := Backup(t.Context(), source, backup); err != nil {
		t.Fatalf("supported older schema rejected: %v", err)
	}
	if err := Migrate(t.Context(), backup); err != nil {
		t.Fatalf("older-schema backup did not restore forward: %v", err)
	}
}

func TestBackupSupportsSchemaOneWithoutFTS(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "schema-one.sqlite")
	createBackupSchemaFixture(t, source, 1)
	backup := filepath.Join(dir, "schema-one-backup.sqlite")
	if err := Backup(t.Context(), source, backup); err != nil {
		t.Fatalf("schema one rejected: %v", err)
	}
	if err := Migrate(t.Context(), backup); err != nil {
		t.Fatalf("schema-one backup did not restore forward: %v", err)
	}
}

func TestBackupRejectsSchemaNewerThanBinary(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "newer.sqlite")
	if err := Migrate(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.writer.ExecContext(t.Context(), "INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,1)"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	err = Backup(t.Context(), source, filepath.Join(dir, "copy.sqlite"))
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("newer schema error = %v", err)
	}
}

func TestBackupRestorePreservesArticleFTS(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "source.sqlite")
	if err := Migrate(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	seed(t, store, tenantA)
	if err := store.PutArticle(t.Context(), operator(tenantA), item()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "backup.sqlite")
	if err := Backup(t.Context(), source, backup); err != nil {
		t.Fatal(err)
	}
	restoreDir := filepath.Join(dir, "restore-fts")
	if err := os.Mkdir(restoreDir, 0700); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(restoreDir, "northway.sqlite")
	copyPrivateFile(t, backup, restored)
	if err := Migrate(t.Context(), restored); err != nil {
		t.Fatal(err)
	}
	restoredStore, err := Open(t.Context(), restored)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredStore.Close()
	if got := search(t, restoredStore, tenantA, "PHP"); len(got) != 1 || got[0].ID != itemID {
		t.Fatalf("restored FTS results = %+v", got)
	}
}

func createBackupSchemaFixture(t *testing.T, path string, version int64) {
	t.Helper()
	file, absolute, err := lockFile(path, true)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openPool(absolute, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	migrations, err := fs.Sub(assets.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := provider(db, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCrashWriter(t *testing.T) {
	path := os.Getenv("NORTHWAY_CRASH_WRITER")
	if path == "" {
		return
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		os.Exit(2)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		os.Exit(3)
	}
	if _, err = db.Exec("INSERT INTO tenants(id,created_at) VALUES(?,?)", os.Getenv("NORTHWAY_CRASH_TENANT"), time.Now().UnixMicro()); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestBackupRefusesUnsafeOrUnavailableBoundaries(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "source.sqlite")
	if err := Migrate(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(dir, "backups")
	if err := os.Mkdir(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(backupDir, "northway.sqlite")

	store, err := Open(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err := Backup(t.Context(), source, output); err == nil {
		t.Fatal("live database backup accepted")
	}
	store.Close()
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed live backup published output")
	}

	if err := os.WriteFile(output, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Backup(t.Context(), source, output); err == nil {
		t.Fatal("existing output replaced")
	}
	if got, _ := os.ReadFile(output); string(got) != "owned" {
		t.Fatal("existing output changed")
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, output); err != nil {
		t.Fatal(err)
	}
	if err := Backup(t.Context(), source, output); err == nil {
		t.Fatal("symlink output accepted")
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if err := Backup(t.Context(), source, source); err == nil {
		t.Fatal("same source and output accepted")
	}

	public := filepath.Join(dir, "public")
	if err := os.Mkdir(public, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Backup(t.Context(), source, filepath.Join(public, "copy.sqlite")); err == nil {
		t.Fatal("public output directory accepted")
	}
	link := filepath.Join(dir, "backup-link")
	if err := os.Symlink(backupDir, link); err != nil {
		t.Fatal(err)
	}
	if err := Backup(t.Context(), source, filepath.Join(link, "copy.sqlite")); err == nil {
		t.Fatal("symlink output directory accepted")
	}
}

func TestVacuumStageReportsCancellation(t *testing.T) {
	dir := privateDir(t)
	source := filepath.Join(dir, "source.sqlite")
	if err := Migrate(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	db, err := openPool(source, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	output := filepath.Join(dir, "cancelled.sqlite")
	if err := vacuumInto(ctx, db, output); !errors.Is(err, context.Canceled) {
		t.Fatalf("vacuum cancellation error = %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled vacuum published output")
	}
}

func TestBackupFailureLeavesNoOutput(t *testing.T) {
	dir := privateDir(t)
	corrupt := filepath.Join(dir, "corrupt.sqlite")
	if err := os.WriteFile(corrupt, []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "copy.sqlite")
	if err := Backup(t.Context(), corrupt, output); err == nil {
		t.Fatal("corrupt source accepted")
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("corrupt backup published output")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Backup(ctx, corrupt, output); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".northway-backup-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func copyPrivateFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0600); err != nil {
		t.Fatal(err)
	}
}
