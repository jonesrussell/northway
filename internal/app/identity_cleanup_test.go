package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonesrussell/northway/internal/identity"
)

type failingKeyCreator struct{ directory string }

func (s failingKeyCreator) CreateAPIKey(context.Context, identity.Principal, identity.KeyRecord) error {
	// The handoff file already exists. Remove directory write permission so the
	// real unlink fails during deferred cleanup; no credential enters a database.
	if err := os.Chmod(s.directory, 0500); err != nil {
		return err
	}
	return errors.New("simulated storage failure with private diagnostics")
}

func TestKeyCleanupFailureIsReportedWithoutSecret(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the directory permissions used by this failure test")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(directory, 0700); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(directory, "client.key")
	principal, err := identity.Operator("00000001-0000-4000-8000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	_, err = writeNewKey(t.Context(), failingKeyCreator{directory}, principal, identity.FeedsRead, path)
	if err == nil || !strings.Contains(err.Error(), "key provisioning failed") || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatal("cleanup failure was not reported")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal("failed removal fixture did not leave the private handoff")
	}
	if strings.Contains(err.Error(), strings.TrimSpace(string(data))) || strings.Contains(err.Error(), "private diagnostics") || strings.Contains(err.Error(), path) {
		t.Fatal("cleanup error leaked private data")
	}
	info, statErr := os.Stat(path)
	if statErr != nil || info.Mode().Perm() != 0600 {
		t.Fatal("failed cleanup changed private file permissions")
	}
}
