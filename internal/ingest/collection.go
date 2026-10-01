package ingest

import (
	"context"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/identity"
	"net/url"
	"strings"
	"time"
)

// Observation is metadata/reference evidence, never a binary or ownership grant.
type Observation struct {
	Version     int    `json:"version"`
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	SourceID    string `json:"source_id"`
	AccountURL  string `json:"account_url"`
	OriginalURL string `json:"original_url"`
	EvidenceURL string `json:"evidence_url"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PreviewURL  string `json:"preview_url"`
	EmbedURL    string `json:"embed_url"`
	Display     string `json:"display"`
	State       string `json:"state"`
	Method      string `json:"method"`
	ObservedAt  int64  `json:"observed_at"`
}
type Event struct {
	Cursor      int64       `json:"cursor"`
	Observation Observation `json:"observation"`
}
type Batch struct {
	Version int     `json:"version"`
	After   int64   `json:"after"`
	Next    int64   `json:"next"`
	Events  []Event `json:"events"`
}

func (o Observation) Fingerprint() []byte {
	o.ID = ""
	o.Revision = 0
	o.ObservedAt = 0
	b, _ := json.Marshal(o)
	return b
}

type CollectionStore interface {
	ClaimCollection(context.Context, identity.Principal) (Claim, error)
	FinishPoll(context.Context, identity.Principal, string, Result) error
}
type CollectionFetcher interface {
	FetchCollection(context.Context, Claim) Result
}
type Collector struct {
	store   CollectionStore
	fetcher CollectionFetcher
}

func NewCollector(s CollectionStore, f CollectionFetcher) *Collector { return &Collector{s, f} }

// RunOnce is explicit operator work; no ticker, startup call or URL expansion.
func (c *Collector) RunOnce(ctx context.Context, p identity.Principal) (Result, error) {
	if _, e := p.RequireOperator(); e != nil {
		return Result{}, e
	}
	cl, e := c.store.ClaimCollection(ctx, p)
	if e != nil {
		return Result{}, e
	}
	fc, cancel := context.WithDeadline(ctx, cl.Until)
	defer cancel()
	r := c.fetcher.FetchCollection(fc, cl)
	if ctx.Err() != nil {
		r = Result{Status: r.Status, Bytes: r.Bytes, Failure: "cancelled"}
	}
	sc, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if e = c.store.FinishPoll(sc, p, cl.ID, r); e != nil {
		if e == ErrCorpusFull {
			_ = c.store.FinishPoll(sc, p, cl.ID, Result{Status: r.Status, Bytes: r.Bytes, Failure: "corpus_full"})
		}
		return r, e
	}
	if r.Failure != "" {
		return r, ErrFetch
	}
	return r, nil
}

// SafeEmbed admits provider playback references only, never arbitrary HTML.
func SafeEmbed(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	var id string
	switch u.Hostname() {
	case "www.youtube-nocookie.com":
		if !strings.HasPrefix(u.Path, "/embed/") {
			return false
		}
		id = strings.TrimPrefix(u.Path, "/embed/")
		if len(id) != 11 {
			return false
		}
	case "player.vimeo.com":
		if !strings.HasPrefix(u.Path, "/video/") {
			return false
		}
		id = strings.TrimPrefix(u.Path, "/video/")
		if len(id) == 0 || len(id) > 20 {
			return false
		}
	default:
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || u.Hostname() == "www.youtube-nocookie.com" && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-')) {
			return false
		}
	}
	return true
}

// CollectionSeed admits one exact URL; scraped links cannot call this operation.
type CollectionSeed struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}
type CollectionStatus struct {
	Seeds     int64 `json:"seeds"`
	Enabled   int64 `json:"enabled"`
	Items     int64 `json:"items"`
	Revisions int64 `json:"revisions"`
}
