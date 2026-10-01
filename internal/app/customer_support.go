package app

import (
	"context"
	"errors"
	"flag"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
	"io"
	"os"
	"path/filepath"
)

func executeCustomerSupport(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("customer requires export, suspend or delete")
	}
	action := args[0]
	f := flag.NewFlagSet("customer "+action, flag.ContinueOnError)
	f.SetOutput(out)
	database := f.String("database", "", "private offline database path")
	tenant := f.String("tenant", "", "explicit verified customer UUID")
	output := f.String("output", "", "new private export file")
	confirm := f.String("confirm-tenant", "", "repeat tenant UUID for suspension/deletion")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || *database == "" || identity.TenantID(*tenant).Validate() != nil {
		return errors.New("explicit database and customer UUID required")
	}
	if action != "export" {
		if *confirm != *tenant || *output != "" {
			return errors.New("repeat exact customer UUID with --confirm-tenant")
		}
		return sqlite.CustomerSupport(ctx, *database, identity.TenantID(*tenant), action, io.Discard)
	}
	info, err := os.Stat(filepath.Dir(*output))
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("export requires private output directory")
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = sqlite.CustomerSupport(ctx, *database, identity.TenantID(*tenant), action, file); err != nil {
		return err
	}
	return file.Sync()
}
