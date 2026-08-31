// Package query implements bounded deterministic metadata retrieval and durable
// query work. Model calls and product HTTP routes remain separate.
package query

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jonesrussell/northway/internal/identity"
)

var (
	ErrInvalid     = errors.New("invalid query input")
	ErrConflict    = errors.New("query conflicts with durable state")
	ErrInProgress  = errors.New("query in progress")
	ErrUnavailable = errors.New("query requires a new key or operator reconciliation")
)

const RetryAfter = time.Second

type Technology struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}
type Context struct {
	Intent       string       `json:"intent"`
	Technologies []Technology `json:"technologies"`
	Focus        []string     `json:"focus,omitempty"`
}
type Request struct {
	FeedID      string  `json:"feed_id"`
	Context     Context `json:"context"`
	MaxAgeHours int     `json:"max_age_hours"`
	Limit       int     `json:"limit"`
}

func bounded(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0)
}

func ValidKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range []byte(key) {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

// Digest binds the fixed route and validated typed request. Object key order
// and transport whitespace are immaterial; array order and text remain exact.
// Only this digest, never raw context or the caller's key, enters storage.
func (r Request) Digest() ([32]byte, error) {
	if identity.ValidateID(r.FeedID) != nil || r.Limit < 1 || r.Limit > 20 || r.MaxAgeHours < 1 || r.MaxAgeHours > 720 || !bounded(r.Context.Intent, 2000) || r.Context.Technologies == nil || len(r.Context.Technologies) > 32 || len(r.Context.Focus) > 12 {
		return [32]byte{}, ErrInvalid
	}
	for _, t := range r.Context.Technologies {
		if !bounded(t.Name, 100) || (t.Version != "" && !bounded(t.Version, 100)) {
			return [32]byte{}, ErrInvalid
		}
	}
	for _, f := range r.Context.Focus {
		if !bounded(f, 100) {
			return [32]byte{}, ErrInvalid
		}
	}
	b, err := json.Marshal(r)
	if err != nil || len(b) > 32768 {
		return [32]byte{}, ErrInvalid
	}
	return sha256.Sum256(append([]byte("POST /v1/feed-queries\x00"), b...)), nil
}

// Policy is trusted application configuration, never request/model input.
// WorstCaseMicros must cover the entire single capped provider attempt in USD
// millionths, including its maximum input/output and any provider overhead.
type Policy struct {
	RankerVersion   string
	WorstCaseMicros int64
	Lease           time.Duration
	CacheTTL        time.Duration
}

func (p Policy) Validate() error {
	if !bounded(p.RankerVersion, 100) || p.WorstCaseMicros < 0 || p.Lease < time.Second || p.Lease > 5*time.Minute || p.CacheTTL < time.Second || p.CacheTTL > time.Hour {
		return ErrInvalid
	}
	return nil
}

// Item is an immutable storage projection, not the draft public response. The
// caller supplies an allowed article ID/version and a validated explanation;
// storage copies the title/link from that version's current corpus row.
type Item struct {
	ArticleID   string
	SourceID    string
	ContentHash string
	Title       string
	URL         string
	Explanation string
	SourceName  string
	PublishedAt *time.Time
	ObservedAt  time.Time
	Category    string
}
type Selection struct{ ArticleID, ContentHash, Explanation, Category string }

func (s Selection) Validate() error {
	if identity.ValidateID(s.ArticleID) != nil || len(s.ContentHash) != 64 || strings.Trim(s.ContentHash, "0123456789abcdef") != "" || !bounded(s.Explanation, 1000) {
		return ErrInvalid
	}
	return nil
}

type Snapshot struct {
	ID, FeedID, RankerVersion, Mode string
	FeedRevision                    int64
	GeneratedAt, ExpiresAt          time.Time
	Items                           []Item
	Suppressed                      bool
	Details                         *Details
}
type Claim struct {
	// WorkID is an internal coordination handle, never an HTTP write capability.
	WorkID          string
	ProviderAllowed bool
	Snapshot        *Snapshot
}

// Settlement is accepted only from trusted provider adapters/reconciliation.
// Unknown retains the entire hold; zero cost requires affirmative evidence
// once StartProvider has committed. Timeout alone is never that evidence.
type Settlement struct {
	Known        bool
	ActualMicros int64
}
