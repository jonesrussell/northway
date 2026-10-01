package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type CollectionScopes uint8

const (
	CollectionSeed CollectionScopes = 1 << iota
	CollectionStatus
	CollectionObservationsRead
)

func (s CollectionScopes) Valid() bool {
	return s != 0 && s & ^(CollectionSeed|CollectionStatus|CollectionObservationsRead) == 0
}
func (p Principal) RequireCollection(s CollectionScopes) (TenantID, error) {
	if p.tenant.Validate() != nil {
		return "", ErrUnauthorized
	}
	if !s.Valid() || (!p.operator && p.collection&s != s) {
		return "", ErrForbidden
	}
	return p.tenant, nil
}

type AgentGrant struct {
	ID                   string
	TenantID             TenantID
	Digest               [32]byte
	Scopes               CollectionScopes
	CreatedAt, ExpiresAt time.Time
	RevokedAt            *time.Time
	Label                string
}

func GenerateAgentGrant(p Principal, scopes CollectionScopes, label string, now, expires time.Time) (AgentGrant, Secret, error) {
	tenant, e := p.RequireOperator()
	if e != nil {
		return AgentGrant{}, Secret{}, e
	}
	if !scopes.Valid() || strings.TrimSpace(label) == "" || len(label) > 128 || !expires.After(now) || expires.Sub(now) > 24*time.Hour {
		return AgentGrant{}, Secret{}, ErrForbidden
	}
	var id [16]byte
	var secret [32]byte
	if _, e = rand.Read(id[:]); e != nil {
		return AgentGrant{}, Secret{}, e
	}
	if _, e = rand.Read(secret[:]); e != nil {
		return AgentGrant{}, Secret{}, e
	}
	keyID := hex.EncodeToString(id[:])
	raw := "nwa1_" + keyID + "_" + base64.RawURLEncoding.EncodeToString(secret[:])
	return AgentGrant{ID: keyID, TenantID: tenant, Digest: sha256.Sum256([]byte(raw)), Scopes: scopes, CreatedAt: now, ExpiresAt: expires, Label: label}, Secret{raw}, nil
}

type AgentGrantStore interface {
	LookupAgentGrant(context.Context, string) (AgentGrant, error)
	TouchAgentGrant(context.Context, TenantID, string, time.Time) (bool, error)
}
type AgentService struct{ store AgentGrantStore }

func NewAgentService(s AgentGrantStore) *AgentService { return &AgentService{s} }
func (s *AgentService) Authenticate(ctx context.Context, raw string) (Principal, error) {
	if len(raw) != 81 || !strings.HasPrefix(raw, "nwa1_") || raw[37] != '_' {
		return Principal{}, ErrUnauthorized
	}
	id := raw[5:37]
	secret, e := base64.RawURLEncoding.Strict().DecodeString(raw[38:])
	if !ValidKeyID(id) || e != nil || len(secret) != 32 {
		return Principal{}, ErrUnauthorized
	}
	if s == nil || s.store == nil {
		return Principal{}, ErrUnavailable
	}
	g, e := s.store.LookupAgentGrant(ctx, id)
	if e != nil && !errors.Is(e, ErrUnauthorized) {
		return Principal{}, ErrUnavailable
	}
	digest := sha256.Sum256([]byte(raw))
	now := time.Now().UTC()
	if subtle.ConstantTimeCompare(digest[:], g.Digest[:]) != 1 || e != nil || g.ID != id || g.TenantID.Validate() != nil || !g.Scopes.Valid() || g.RevokedAt != nil || now.Before(g.CreatedAt) || !now.Before(g.ExpiresAt) {
		return Principal{}, ErrUnauthorized
	}
	active, e := s.store.TouchAgentGrant(ctx, g.TenantID, id, now)
	if e != nil {
		return Principal{}, ErrUnavailable
	}
	if !active {
		return Principal{}, ErrUnauthorized
	}
	return Principal{tenant: g.TenantID, keyID: id, collection: g.Scopes}, nil
}
