package feed

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func validPreferences() Preferences {
	return Preferences{Categories: []string{"development"}, Sources: []SourceRule{{SourceID: "00000001-0000-4000-8000-000000000000", PublisherGroup: "publisher", Categories: []string{"development"}}}, PublisherCap: 2}
}
func TestPreferencesRejectInvalidPolicy(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Preferences)
	}{
		{"missing categories", func(p *Preferences) { p.Categories = nil }},
		{"unknown category", func(p *Preferences) { p.Categories = []string{"sports"} }},
		{"duplicate category", func(p *Preferences) { p.Categories = append(p.Categories, "development") }},
		{"excess categories", func(p *Preferences) { p.Categories = make([]string, 6) }},
		{"missing sources", func(p *Preferences) { p.Sources = nil }},
		{"excess sources", func(p *Preferences) { p.Sources = make([]SourceRule, 101) }},
		{"duplicate source", func(p *Preferences) { p.Sources = append(p.Sources, p.Sources[0]) }},
		{"invalid source", func(p *Preferences) { p.Sources[0].SourceID = "bad-id" }},
		{"unselected source category", func(p *Preferences) { p.Sources[0].Categories = []string{"world"} }},
		{"duplicate source category", func(p *Preferences) { p.Sources[0].Categories = append(p.Sources[0].Categories, "development") }},
		{"missing source category", func(p *Preferences) { p.Sources[0].Categories = nil }},
		{"too many source categories", func(p *Preferences) { p.Sources[0].Categories = make([]string, 6) }},
		{"invalid publisher group", func(p *Preferences) { p.Sources[0].PublisherGroup = "publisher\"" }},
		{"empty publisher group", func(p *Preferences) { p.Sources[0].PublisherGroup = "" }},
		{"long publisher group", func(p *Preferences) { p.Sources[0].PublisherGroup = strings.Repeat("a", 65) }},
		{"zero cap", func(p *Preferences) { p.PublisherCap = 0 }},
		{"excess cap", func(p *Preferences) { p.PublisherCap = 3 }},
		{"FTS exclusion syntax", func(p *Preferences) { p.Exclude = []string{`a" OR title:b`} }},
		{"empty exclusion", func(p *Preferences) { p.Exclude = []string{""} }},
		{"spaced exclusion", func(p *Preferences) { p.Exclude = []string{" hello "} }},
		{"long exclusion", func(p *Preferences) { p.Exclude = []string{strings.Repeat("x", 65)} }},
		{"invalid UTF8", func(p *Preferences) { p.Exclude = []string{string([]byte{0xff})} }},
		{"too many exclusions", func(p *Preferences) { p.Exclude = make([]string, 21) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validPreferences()
			tc.change(&p)
			if p.Validate() == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
}
func TestPreferencesAcceptBoundedUnicodeAndMaximumSources(t *testing.T) {
	p := validPreferences()
	p.Exclude = []string{"café", "语言", "123"}
	p.Sources = nil
	for i := 0; i < 100; i++ {
		p.Sources = append(p.Sources, SourceRule{SourceID: fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1), PublisherGroup: "publisher_1-a", Categories: []string{"development"}})
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip Preferences
	if json.Unmarshal(data, &roundtrip) != nil || roundtrip.Validate() != nil {
		t.Fatal("policy roundtrip failed")
	}
}
