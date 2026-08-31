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

var (
	ErrNotFound     = errors.New("object not found in tenant scope")
	ErrUnauthorized = errors.New("invalid credentials")
	ErrForbidden    = errors.New("insufficient scope")
	ErrUnavailable  = errors.New("identity storage unavailable")
)

type Scopes uint8

const (
	FeedsRead Scopes = 1 << iota
	FeedbackWrite
)

func (s Scopes) Valid() bool { return s != 0 && s & ^(FeedsRead|FeedbackWrite) == 0 }

func ParseScopes(value string) (Scopes, error) {
	var scopes Scopes
	for _, name := range strings.Split(value, ",") {
		var scope Scopes
		switch name {
		case "feeds:read":
			scope = FeedsRead
		case "feedback:write":
			scope = FeedbackWrite
		default:
			return 0, errors.New("unknown or empty scope")
		}
		if scopes&scope != 0 {
			return 0, errors.New("duplicate scope")
		}
		scopes |= scope
	}
	return scopes, nil
}

// Principal is a request-local capability. Fields cannot be populated by a
// transport decoder. Its zero value grants no authority; do not cache it.
type Principal struct {
	tenant   TenantID
	keyID    string
	scopes   Scopes
	operator bool
}

func (p Principal) TenantID() TenantID { return p.tenant }
func (p Principal) KeyID() string      { return p.keyID }

func (p Principal) Require(scope Scopes) (TenantID, error) {
	if p.tenant.Validate() != nil {
		return "", ErrUnauthorized
	}
	if !scope.Valid() || (!p.operator && p.scopes&scope != scope) {
		return "", ErrForbidden
	}
	return p.tenant, nil
}

func (p Principal) RequireOperator() (TenantID, error) {
	if p.tenant.Validate() != nil {
		return "", ErrUnauthorized
	}
	if !p.operator {
		return "", ErrForbidden
	}
	return p.tenant, nil
}

// Operator is exclusively for trusted local provisioning or explicitly scoped
// internal jobs. Never construct it from an HTTP request or model output.
// Database-file operators already have authority over all tenants.
func Operator(tenant TenantID) (Principal, error) {
	if err := tenant.Validate(); err != nil {
		return Principal{}, err
	}
	return Principal{tenant: tenant, operator: true}, nil
}

// Secret must be revealed explicitly for the one-time private-file handoff.
// Default formatting and JSON/text marshaling never emit credential material.
type Secret struct{ raw string }

func (Secret) String() string               { return "[REDACTED]" }
func (Secret) GoString() string             { return "[REDACTED]" }
func (Secret) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }
func (s Secret) Reveal() string             { return s.raw }

// KeyRecord contains no recoverable credential. ID is a nonsecret lookup handle.
type KeyRecord struct {
	ID                    string
	TenantID              TenantID
	Digest                [32]byte
	Scopes                Scopes
	CreatedAt             time.Time
	LastUsedAt, RevokedAt *time.Time
}

func ValidKeyID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// GenerateKey uses independent 128-bit public lookup and 256-bit secret values.
// SHA-256 is appropriate for generated entropy, not human passwords.
func GenerateKey(p Principal, scopes Scopes) (KeyRecord, Secret, error) {
	tenant, err := p.RequireOperator()
	if err != nil {
		return KeyRecord{}, Secret{}, err
	}
	if !scopes.Valid() {
		return KeyRecord{}, Secret{}, ErrForbidden
	}
	var id [16]byte
	var secret [32]byte
	rand.Read(id[:])
	rand.Read(secret[:])
	keyID := hex.EncodeToString(id[:])
	raw := "nw1_" + keyID + "_" + base64.RawURLEncoding.EncodeToString(secret[:])
	return KeyRecord{ID: keyID, TenantID: tenant, Digest: sha256.Sum256([]byte(raw)), Scopes: scopes, CreatedAt: time.Now().UTC()}, Secret{raw}, nil
}

func parseKey(raw string) (string, bool) {
	if len(raw) != 80 || !strings.HasPrefix(raw, "nw1_") || raw[36] != '_' {
		return "", false
	}
	id := raw[4:36]
	secret, err := base64.RawURLEncoding.Strict().DecodeString(raw[37:])
	return id, ValidKeyID(id) && err == nil && len(secret) == 32
}

// KeyStore is the narrow exception to tenant-first lookup: only a validated
// nonsecret key handle may discover identity. All corpus I/O uses a principal.
type KeyStore interface {
	LookupAPIKey(context.Context, string) (KeyRecord, error)
	TouchAPIKey(context.Context, TenantID, string, time.Time) (bool, error)
}

type Service struct{ store KeyStore }

func NewService(store KeyStore) *Service { return &Service{store: store} }

// Authenticate checks persistent revocation on every call; there is no auth
// cache. An in-flight request may finish after revocation. A later call cannot.
func (s *Service) Authenticate(ctx context.Context, raw string) (Principal, error) {
	id, valid := parseKey(raw)
	if !valid {
		return Principal{}, ErrUnauthorized
	}
	if s == nil || s.store == nil {
		return Principal{}, ErrUnavailable
	}
	record, err := s.store.LookupAPIKey(ctx, id)
	if err != nil && !errors.Is(err, ErrUnauthorized) {
		return Principal{}, ErrUnavailable
	}
	digest := sha256.Sum256([]byte(raw))
	match := subtle.ConstantTimeCompare(digest[:], record.Digest[:])
	if err != nil || match != 1 || record.ID != id || record.RevokedAt != nil || !record.Scopes.Valid() || record.TenantID.Validate() != nil {
		return Principal{}, ErrUnauthorized
	}
	active, err := s.store.TouchAPIKey(ctx, record.TenantID, id, time.Now().UTC())
	if err != nil {
		return Principal{}, ErrUnavailable
	}
	if !active {
		return Principal{}, ErrUnauthorized
	}
	return Principal{tenant: record.TenantID, keyID: id, scopes: record.Scopes}, nil
}
