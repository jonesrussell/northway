// Package catalogue validates operator-owned metadata registers; it never fetches.
package catalogue

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxFileBytes = 256 << 10

type Candidate struct {
	URL           string `json:"url"`
	DiscoveredURL string `json:"discovered_url"`
	Title         string `json:"title"`
	Publisher     string `json:"publisher"`
	Topic         string `json:"topic"`
	Region        string `json:"region"`
	Language      string `json:"language"`
	Rights        string `json:"publication_rights_status"`
	Notes         string `json:"notes"`
}
type Entry struct {
	Candidate
	Selected      bool   `json:"selected"`
	MetadataBasis string `json:"metadata_basis"`
}
type Manifest struct {
	SchemaVersion   int     `json:"schema_version"`
	Profile         string  `json:"profile"`
	ResearchSHA256  string  `json:"research_sha256"`
	Enabled         bool    `json:"enabled"`
	ApprovalRecord  string  `json:"approval_record"`
	IntervalSeconds int     `json:"interval_seconds"`
	MaxBytes        int64   `json:"max_bytes"`
	Entries         []Entry `json:"entries"`
}

func bounded(s string, n int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= n && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\r\n")
}
func Read(path string) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
		return nil, errors.New("catalogue requires bounded regular file")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return nil, errors.New("catalogue changed while opening")
	}
	b, e := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if e != nil || len(b) > MaxFileBytes {
		return nil, errors.New("catalogue too large")
	}
	return b, nil
}
func decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("invalid catalogue JSON")
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return errors.New("catalogue requires one JSON value")
	}
	return nil
}
func valid(c Candidate) bool {
	return bounded(c.URL, 2048) && bounded(c.Title, 256) && bounded(c.Publisher, 128) && bounded(c.Topic, 64) && bounded(c.Region, 128) && bounded(c.Language, 32) && bounded(c.Rights, 128) && (c.Notes == "" || bounded(c.Notes, 2048))
}
func Category(topic string) string {
	switch topic {
	case "technology":
		return "development"
	case "indigenous":
		return "first_nations"
	case "canada":
		return "canada"
	case "arts", "books", "culture", "film", "food", "lifestyle", "music", "sports", "travel":
		return "entertainment"
	default:
		return "world"
	}
}
func ID(kind, key string) string {
	h := sha256.Sum256([]byte("northcloud-curated-v1:" + kind + ":" + key))
	h[6] = (h[6] & 15) | 0x50
	h[8] = (h[8] & 63) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}
func PublisherGroup(name string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return fmt.Sprintf("publisher-%x", h[:12])
}

// Prepare retains every candidate and restriction. Selection is an inert diverse canary.
func Prepare(b []byte, limit int) (Manifest, error) {
	var rows []Candidate
	if len(b) > MaxFileBytes || decode(b, &rows) != nil || len(rows) == 0 || len(rows) > 500 || limit < 1 || limit > 10 {
		return Manifest{}, errors.New("invalid research register or canary limit")
	}
	m := Manifest{SchemaVersion: 1, Profile: "curated-v1", ResearchSHA256: fmt.Sprintf("%x", sha256.Sum256(b)), IntervalSeconds: 86400, MaxBytes: 2 << 20}
	urls, topics, publishers := map[string]bool{}, map[string]bool{}, map[string]bool{}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Topic != rows[j].Topic {
			return rows[i].Topic < rows[j].Topic
		}
		return rows[i].URL < rows[j].URL
	})
	selected := 0
	for _, c := range rows {
		if !valid(c) || urls[c.URL] {
			return Manifest{}, errors.New("invalid or duplicate research candidate")
		}
		urls[c.URL] = true
		_, e := sourceURL(c.URL)
		choose := e == nil && selected < limit && !topics[c.Topic] && !publishers[PublisherGroup(c.Publisher)] && c.Rights != "explicit_restrictions_review_required"
		if choose {
			selected++
			topics[c.Topic] = true
			publishers[PublisherGroup(c.Publisher)] = true
		}
		m.Entries = append(m.Entries, Entry{Candidate: c, Selected: choose})
	}
	return m, m.Validate()
}
func Parse(b []byte, digest string) (Manifest, error) {
	var m Manifest
	if len(b) > MaxFileBytes || len(digest) != 64 || fmt.Sprintf("%x", sha256.Sum256(b)) != digest {
		return m, errors.New("catalogue digest mismatch")
	}
	if e := decode(b, &m); e != nil {
		return m, e
	}
	return m, m.Validate()
}
func (m Manifest) Validate() error {
	bad := errors.New("invalid curated metadata policy")
	if _, e := hex.DecodeString(m.ResearchSHA256); e != nil {
		return bad
	}
	if m.SchemaVersion != 1 || m.Profile != "curated-v1" || len(m.ResearchSHA256) != 64 || len(m.Entries) < 1 || len(m.Entries) > 500 || m.IntervalSeconds < 86400 || m.IntervalSeconds > 604800 || m.MaxBytes < 1024 || m.MaxBytes > 2<<20 {
		return bad
	}
	if m.Enabled && !bounded(m.ApprovalRecord, 512) {
		return bad
	}
	urls, topics := map[string]bool{}, map[string]bool{}
	selected := 0
	for _, v := range m.Entries {
		if !valid(v.Candidate) || urls[v.URL] {
			return bad
		}
		urls[v.URL] = true
		if !v.Selected {
			continue
		}
		if _, e := sourceURL(v.URL); e != nil {
			return bad
		}
		selected++
		topics[v.Topic] = true
		if m.Enabled && !bounded(v.MetadataBasis, 2048) {
			return errors.New("selected source requires explicit metadata-use review including restrictions")
		}
	}
	if selected < 1 || selected > 100 || len(topics) > 20 {
		return bad
	}
	return nil
}

// Admission URL syntax matches the current feed transport; network/DNS safety
// remains the fetch adapter's responsibility. Noncanonical aliases need review.
func sourceURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || !utf8.ValidString(raw) || strings.ContainsAny(raw, "\x00\r\n\t ") || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || strings.HasSuffix(u.Hostname(), ".") {
		return nil, errors.New("unsupported source URL")
	}
	if u.Hostname() != strings.ToLower(u.Hostname()) || u.Port() == "443" {
		return nil, errors.New("source URL alias requires canonical review")
	}
	return u, nil
}
