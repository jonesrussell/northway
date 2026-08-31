package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/source"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

func newKey(t *testing.T, s *Store, tenant identity.TenantID, scopes identity.Scopes) (identity.KeyRecord, identity.Secret) {
	t.Helper()
	key, secret, err := identity.GenerateKey(operator(tenant), scopes)
	must(t, err)
	must(t, s.CreateAPIKey(t.Context(), operator(tenant), key))
	return key, secret
}

func TestKeyLifecycleAndIsolation(t *testing.T) {
	s, path := fresh(t)
	seed(t, s, tenantA)
	seed(t, s, tenantB)
	key, secret := newKey(t, s, tenantA, identity.FeedsRead)
	bKey, bSecret := newKey(t, s, tenantB, identity.FeedsRead)
	auth := identity.NewService(s)
	if key.Digest != sha256.Sum256([]byte(secret.Reveal())) {
		t.Fatal("digest mismatch")
	}
	for _, raw := range []string{"", "invalid", secret.Reveal() + "x", strings.Replace(secret.Reveal(), "nw1_", "nw2_", 1), secret.Reveal()[:37] + bSecret.Reveal()[37:]} {
		if _, err := auth.Authenticate(t.Context(), raw); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatal("invalid credential accepted or leaked an error")
		}
	}
	before, err := s.LookupAPIKey(t.Context(), key.ID)
	must(t, err)
	if before.LastUsedAt != nil {
		t.Fatal("invalid requests updated usage")
	}
	principal, err := auth.Authenticate(t.Context(), secret.Reveal())
	must(t, err)
	if principal.TenantID() != tenantA || principal.KeyID() != key.ID {
		t.Fatal("wrong authenticated identity")
	}
	after, err := s.LookupAPIKey(t.Context(), key.ID)
	must(t, err)
	if after.LastUsedAt == nil {
		t.Fatal("successful authentication did not record use")
	}
	if err := s.RevokeAPIKey(t.Context(), operator(tenantB), key.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-tenant key revocation")
	}
	if err := s.RevokeAPIKey(t.Context(), principal, key.ID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("service key gained operator authority")
	}
	active, err := s.TouchAPIKey(t.Context(), tenantB, key.ID, time.Now())
	must(t, err)
	if active {
		t.Fatal("cross-tenant key metadata write")
	}
	_, replacement := newKey(t, s, tenantA, identity.FeedsRead|identity.FeedbackWrite)
	must(t, s.RevokeAPIKey(t.Context(), operator(tenantA), key.ID))
	must(t, s.RevokeAPIKey(t.Context(), operator(tenantA), key.ID))
	if _, err := auth.Authenticate(t.Context(), secret.Reveal()); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatal("revoked key authenticated")
	}
	active, err = s.TouchAPIKey(t.Context(), tenantA, key.ID, time.Now())
	must(t, err)
	if active {
		t.Fatal("revocation race guard failed")
	}
	if _, err := auth.Authenticate(t.Context(), replacement.Reveal()); err != nil {
		t.Fatal("rotation invalidated replacement")
	}
	if _, err := auth.Authenticate(t.Context(), bSecret.Reveal()); err != nil {
		t.Fatal("revocation crossed tenants")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		must(t, err)
		for _, value := range []identity.Secret{secret, bSecret, replacement} {
			if bytes.Contains(data, []byte(value.Reveal())) || bytes.Contains(data, []byte(value.Reveal()[37:])) {
				t.Fatal("credential material persisted in SQLite")
			}
		}
	}
	must(t, s.Close())
	reopened, err := Open(t.Context(), path)
	must(t, err)
	defer reopened.Close()
	auth = identity.NewService(reopened)
	if _, err := auth.Authenticate(t.Context(), secret.Reveal()); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatal("restart lost revocation")
	}
	if _, err := auth.Authenticate(t.Context(), replacement.Reveal()); err != nil {
		t.Fatal("restart lost replacement")
	}
	stored, err := reopened.LookupAPIKey(t.Context(), bKey.ID)
	must(t, err)
	if stored.TenantID != tenantB {
		t.Fatal("wrong key owner after restart")
	}
}

func TestPrincipalGuardsEveryCorpusPath(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	seed(t, s, tenantB)
	must(t, s.PutArticle(t.Context(), operator(tenantA), item()))
	_, readSecret := newKey(t, s, tenantA, identity.FeedsRead)
	_, writeSecret := newKey(t, s, tenantA, identity.FeedbackWrite)
	_, otherSecret := newKey(t, s, tenantB, identity.FeedsRead|identity.FeedbackWrite)
	auth := identity.NewService(s)
	read, err := auth.Authenticate(t.Context(), readSecret.Reveal())
	must(t, err)
	feedback, err := auth.Authenticate(t.Context(), writeSecret.Reveal())
	must(t, err)
	other, err := auth.Authenticate(t.Context(), otherSecret.Reveal())
	must(t, err)
	for _, p := range []identity.Principal{{}, feedback} {
		if _, err := s.GetArticle(t.Context(), p, itemID); err == nil {
			t.Fatal("unscoped article read")
		}
		if _, err := s.GetFeed(t.Context(), p, feedID); err == nil {
			t.Fatal("unscoped feed read")
		}
		if _, err := s.Search(t.Context(), p, feedID, "PHP", time.Unix(0, 0), 1); err == nil {
			t.Fatal("unscoped FTS read")
		}
	}
	for _, p := range []identity.Principal{{}, read, feedback, other} {
		_, want := p.RequireOperator()
		if err := s.CreateSource(t.Context(), p, source.Source{ID: sourceID, URL: "https://example.invalid/other", Title: "Denied"}); !errors.Is(err, want) {
			t.Fatal("unprivileged source creation")
		}
		if err := s.CreateFeed(t.Context(), p, feed.Feed{ID: feedID, Title: "Denied"}); !errors.Is(err, want) {
			t.Fatal("unprivileged feed creation")
		}
		if err := s.AttachSource(t.Context(), p, feedID, sourceID); !errors.Is(err, want) {
			t.Fatal("unprivileged source attachment")
		}
		if err := s.PutArticle(t.Context(), p, item()); !errors.Is(err, want) {
			t.Fatal("unprivileged corpus update")
		}
		if err := s.DeleteArticle(t.Context(), p, itemID); !errors.Is(err, want) {
			t.Fatal("unprivileged corpus deletion")
		}
		key, _, err := identity.GenerateKey(operator(tenantA), identity.FeedsRead)
		must(t, err)
		if err := s.CreateAPIKey(t.Context(), p, key); !errors.Is(err, want) {
			t.Fatal("unprivileged key creation")
		}
	}
	if _, err := s.GetArticle(t.Context(), other, itemID); !errors.Is(err, ErrNotFound) {
		t.Fatal("guessed other tenant article")
	}
	rows, err := s.Search(t.Context(), other, feedID, "PHP", time.Unix(0, 0), 50)
	must(t, err)
	if len(rows) != 0 {
		t.Fatal("FTS crossed tenant")
	}
	if _, err := s.GetArticle(t.Context(), read, itemID); err != nil {
		t.Fatal("authorized article unavailable")
	}
	// Sequential batches and asynchronous work carry the same explicit principal.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for _, id := range []string{itemID, "00000009-0000-4000-8000-000000000000"} {
				if _, err := s.GetArticle(t.Context(), other, id); !errors.Is(err, ErrNotFound) {
					t.Error("background/batch tenant bypass")
				}
			}
		})
	}
	wg.Wait()
}

func TestAuthenticationFailsClosedOnCancellationAndStorageFailure(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	_, secret := newKey(t, s, tenantA, identity.FeedsRead)
	auth := identity.NewService(s)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := auth.Authenticate(ctx, secret.Reveal()); !errors.Is(err, identity.ErrUnavailable) {
		t.Fatal("canceled auth did not fail closed")
	}
	must(t, s.Close())
	if _, err := auth.Authenticate(t.Context(), secret.Reveal()); !errors.Is(err, identity.ErrUnavailable) {
		t.Fatal("storage failure did not fail closed")
	}
}

func TestUpgradeSchemaTwoPreservesCorpus(t *testing.T) {
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
	_, err = p.UpTo(t.Context(), 2)
	must(t, err)
	q := sqlc.New(db)
	must(t, q.CreateTenant(t.Context(), sqlc.CreateTenantParams{ID: string(tenantA), CreatedAt: 1}))
	must(t, q.CreateSource(t.Context(), sqlc.CreateSourceParams{TenantID: string(tenantA), ID: sourceID, Url: "https://example.invalid/feed", Title: "Source"}))
	_, err = q.PutArticle(t.Context(), sqlc.PutArticleParams{TenantID: string(tenantA), ID: itemID, SourceID: sourceID, OriginID: "before", Url: "https://example.invalid/item", Title: "PHP upgrade", ContentHash: strings.Repeat("a", 64), ObservedAt: 1})
	must(t, err)
	must(t, db.Close())
	must(t, file.Close())
	if old, err := Open(t.Context(), path); err == nil {
		old.Close()
		t.Fatal("schema 2 served before migration")
	}
	must(t, Migrate(t.Context(), path))
	must(t, Migrate(t.Context(), path))
	s, err := Open(t.Context(), path)
	must(t, err)
	defer s.Close()
	got, err := s.GetArticle(t.Context(), operator(tenantA), itemID)
	must(t, err)
	if got.Title != "PHP upgrade" {
		t.Fatal("upgrade changed corpus")
	}
	_, secret := newKey(t, s, tenantA, identity.FeedsRead)
	if _, err := identity.NewService(s).Authenticate(t.Context(), secret.Reveal()); err != nil {
		t.Fatal("upgraded key storage unavailable")
	}
}
