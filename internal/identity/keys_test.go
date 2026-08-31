package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestKeyFormatAndRedaction(t *testing.T) {
	p, err := Operator("00000001-0000-4000-8000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for i := 0; i < 32; i++ {
		key, secret, err := GenerateKey(p, FeedsRead)
		if err != nil {
			t.Fatal(err)
		}
		id, ok := parseKey(secret.Reveal())
		if !ok || id != key.ID || seen[id] {
			t.Fatal("malformed or duplicate generated key")
		}
		seen[id] = true
		encoded, err := json.Marshal(secret)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{string(encoded), fmt.Sprint(secret), fmt.Sprintf("%+v %#v", secret, secret)} {
			if strings.Contains(text, secret.Reveal()) || !strings.Contains(text, "REDACTED") {
				t.Fatal("secret formatting is unsafe")
			}
		}
	}
	for _, value := range []string{"", "feeds:read,feeds:read", "admin", "feeds:read,", " feeds:read"} {
		if _, err := ParseScopes(value); err == nil {
			t.Fatal("accepted invalid scopes")
		}
	}
	if scopes, err := ParseScopes("feeds:read,feedback:write"); err != nil || scopes != FeedsRead|FeedbackWrite {
		t.Fatal("valid scopes rejected")
	}
	if _, _, err := GenerateKey(Principal{}, FeedsRead); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("zero principal provisioned key")
	}
	var decoded Principal
	if err := json.Unmarshal([]byte(`{"tenant":"00000001-0000-4000-8000-000000000000","operator":true,"scopes":3}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := decoded.Require(FeedsRead); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("JSON forged identity")
	}
}

type revokingStore struct{ key KeyRecord }

func (s revokingStore) LookupAPIKey(context.Context, string) (KeyRecord, error) { return s.key, nil }
func (revokingStore) TouchAPIKey(context.Context, TenantID, string, time.Time) (bool, error) {
	return false, nil
}

func TestRevocationBetweenLookupAndUse(t *testing.T) {
	p, _ := Operator("00000001-0000-4000-8000-000000000000")
	key, secret, err := GenerateKey(p, FeedsRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(revokingStore{key}).Authenticate(t.Context(), secret.Reveal()); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revocation race authenticated")
	}
}
