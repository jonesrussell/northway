package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
)

const identityHelp = `Usage:
  northway tenant create --database PATH --tenant UUID
  northway key create --database PATH --tenant UUID --scopes feeds:read,feedback:write --output PATH
  northway key revoke --database PATH --tenant UUID --key-id ID
Database defaults to NORTHWAY_DATABASE_PATH. Stop serve first; migrations must be current.
Choose only needed scopes. New keys go to a new 0600 file in an existing private 0700 directory.
No secret is accepted on the command line, printed, or returned by HTTP.
`

func executeIdentity(ctx context.Context, args []string, lookup func(string) (string, bool), stdout io.Writer) error {
	if len(args) == 2 && (args[1] == "--help" || args[1] == "help") {
		_, err := io.WriteString(stdout, identityHelp)
		return err
	}
	if len(args) < 2 || (args[0] == "tenant" && args[1] != "create") || (args[0] == "key" && args[1] != "create" && args[1] != "revoke") {
		return errors.New("unknown identity command; use 'northway key --help'")
	}
	path, _ := lookup("NORTHWAY_DATABASE_PATH")
	var tenant, scopes, output, keyID string
	fs := flag.NewFlagSet("identity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&path, "database", path, "local database")
	fs.StringVar(&tenant, "tenant", "", "tenant UUID")
	if args[0] == "key" && args[1] == "create" {
		fs.StringVar(&scopes, "scopes", "", "explicit scopes")
		fs.StringVar(&output, "output", "", "new private key file")
	}
	if args[1] == "revoke" {
		fs.StringVar(&keyID, "key-id", "", "nonsecret key ID")
	}
	if err := fs.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err := io.WriteString(stdout, identityHelp)
			return err
		}
		return errors.New("invalid identity flags; use 'northway key --help'")
	}
	if path == "" || fs.NArg() != 0 {
		return errors.New("identity command requires a database and no positional arguments")
	}
	principal, err := identity.Operator(identity.TenantID(tenant))
	if err != nil {
		return errors.New("identity command requires a canonical tenant UUID")
	}
	var granted identity.Scopes
	if args[0] == "key" && args[1] == "create" {
		granted, err = identity.ParseScopes(scopes)
		if err != nil || output == "" {
			return errors.New("key creation requires explicit valid scopes and an output path")
		}
	}
	if args[1] == "revoke" && !identity.ValidKeyID(keyID) {
		return errors.New("revocation requires a valid nonsecret key ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	store, err := sqlite.Open(ctx, path)
	if err != nil {
		return errors.New("cannot open identity storage; migrate first and stop serve")
	}
	defer store.Close()
	if args[0] == "tenant" {
		if err := store.CreateTenant(ctx, principal.TenantID()); err != nil {
			return errors.New("tenant provisioning failed")
		}
	} else if args[1] == "revoke" {
		if err := store.RevokeAPIKey(ctx, principal, keyID); err != nil {
			return errors.New("key unavailable in tenant scope or revocation failed")
		}
	} else {
		keyID, err = writeNewKey(ctx, store, principal, granted, output)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(stdout).Encode(struct {
		Status   string `json:"status"`
		TenantID string `json:"tenant_id"`
		KeyID    string `json:"key_id,omitempty"`
	}{"complete", tenant, keyID})
}

func writeNewKey(ctx context.Context, store *sqlite.Store, principal identity.Principal, scopes identity.Scopes, path string) (string, error) {
	// Ancestors are operator-controlled, just as for the private SQLite path.
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return "", errors.New("key output requires an existing private 0700 directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return "", errors.New("key output must be a new private file")
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	key, secret, err := identity.GenerateKey(principal, scopes)
	if err != nil {
		return "", err
	}
	if _, err = io.WriteString(file, secret.Reveal()+"\n"); err != nil {
		return "", errors.New("key output write failed")
	}
	if err = file.Sync(); err != nil {
		return "", errors.New("key output sync failed")
	}
	if err = file.Close(); err != nil {
		return "", errors.New("key output close failed")
	}
	directory, err := os.Open(parent)
	if err != nil {
		return "", errors.New("key output directory unavailable")
	}
	err = errors.Join(directory.Sync(), directory.Close())
	if err != nil {
		return "", errors.New("key output directory sync failed")
	}
	// Persist the credential only after its private handoff is durable. A crash
	// before this insert can leave an inactive file; reconcile uncertain outcomes.
	if err = store.CreateAPIKey(ctx, principal, key); err != nil {
		return "", errors.New("key provisioning failed")
	}
	keep = true
	return key.ID, nil
}
