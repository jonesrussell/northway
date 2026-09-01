package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
)

// Backup creates a validated, coherent SQLite snapshot while the source is
// offline. The source ownership lock makes overlap with serve or migrate fail.
func Backup(ctx context.Context, source, output string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, sourcePath, err := lockFile(source, false)
	if err != nil {
		return err
	}
	defer file.Close()

	outputPath, err := filepath.Abs(output)
	if err != nil || output == "" || output == ":memory:" {
		return errors.New("a local backup output path is required")
	}
	if sourcePath == outputPath {
		return errors.New("backup output must differ from the source database")
	}
	parent := filepath.Dir(outputPath)
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("backup directory must already exist")
	}
	if err != nil {
		return errors.New("backup directory is unavailable")
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("backup directory must be private (0700) and not a symlink")
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return errors.New("backup output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("backup output is unavailable")
	}

	temporary, err := os.CreateTemp(parent, ".northway-backup-*.sqlite")
	if err != nil {
		return errors.New("create private backup staging file")
	}
	temporaryPath := temporary.Name()
	if closeErr := temporary.Close(); closeErr != nil {
		os.Remove(temporaryPath)
		return closeErr
	}
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	published := false
	defer func() {
		_ = os.Remove(temporaryPath)
		_ = os.Remove(temporaryPath + "-wal")
		_ = os.Remove(temporaryPath + "-shm")
		if err != nil && published {
			_ = os.Remove(outputPath)
		}
	}()

	db, err := openPool(sourcePath, false)
	if err != nil {
		return err
	}
	defer db.Close()
	sourceVersion, err := verifySnapshot(ctx, db)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if sourceVersion >= 2 {
		if _, err := db.ExecContext(ctx, "INSERT INTO article_fts(article_fts,rank) VALUES('integrity-check',1)"); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("source full-text index failed backup integrity checks")
		}
	}
	if err := vacuumInto(ctx, db, temporaryPath); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("create coherent SQLite backup")
	}
	if err := os.Chmod(temporaryPath, 0600); err != nil {
		return err
	}
	copyDB, err := openSnapshot(temporaryPath)
	if err != nil {
		return err
	}
	copyVersion, verifyErr := verifySnapshot(ctx, copyDB)
	closeErr := copyDB.Close()
	if err := errors.Join(verifyErr, closeErr); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("backup failed integrity verification")
	}
	if copyVersion != sourceVersion {
		return errors.New("backup schema version changed during snapshot")
	}
	copyFile, err := os.OpenFile(temporaryPath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	err = errors.Join(copyFile.Sync(), copyFile.Close())
	if err != nil {
		return err
	}
	// A hard link publishes without replacing an operator-owned destination.
	if err := os.Link(temporaryPath, outputPath); err != nil {
		return errors.New("publish backup without overwrite")
	}
	published = true
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	directory, err := os.Open(parent)
	if err != nil {
		return err
	}
	err = errors.Join(directory.Sync(), directory.Close())
	return err
}

func vacuumInto(ctx context.Context, db *sql.DB, destination string) error {
	_, err := db.ExecContext(ctx, "VACUUM INTO ?", destination)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func openSnapshot(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	query := u.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "query_only(1)")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err == nil {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	return db, err
}

func verifySnapshot(ctx context.Context, db *sql.DB) (int, error) {
	var check string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return 0, errors.New("database integrity check failed")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return 0, err
	}
	if rows.Next() {
		rows.Close()
		return 0, errors.New("database foreign key integrity check failed")
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	var version int
	if err := db.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version WHERE is_applied=1").Scan(&version); err != nil {
		return 0, err
	}
	if version < 1 {
		return 0, errors.New("database schema is not a supported Northway schema")
	}
	if version > schemaVersion {
		return 0, errors.New("database schema is newer than this backup binary")
	}
	return version, nil
}
