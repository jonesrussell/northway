package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const agentGrantHelp = `Usage:
 northway agent-grant create --database PATH --tenant UUID --scopes collection:observations:read --label RECIPIENT --ttl 1h --output PATH [--dry-run]
 northway agent-grant revoke --database PATH --tenant UUID --grant-id ID [--dry-run]
Stop serve first. New credentials use an existing 0700 directory and a new 0600 file.
Dry-run validates ownership/admission but never generates, writes, inserts or revokes.
Maximum TTL 24h. No bearer secret is accepted as an argument or printed.
`

type agentGrantResult struct {
	Status    string     `json:"status"`
	Operation string     `json:"operation"`
	TenantID  string     `json:"tenant_id"`
	GrantID   string     `json:"grant_id,omitempty"`
	Scopes    string     `json:"scopes,omitempty"`
	Label     string     `json:"label,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	DryRun    bool       `json:"dry_run"`
}

func executeAgentGrant(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "help") {
		_, e := io.WriteString(out, agentGrantHelp)
		return e
	}
	if len(args) == 0 || (args[0] != "create" && args[0] != "revoke") {
		return errors.New("expected agent-grant create|revoke")
	}
	fs := flag.NewFlagSet("agent-grant", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var path, tenant, scopeNames, label, output, id, ttl string
	var dry bool
	fs.StringVar(&path, "database", "", "private database")
	fs.StringVar(&tenant, "tenant", "", "operator tenant")
	fs.BoolVar(&dry, "dry-run", false, "validate without mutation")
	if args[0] == "create" {
		fs.StringVar(&scopeNames, "scopes", "", "explicit collection capabilities")
		fs.StringVar(&label, "label", "", "named recipient/purpose")
		fs.StringVar(&output, "output", "", "new private credential file")
		fs.StringVar(&ttl, "ttl", "", "explicit bounded duration")
	} else {
		fs.StringVar(&id, "grant-id", "", "nonsecret grant ID")
	}
	if e := fs.Parse(args[1:]); e != nil || fs.NArg() != 0 || path == "" {
		return errors.New("invalid agent grant flags")
	}
	p, e := identity.Operator(identity.TenantID(tenant))
	if e != nil {
		return errors.New("canonical tenant UUID required")
	}
	var scopes identity.CollectionScopes
	var duration time.Duration
	if args[0] == "create" {
		scopes, e = identity.ParseCollectionScopes(scopeNames)
		if e != nil {
			return errors.New("explicit collection scopes required")
		}
		duration, e = time.ParseDuration(ttl)
		if e != nil || duration < time.Minute || duration > 24*time.Hour || !identity.ValidAgentLabel(label) {
			return errors.New("named recipient label and TTL 1m..24h required")
		}
		if e = validateGrantOutput(output); e != nil {
			return e
		}
	} else if !identity.ValidKeyID(id) {
		return errors.New("valid nonsecret grant ID required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	store, e := sqlite.Open(ctx, path)
	if e != nil {
		return errors.New("cannot open grant storage; migrate and stop serve first")
	}
	defer store.Close()
	if e = store.RequireTenant(ctx, p); e != nil {
		return errors.New("tenant is not provisioned")
	}
	result := agentGrantResult{Status: "complete", Operation: agentGrantOperation(args[0]).ID, TenantID: tenant, DryRun: dry}
	if args[0] == "revoke" {
		grant, e := store.LookupAgentGrant(ctx, id)
		if e != nil || grant.TenantID != p.TenantID() {
			return errors.New("grant unavailable in tenant scope")
		}
		result.GrantID = id
		if !dry {
			if e = store.RevokeAgentGrant(ctx, p, id); e != nil {
				return errors.New("grant revocation failed")
			}
		}
	} else {
		if e = store.AgentGrantIssueReady(ctx, p); e != nil {
			return errors.New("grant admission unavailable in tenant scope")
		}
		result.Scopes = scopes.Names()
		result.Label = label
		if !dry {
			now := time.Now().UTC()
			grant, secret, e := identity.GenerateAgentGrant(p, scopes, label, now, now.Add(duration))
			if e != nil {
				return errors.New("grant generation failed")
			}
			if e = writeAgentGrant(ctx, store, p, grant, secret, output); e != nil {
				return e
			}
			result.GrantID = grant.ID
			result.ExpiresAt = &grant.ExpiresAt
		}
	}
	if dry {
		result.Status = "validated"
	}
	return json.NewEncoder(out).Encode(result)
}
func validateGrantOutput(path string) error {
	if path == "" {
		return errors.New("new private output file required")
	}
	parent := filepath.Dir(path)
	info, e := os.Lstat(parent)
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return errors.New("grant output requires an existing private 0700 directory")
	}
	if _, e = os.Lstat(path); !errors.Is(e, os.ErrNotExist) {
		return errors.New("grant output must be a new private file")
	}
	return nil
}

type agentGrantCreator interface {
	CreateAgentGrant(context.Context, identity.Principal, identity.AgentGrant) error
}

func writeAgentGrant(ctx context.Context, store agentGrantCreator, p identity.Principal, g identity.AgentGrant, secret identity.Secret, path string) (err error) {
	if err = validateGrantOutput(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return errors.New("grant output must be a new private file")
	}
	keep := false
	defer func() {
		file.Close()
		if !keep {
			if e := os.Remove(path); e != nil && !os.IsNotExist(e) {
				err = errors.Join(err, errors.New("grant output cleanup failed; reconcile before retrying"))
			}
		}
	}()
	if _, err = io.WriteString(file, secret.Reveal()+"\n"); err != nil {
		return errors.New("grant output write failed")
	}
	if err = file.Sync(); err != nil {
		return errors.New("grant output sync failed")
	}
	if err = file.Close(); err != nil {
		return errors.New("grant output close failed")
	}
	directory, e := os.Open(filepath.Dir(path))
	if e != nil {
		return errors.New("grant output directory unavailable")
	}
	if e = errors.Join(directory.Sync(), directory.Close()); e != nil {
		return errors.New("grant output directory sync failed")
	}
	if err = store.CreateAgentGrant(ctx, p, g); err != nil {
		return errors.New("grant provisioning failed")
	}
	keep = true
	return nil
}
