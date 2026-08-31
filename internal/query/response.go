package query

import (
	"github.com/jonesrussell/northway/internal/identity"
	"strings"
	"time"
	"unicode/utf8"
)

type Ranking struct {
	Mode    string `json:"mode"`
	Version string `json:"version"`
}
type Coverage struct {
	Status   string `json:"status"`
	Selected int    `json:"sources_selected"`
	Current  int    `json:"sources_current"`
}
type Evidence struct {
	URL   string `json:"source_url"`
	Text  string `json:"text"`
	Basis string `json:"basis"`
}
type ResponseItem struct {
	ArticleID   string     `json:"article_id"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	SourceName  string     `json:"source_name"`
	PublishedAt *time.Time `json:"published_at"`
	ObservedAt  time.Time  `json:"observed_at"`
	Summary     string     `json:"summary"`
	WhyRelevant string     `json:"why_relevant"`
	Evidence    []Evidence `json:"evidence"`
}
type Response struct {
	RequestID    string         `json:"request_id"`
	SnapshotID   string         `json:"snapshot_id"`
	FeedID       string         `json:"feed_id"`
	FeedRevision int64          `json:"feed_revision"`
	GeneratedAt  time.Time      `json:"generated_at"`
	ExpiresAt    time.Time      `json:"expires_at"`
	Ranking      Ranking        `json:"ranking"`
	Coverage     Coverage       `json:"coverage"`
	Warnings     []string       `json:"warnings"`
	Items        []ResponseItem `json:"items"`
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// Response projects frozen evidence, rechecked access and ageing last-known
// poll health. It never rereads publisher content or triggers work.
func (s Snapshot) Response(requestID string, now time.Time) (Response, error) {
	if identity.ValidateID(requestID) != nil || s.Details == nil || len(s.Details.Sources) < 1 || len(s.Details.Sources) > 100 || len(s.Items) > 20 || now.Before(s.GeneratedAt) {
		return Response{}, ErrInvalid
	}
	r := Response{RequestID: requestID, SnapshotID: s.ID, FeedID: s.FeedID, FeedRevision: s.FeedRevision, GeneratedAt: s.GeneratedAt, ExpiresAt: s.ExpiresAt, Ranking: Ranking{s.Mode, s.RankerVersion}, Coverage: Coverage{Status: "stale", Selected: len(s.Details.Sources)}, Warnings: append([]string{}, s.Details.Warnings...), Items: []ResponseItem{}}
	for _, src := range s.Details.Sources {
		if src.Allowed && src.CurrentUntil != nil && now.Before(*src.CurrentUntil) {
			r.Coverage.Current++
		}
	}
	if r.Coverage.Current == r.Coverage.Selected {
		r.Coverage.Status = "complete"
	} else if r.Coverage.Current > 0 {
		r.Coverage.Status = "partial"
	}
	if r.Coverage.Status != "complete" {
		r.Warnings = append(r.Warnings, "Some selected sources have no current successful poll in this snapshot; coverage is not exhaustive.")
	}
	if !now.Before(s.ExpiresAt) {
		r.Warnings = append(r.Warnings, "Retained snapshot: its cache freshness window has expired; ranking has not been refreshed.")
	}
	if s.Suppressed {
		r.Warnings = append(r.Warnings, "Source access changed; revoked items are suppressed without reranking.")
	}
	undated := false
	for _, item := range s.Items {
		if item.PublishedAt == nil {
			undated = true
		}
		r.Items = append(r.Items, ResponseItem{ArticleID: item.ArticleID, Title: clip(item.Title, 500), URL: item.URL, SourceName: clip(item.SourceName, 200), PublishedAt: item.PublishedAt, ObservedAt: item.ObservedAt, Summary: "Headline metadata only; read the linked source for details.", WhyRelevant: item.Explanation, Evidence: []Evidence{{URL: item.URL, Text: item.Title, Basis: "source_metadata"}}})
	}
	if undated {
		r.Warnings = append(r.Warnings, "Publication date is unknown for some items; observation time is used for eligibility, not proof of recent publication.")
	}
	for _, cat := range s.Details.Categories {
		found := false
		for _, item := range s.Items {
			if item.Category == cat {
				found = true
				break
			}
		}
		already := false
		for _, w := range r.Warnings {
			if strings.Contains(w, "category: "+cat+".") {
				already = true
			}
		}
		if !found && !already {
			r.Warnings = append(r.Warnings, "No eligible item for category: "+cat+". Access changes may have removed retained items.")
		}
	}
	if len(r.Warnings) > 20 {
		return Response{}, ErrInvalid
	}
	return r, nil
}
