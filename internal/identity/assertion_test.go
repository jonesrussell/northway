package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

type replayStub struct{ calls int }

func (s *replayStub) ConsumeAssertion(context.Context, string, string, TenantID, time.Time, time.Time) error {
	s.calls++
	return nil
}
func TestAssertionStrictWireShape(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := &replayStub{}
	v, err := NewAssertionVerifier("northcloud", "northcloud-api", map[string]ed25519.PublicKey{"test": pub}, store)
	if err != nil {
		t.Fatal(err)
	}
	h := `{"alg":"EdDSA","typ":"ncl-fpa+jwt","kid":"test"}`
	c := `{"iss":"northcloud","aud":"northcloud-api","sub":"00000000-0000-4000-8000-000000000001","tenant":"00000000-0000-4000-8000-000000000001","iat":100,"nbf":100,"exp":130,"jti":"00000000-0000-4000-8000-000000000002","op":"PUT /v1/workspace","ver":1}`
	sign := func(header, claims string) string {
		m := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(claims))
		return m + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(m)))
	}
	for _, bad := range []string{strings.Replace(c, `"ver":1`, `"ver":1,"ver":1`, 1), strings.Replace(c, `"ver":1`, `"Ver":1`, 1), strings.Replace(c, `"ver":1`, `"ver":null`, 1), strings.Replace(c, `"ver":1`, `"ver":{}`, 1), c + ` {}`, strings.Replace(c, `"iat":100`, `"iat":1e2`, 1)} {
		if _, err := v.Verify(t.Context(), sign(h, bad), "PUT /v1/workspace", time.Unix(110, 0)); err == nil {
			t.Fatal("invalid claims accepted")
		}
	}
	for _, bad := range []string{strings.Replace(h, "ncl-fpa+jwt", "gofx-fpa+jwt", 1), strings.Replace(h, "EdDSA", "HS256", 1), strings.Replace(h, "test", "unknown", 1), strings.Replace(h, `"kid":"test"`, `"kid":"test","kid":"test"`, 1)} {
		if _, err := v.Verify(t.Context(), sign(bad, c), "PUT /v1/workspace", time.Unix(110, 0)); err == nil {
			t.Fatal("invalid header accepted")
		}
	}
	if store.calls != 0 {
		t.Fatal("invalid assertions consumed replay")
	}
	p, err := v.Verify(t.Context(), sign(h, c), "PUT /v1/workspace", time.Unix(110, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.RequireOperator(); err == nil {
		t.Fatal("assertion gained local operator")
	}
	if _, err = p.RequireManagement(); err != nil {
		t.Fatal(err)
	}
}
