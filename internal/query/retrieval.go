package query

import (
	"context"
	"errors"
	"github.com/jonesrussell/northway/internal/article"
	"github.com/jonesrussell/northway/internal/feed"
	"github.com/jonesrussell/northway/internal/identity"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
)

const DeterministicVersion = "metadata-v1"
const CandidatesPerCategory = 50
const RetrievalTimeout = 5 * time.Second

type Candidate struct {
	Article        article.Article
	PublisherGroup string
	Category       string
}
type Corpus struct {
	Preferences feed.Preferences
	Candidates  []Candidate
	Terms       []string
	Truncated   bool
}
type Retrieval struct {
	Selections []Selection
	Warnings   []string
	Categories []string
}
type SourceHealth struct {
	SourceID     string
	CurrentUntil *time.Time
	Allowed      bool
}
type Details struct {
	Sources    []SourceHealth
	Warnings   []string
	Categories []string
}

// Terms is a bounded lexical baseline, not natural-language intent reasoning.
// Personal policies ignore workspace context; configured contextual feeds use
// technology names, focus and then intent. Versions never imply applicability.
func Terms(c Context, useContext bool) []string {
	if !useContext {
		return nil
	}
	stop := map[string]bool{}
	for _, w := range strings.Fields("a an and are as at be by for from i in is it me my of on or our please show recent latest news updates developments keep informed relevant to this the with about project working development entertainment canada canadian first nations first_nations world general") {
		stop[w] = true
	}
	values := make([]string, 0, len(c.Technologies)+len(c.Focus)+1)
	for _, t := range c.Technologies {
		values = append(values, t.Name)
	}
	values = append(values, c.Focus...)
	values = append(values, c.Intent)
	var terms []string
	seen := map[string]bool{}
	for _, v := range values {
		for _, w := range tokens(v) {
			if len(w) > 64 || len(w) < 2 || stop[w] || seen[w] {
				continue
			}
			seen[w] = true
			terms = append(terms, w)
			if len(terms) == 8 {
				return terms
			}
		}
	}
	return terms
}
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}
func LiteralMatch(terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, len(terms))
	for i, v := range terms {
		quoted[i] = `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
	}
	return "title : (" + strings.Join(quoted, " OR ") + ")"
}

// CanonicalURL removes fragments, normalizes host/default port and an empty
// path. It preserves query parameters and path escaping; no story clustering.
func CanonicalURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Fragment = ""
	u.RawFragment = ""
	u.Host = strings.ToLower(u.Host)
	if u.Scheme == "https" && u.Port() == "443" {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}
func EffectiveTime(a article.Article) time.Time {
	if a.PublishedAt != nil {
		return *a.PublishedAt
	}
	return a.ObservedAt
}

// Select is deterministic over a bounded, authorized metadata shortlist.
func Select(c Corpus, limit int) (Retrieval, error) {
	if c.Preferences.Validate() != nil || limit < 1 || limit > 20 || len(c.Candidates) > 5*CandidatesPerCategory || len(c.Terms) > 8 {
		return Retrieval{}, ErrInvalid
	}
	out := Retrieval{Selections: []Selection{}, Warnings: []string{"Metadata only: headlines do not verify event details or version applicability."}}
	cats := c.Preferences.Categories
	if len(cats) > 1 && len(cats) > limit {
		cats = cats[:limit]
		out.Warnings = append(out.Warnings, "Category selection is limited by the requested result count.")
	}
	out.Categories = slices.Clone(cats)
	if c.Truncated {
		out.Warnings = append(out.Warnings, "Candidate window capped at 50 recent eligible items per category; older matches may be omitted.")
	}
	usedURLs := map[string]bool{}
	usedIDs := map[string]bool{}
	publishers := map[string]int{}
	for _, cat := range cats {
		candidates := make([]Candidate, 0, CandidatesPerCategory)
		for _, v := range c.Candidates {
			if v.Category == cat {
				candidates = append(candidates, v)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			ta, tb := EffectiveTime(a.Article), EffectiveTime(b.Article)
			if !ta.Equal(tb) {
				return ta.After(tb)
			}
			return a.Article.ID < b.Article.ID
		})
		picked := 0
		for _, v := range candidates {
			canonical := CanonicalURL(v.Article.URL)
			if usedIDs[v.Article.ID] || usedURLs[canonical] || publishers[v.PublisherGroup] >= c.Preferences.PublisherCap {
				continue
			}
			explanation := "Recent eligible headline from the selected " + cat + " feed; details and applicability are unverified."
			if len(c.Terms) > 0 {
				explanation = "Headline matched explicit context terms within the selected feed; details and version applicability are unverified."
			}
			out.Selections = append(out.Selections, Selection{ArticleID: v.Article.ID, ContentHash: v.Article.ContentHash, Explanation: explanation, Category: cat})
			usedIDs[v.Article.ID] = true
			usedURLs[canonical] = true
			publishers[v.PublisherGroup]++
			picked++
			if len(cats) > 1 || len(out.Selections) == limit {
				break
			}
		}
		if picked == 0 {
			out.Warnings = append(out.Warnings, "No eligible item for category: "+cat+". Sources, exclusions, duplicates, publisher caps or the candidate window may limit coverage.")
		}
	}
	return out, nil
}

type RetrievalStore interface {
	BeginQuery(context.Context, identity.Principal, string, Request, Policy) (Claim, error)
	RetrieveCandidates(context.Context, identity.Principal, string, Request) (Corpus, error)
	CompleteRetrieval(context.Context, identity.Principal, string, Retrieval) (Snapshot, error)
	FailQuery(context.Context, identity.Principal, string) error
	GetSnapshot(context.Context, identity.Principal, string) (Snapshot, error)
}
type Service struct{ store RetrievalStore }

func NewService(store RetrievalStore) *Service { return &Service{store: store} }
func (s *Service) Query(ctx context.Context, p identity.Principal, key string, r Request) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, RetrievalTimeout)
	defer cancel()
	claim, err := s.store.BeginQuery(ctx, p, key, r, Policy{RankerVersion: DeterministicVersion, Lease: time.Minute, CacheTTL: time.Minute})
	if err != nil {
		return Snapshot{}, err
	}
	if claim.Snapshot != nil {
		if claim.Snapshot.Details == nil {
			return Snapshot{}, ErrUnavailable
		}
		return *claim.Snapshot, nil
	}
	corpus, err := s.store.RetrieveCandidates(ctx, p, claim.WorkID, r)
	var result Retrieval
	if err == nil {
		result, err = Select(corpus, r.Limit)
	}
	if err == nil {
		var snapshot Snapshot
		snapshot, err = s.store.CompleteRetrieval(ctx, p, claim.WorkID, result)
		if err == nil {
			return snapshot, nil
		}
	}
	cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer done()
	return Snapshot{}, errors.Join(err, s.store.FailQuery(cleanup, p, claim.WorkID))
}
func (s *Service) Get(ctx context.Context, p identity.Principal, id string) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, RetrievalTimeout)
	defer cancel()
	snap, err := s.store.GetSnapshot(ctx, p, id)
	if err == nil && snap.Details == nil {
		return Snapshot{}, ErrUnavailable
	}
	return snap, err
}
