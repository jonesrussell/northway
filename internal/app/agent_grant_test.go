package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentGrantCLIDryRunLifecycleAndTenantCustody(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	database := filepath.Join(dir, "northway-collection.fixture.sqlite")
	var out bytes.Buffer
	run := func(args ...string) error { out.Reset(); return Execute(t.Context(), args, os.LookupEnv, &out, &out) }
	if e := run("collection", "fixture", "--database", database, "--input", "../../testdata/creator.html"); e != nil {
		t.Fatal(e)
	}
	output := filepath.Join(dir, "fetder.key")
	create := []string{"agent-grant", "create", "--database", database, "--tenant", string(fixtureTenant), "--scopes", "collection:observations:read", "--label", "disposable-fetder-delivery", "--ttl", "1h", "--output", output}
	if e := run(append(create, "--dry-run")...); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(output); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("dry-run created secret")
	}
	var dry agentGrantResult
	if e := json.Unmarshal(out.Bytes(), &dry); e != nil || !dry.DryRun || dry.GrantID != "" || dry.Status != "validated" {
		t.Fatal(dry, e)
	}
	s, e := sqlite.Open(t.Context(), database)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.LookupAgentGrant(t.Context(), strings.Repeat("0", 32)); !errors.Is(e, identity.ErrUnauthorized) {
		t.Fatal(e)
	}
	s.Close()
	if e = run(create...); e != nil {
		t.Fatal(e)
	}
	var result agentGrantResult
	if e = json.Unmarshal(out.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(output)
	if e != nil {
		t.Fatal(e)
	}
	token := strings.TrimSpace(string(raw))
	if strings.Contains(out.String(), token) || !identity.ValidKeyID(result.GrantID) || result.ExpiresAt == nil {
		t.Fatal("unsafe typed result")
	}
	info, e := os.Lstat(output)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, e)
	}
	s, e = sqlite.Open(t.Context(), database)
	if e != nil {
		t.Fatal(e)
	}
	p, e := identity.NewAgentService(s).Authenticate(t.Context(), token)
	if e != nil {
		t.Fatal(e)
	}
	if p.TenantID() != fixtureTenant {
		t.Fatal("tenant mismatch")
	}
	s.Close()
	replacementArgs := append([]string{}, create...)
	replacementPath := filepath.Join(dir, "replacement.key")
	replacementArgs[len(replacementArgs)-1] = replacementPath
	if e = run(replacementArgs...); e != nil {
		t.Fatal(e)
	}
	replacementBytes, e := os.ReadFile(replacementPath)
	if e != nil {
		t.Fatal(e)
	}
	replacementToken := strings.TrimSpace(string(replacementBytes))
	if e = run(create...); e == nil {
		t.Fatal("credential overwritten")
	}
	if e = run("tenant", "create", "--database", database, "--tenant", "00000002-0000-4000-8000-000000000000"); e != nil {
		t.Fatal(e)
	}
	revoke := []string{"agent-grant", "revoke", "--database", database, "--tenant", string(fixtureTenant), "--grant-id", result.GrantID}
	wrong := append([]string{}, revoke...)
	wrong[5] = "00000002-0000-4000-8000-000000000000"
	if e = run(wrong...); e == nil {
		t.Fatal("cross tenant revoke")
	}
	if e = run(append(revoke, "--dry-run")...); e != nil {
		t.Fatal(e)
	}
	s, e = sqlite.Open(t.Context(), database)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = identity.NewAgentService(s).Authenticate(t.Context(), token); e != nil {
		t.Fatal("dry-run revoked", e)
	}
	s.Close()
	for i := 0; i < 2; i++ {
		if e = run(revoke...); e != nil {
			t.Fatal(e)
		}
	}
	s, e = sqlite.Open(t.Context(), database)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = identity.NewAgentService(s).Authenticate(t.Context(), replacementToken); e != nil {
		t.Fatal("rotation revoked replacement", e)
	}
	if _, e = identity.NewAgentService(s).Authenticate(t.Context(), token); !errors.Is(e, identity.ErrUnauthorized) {
		t.Fatal("revoked credential accepted", e)
	}
}

type failingAgentCreator struct{}

func (failingAgentCreator) CreateAgentGrant(context.Context, identity.Principal, identity.AgentGrant) error {
	return errors.New("private persistence failure")
}
func TestAgentGrantPrivateOutputFailureAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	p, _ := identity.Operator(fixtureTenant)
	now := time.Now().UTC()
	g, secret, e := identity.GenerateAgentGrant(p, identity.CollectionObservationsRead, "fixture", now, now.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	output := filepath.Join(dir, "grant.key")
	if e = writeAgentGrant(t.Context(), failingAgentCreator{}, p, g, secret, output); e == nil {
		t.Fatal("failure accepted")
	}
	if _, e = os.Stat(output); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("failed issuance retained secret")
	}
	target := filepath.Join(dir, "existing")
	os.WriteFile(target, []byte("retained"), 0600)
	os.Symlink(target, output)
	if e = validateGrantOutput(output); e == nil {
		t.Fatal("symlink accepted")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "retained" {
		t.Fatal("existing file changed")
	}
	if _, e = identity.ParseCollectionScopes("feeds:read"); !errors.Is(e, identity.ErrForbidden) {
		t.Fatal(e)
	}
	if _, e = identity.ParseCollectionScopes("collection:seed,collection:seed"); !errors.Is(e, identity.ErrForbidden) {
		t.Fatal(e)
	}
}
