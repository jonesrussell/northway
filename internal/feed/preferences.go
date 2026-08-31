package feed

import (
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Preferences is saved operator policy, never inferred from request context.
type Preferences struct {
	Categories   []string     `json:"categories"`
	Sources      []SourceRule `json:"sources"`
	Exclude      []string     `json:"exclude"`
	UseContext   bool         `json:"use_context"`
	PublisherCap int          `json:"publisher_cap"`
}
type SourceRule struct {
	SourceID       string   `json:"source_id"`
	PublisherGroup string   `json:"publisher_group"`
	Categories     []string `json:"categories"`
}

func Category(v string) bool {
	switch v {
	case "development", "entertainment", "canada", "first_nations", "world":
		return true
	}
	return false
}
func (p Preferences) Validate() error {
	bad := errors.New("invalid feed preferences")
	if len(p.Categories) < 1 || len(p.Categories) > 5 || len(p.Sources) < 1 || len(p.Sources) > 100 || len(p.Exclude) > 20 || p.PublisherCap < 1 || p.PublisherCap > 2 {
		return bad
	}
	cats := map[string]bool{}
	for _, v := range p.Categories {
		if !Category(v) || cats[v] {
			return bad
		}
		cats[v] = true
	}
	ids := map[string]bool{}
	for _, s := range p.Sources {
		if identity.ValidateID(s.SourceID) != nil || ids[s.SourceID] || len(s.PublisherGroup) < 1 || len(s.PublisherGroup) > 64 || len(s.Categories) < 1 || len(s.Categories) > 5 {
			return bad
		}
		for _, c := range s.PublisherGroup {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return bad
			}
		}
		ids[s.SourceID] = true
		seen := map[string]bool{}
		for _, v := range s.Categories {
			if !cats[v] || seen[v] {
				return bad
			}
			seen[v] = true
		}
	}
	// Exclusions are single literal FTS tokens, not syntax or inferred dislikes.
	for _, v := range p.Exclude {
		if !utf8.ValidString(v) || len(v) < 1 || len(v) > 64 || strings.TrimSpace(v) != v {
			return bad
		}
		for _, c := range v {
			if !unicode.IsLetter(c) && !unicode.IsNumber(c) {
				return bad
			}
		}
	}
	return nil
}
