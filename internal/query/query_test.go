package query

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalRequestAndBounds(t *testing.T) {
	var a, b Request
	if err := json.Unmarshal([]byte(`{"feed_id":"00000004-0000-4000-8000-000000000000","context":{"intent":"PHP","technologies":[]},"max_age_hours":24,"limit":5}`), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{ "limit":5, "max_age_hours":24,"context":{"technologies":[],"focus":[],"intent":"PHP"},"feed_id":"00000004-0000-4000-8000-000000000000"}`), &b); err != nil {
		t.Fatal(err)
	}
	first, err := a.Digest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Digest()
	if err != nil || first != second {
		t.Fatal("transport whitespace/key order altered digest")
	}
	for _, change := range []func(*Request){
		func(r *Request) { r.Limit = 0 }, func(r *Request) { r.Limit = 21 }, func(r *Request) { r.MaxAgeHours = 721 },
		func(r *Request) { r.Context.Intent = strings.Repeat("x", 2001) }, func(r *Request) { r.Context.Intent = "\xff" },
		func(r *Request) { r.Context.Technologies = nil }, func(r *Request) { r.Context.Focus = []string{""} },
		func(r *Request) {
			r.Context.Technologies = []Technology{{Name: "PHP", Version: strings.Repeat("x", 101)}}
		},
	} {
		bad := a
		change(&bad)
		if _, err := bad.Digest(); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
	b.Limit = 6
	second, _ = b.Digest()
	if first == second {
		t.Fatal("different payload has same digest")
	}
	for _, key := range []string{"", strings.Repeat("x", 15), strings.Repeat("x", 129), "sixteen-key\tvalue", strings.Repeat("é", 16)} {
		if ValidKey(key) {
			t.Fatal("invalid key accepted")
		}
	}
	for _, key := range []string{strings.Repeat("x", 16), strings.Repeat("~", 128)} {
		if !ValidKey(key) {
			t.Fatal("valid key rejected")
		}
	}
}
