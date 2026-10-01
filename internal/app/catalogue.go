package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/jonesrussell/northway/internal/catalogue"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
	"io"
	"os"
	"time"
)

const catalogueHelp = "Usage: northway catalogue prepare --research PATH --output PATH [--limit 10]\n       northway catalogue validate --manifest PATH --sha256 DIGEST\n       northway catalogue inventory --database PATH --tenant UUID\n       northway catalogue provision --database PATH --tenant UUID --manifest PATH --sha256 DIGEST\nPrepare is inert and retains restrictions; provision requires offline exclusive storage ownership, creates no tenant/grant/timer, and activates only explicitly reviewed enabled policy.\n"

func executeCatalogue(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		_, e := io.WriteString(out, catalogueHelp)
		return e
	}
	if len(args) == 0 {
		return errors.New("expected catalogue command")
	}
	fs := flag.NewFlagSet("catalogue", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var research, output, manifest, digest, database, tenant string
	var limit int
	fs.StringVar(&research, "research", "", "typed research JSON")
	fs.StringVar(&output, "output", "", "new register file")
	fs.StringVar(&manifest, "manifest", "", "curated register")
	fs.StringVar(&digest, "sha256", "", "exact reviewed register digest")
	fs.StringVar(&database, "database", "", "offline database")
	fs.StringVar(&tenant, "tenant", "", "operator tenant")
	fs.IntVar(&limit, "limit", 10, "inert canary candidates")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return errors.New("invalid catalogue flags")
	}
	if args[0] == "prepare" {
		if research == "" || output == "" || manifest != "" || digest != "" || database != "" || tenant != "" {
			return errors.New("prepare requires research and output only")
		}
		b, e := catalogue.Read(research)
		if e != nil {
			return e
		}
		m, e := catalogue.Prepare(b, limit)
		if e != nil {
			return e
		}
		data, e := json.MarshalIndent(m, "", "  ")
		if e != nil {
			return e
		}
		data = append(data, '\n')
		f, e := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return errors.New("output must be a new operator-owned file")
		}
		_, e = f.Write(data)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		return json.NewEncoder(out).Encode(map[string]any{"status": "prepared_disabled", "sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "candidates": len(m.Entries)})
	}
	if args[0] != "validate" && args[0] != "provision" && args[0] != "inventory" {
		return errors.New("unknown catalogue command")
	}
	if research != "" || output != "" {
		return errors.New("unexpected research/output flags")
	}
	var m catalogue.Manifest
	if args[0] != "inventory" {
		if manifest == "" || digest == "" {
			return errors.New("manifest and exact digest required")
		}
		b, e := catalogue.Read(manifest)
		if e != nil {
			return e
		}
		m, e = catalogue.Parse(b, digest)
		if e != nil {
			return e
		}
	} else if manifest != "" || digest != "" {
		return errors.New("inventory takes no manifest")
	}
	if args[0] == "validate" {
		if database != "" || tenant != "" {
			return errors.New("validate does not open database")
		}
		n := 0
		for _, e := range m.Entries {
			if e.Selected {
				n++
			}
		}
		return json.NewEncoder(out).Encode(map[string]any{"status": "valid", "enabled": m.Enabled, "selected": n, "candidates": len(m.Entries)})
	}
	principal, e := identity.Operator(identity.TenantID(tenant))
	if e != nil || database == "" {
		return errors.New("database and canonical tenant required")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	store, e := sqlite.Open(ctx, database)
	if e != nil {
		return errors.New("cannot open catalogue storage; stop serve and migrate first")
	}
	defer store.Close()
	if args[0] == "inventory" {
		v, e := store.CatalogueInventory(ctx, principal)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	}
	if e = store.ProvisionCuratedCatalogue(ctx, principal, m); e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(map[string]any{"status": "provisioned", "enabled": m.Enabled, "profile": m.Profile, "sha256": digest})
}
