package catalogue

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

func candidate(url, topic, publisher string) Candidate {
	return Candidate{URL: url, DiscoveredURL: url, Title: publisher, Publisher: publisher, Topic: topic, Region: "Global", Language: "en", Rights: "not_assessed", Notes: "Rights not assessed; no original republishing."}
}
func TestPrepareRetainsRestrictionsAndIsInert(t *testing.T) {
	a := []Candidate{candidate("https://a.example/feed", "arts", "A"), candidate("https://b.example/feed", "science", "B"), candidate("https://c.example/feed?q=x", "health", "C")}
	a[1].Rights = "explicit_restrictions_review_required"
	a[1].Notes = "Personal noncommercial only"
	b, _ := json.Marshal(a)
	m, e := Prepare(b, 10)
	if e != nil {
		t.Fatal(e)
	}
	if m.Enabled || m.ApprovalRecord != "" || len(m.Entries) != 3 {
		t.Fatalf("%+v", m)
	}
	n := 0
	for _, v := range m.Entries {
		if v.Selected {
			n++
		}
	}
	if n != 1 || m.Entries[2].Notes != "Personal noncommercial only" {
		t.Fatalf("%+v", m)
	}
}
func TestManifestReviewDigestAndUnsupportedURLs(t *testing.T) {
	b, _ := json.Marshal([]Candidate{candidate("https://a.example/feed", "arts", "A")})
	m, e := Prepare(b, 1)
	if e != nil {
		t.Fatal(e)
	}
	m.Enabled = true
	m.ApprovalRecord = "review-1"
	if m.Validate() == nil {
		t.Fatal("missing source rights basis accepted")
	}
	m.Entries[0].MetadataBasis = "Reviewed linked titles and attribution only; full text prohibited."
	data, _ := json.Marshal(m)
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if _, e := Parse(data, digest); e != nil {
		t.Fatal(e)
	}
	if _, e := Parse(append(data, ' '), digest); e == nil {
		t.Fatal("digest mismatch accepted")
	}
	for _, u := range []string{"http://a.example/feed", "https://a.example/feed?q=x", "https://a.example/feed#x", "https://A.example/feed", "https://a.example:443/feed", "https://u:p@a.example/feed"} {
		m.Entries[0].URL = u
		if m.Validate() == nil {
			t.Fatalf("accepted %s", u)
		}
	}
}
func TestDuplicateAndUnknownFieldsFail(t *testing.T) {
	c := candidate("https://a.example/feed", "arts", "A")
	b, _ := json.Marshal([]Candidate{c, c})
	if _, e := Prepare(b, 1); e == nil {
		t.Fatal("duplicates accepted")
	}
	if _, e := Prepare([]byte("[{\\\"unexpected\\\":true}]"), 1); e == nil {
		t.Fatal("unknown fields accepted")
	}
}
