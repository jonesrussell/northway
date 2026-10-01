package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// AssertionReplayStore must atomically consume an issuer/JTI and persist it
// across restarts. Expiry includes verification skew. Never log assertions.
type AssertionReplayStore interface {
	ConsumeAssertion(context.Context, string, string, TenantID, time.Time, time.Time) error
}

type AssertionVerifier struct {
	issuer, audience string
	keys             map[string]ed25519.PublicKey
	replays          AssertionReplayStore
}

func NewAssertionVerifier(issuer, audience string, keys map[string]ed25519.PublicKey, replays AssertionReplayStore) (*AssertionVerifier, error) {
	if issuer == "" || audience == "" || len(issuer) > 256 || len(audience) > 256 || replays == nil || len(keys) == 0 || len(keys) > 4 {
		return nil, errors.New("invalid assertion configuration")
	}
	copyKeys := make(map[string]ed25519.PublicKey, len(keys))
	for id, key := range keys {
		if id == "" || len(id) > 64 || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("invalid assertion verification key")
		}
		copyKeys[id] = append(ed25519.PublicKey(nil), key...)
	}
	return &AssertionVerifier{issuer, audience, copyKeys, replays}, nil
}

type assertionHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KeyID     string `json:"kid"`
}
type assertionClaims struct {
	Issuer    string   `json:"iss"`
	Audience  string   `json:"aud"`
	Subject   string   `json:"sub"`
	Tenant    TenantID `json:"tenant"`
	Issued    int64    `json:"iat"`
	NotBefore int64    `json:"nbf"`
	Expires   int64    `json:"exp"`
	ID        string   `json:"jti"`
	Operation string   `json:"op"`
	Version   int      `json:"ver"`
}

func (v *AssertionVerifier) Verify(ctx context.Context, raw, operation string, now time.Time) (Principal, error) {
	if v == nil || v.replays == nil {
		return Principal{}, ErrUnavailable
	}
	if len(raw) > 4096 || operation == "" {
		return Principal{}, ErrUnauthorized
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Principal{}, ErrUnauthorized
	}
	hb, e1 := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	cb, e2 := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	sig, e3 := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	var h assertionHeader
	if e1 != nil || e2 != nil || e3 != nil || strictFlatJSON(hb, &h, "alg", "typ", "kid") != nil || h.Algorithm != "EdDSA" || h.Type != "ncl-fpa+jwt" {
		return Principal{}, ErrUnauthorized
	}
	key, ok := v.keys[h.KeyID]
	if !ok || len(sig) != ed25519.SignatureSize || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return Principal{}, ErrUnauthorized
	}
	var c assertionClaims
	if strictFlatJSON(cb, &c, "iss", "aud", "sub", "tenant", "iat", "nbf", "exp", "jti", "op", "ver") != nil || c.Issuer != v.issuer || c.Audience != v.audience || c.Version != 1 || c.Operation != operation || c.Tenant.Validate() != nil || c.Subject != string(c.Tenant) || ValidateID(c.ID) != nil {
		return Principal{}, ErrUnauthorized
	}
	// Bound timestamps before subtraction/addition to prevent overflow.
	if c.Issued < 0 || c.Issued > 253402300700 || c.NotBefore != c.Issued || c.Expires <= c.Issued || c.Expires-c.Issued > 60 || c.Issued > now.Unix()+5 || now.Unix() > c.Expires+5 {
		return Principal{}, ErrUnauthorized
	}
	if err := v.replays.ConsumeAssertion(ctx, c.Issuer, c.ID, c.Tenant, time.Unix(c.Expires+5, 0), now); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, ErrUnavailable
	}
	return Principal{tenant: c.Tenant, scopes: FeedsRead | FeedbackWrite, management: true}, nil
}

// Fixed flat protocol objects reject duplicates, unknown/case-folded keys,
// missing/null values and nested values before Go's permissive struct decoder.
func strictFlatJSON(data []byte, target any, required ...string) error {
	d := json.NewDecoder(bytes.NewReader(data))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return ErrUnauthorized
	}
	allowed := map[string]bool{}
	for _, k := range required {
		allowed[k] = false
	}
	for d.More() {
		t, err = d.Token()
		if err != nil {
			return err
		}
		k, ok := t.(string)
		seen, known := allowed[k]
		if !ok || !known || seen {
			return ErrUnauthorized
		}
		allowed[k] = true
		t, err = d.Token()
		if err != nil || t == nil {
			return ErrUnauthorized
		}
		if _, nested := t.(json.Delim); nested {
			return ErrUnauthorized
		}
	}
	if t, err = d.Token(); err != nil || t != json.Delim('}') {
		return ErrUnauthorized
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		return ErrUnauthorized
	}
	for _, seen := range allowed {
		if !seen {
			return ErrUnauthorized
		}
	}
	return json.Unmarshal(data, target)
}
