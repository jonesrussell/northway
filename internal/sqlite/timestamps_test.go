package sqlite

import (
	"testing"
	"time"
)

func TestTimestampBounds(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	original := item()
	must(t, s.PutArticle(t.Context(), tenantA, original))
	for name, value := range map[string]time.Time{
		"zero":             {},
		"before epoch":     time.Unix(0, -1),
		"outside RFC3339":  time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
		"positive wrap":    time.Date(600000, 1, 1, 0, 0, 0, 0, time.UTC),
		"offset past end":  time.Date(9999, 12, 31, 23, 0, 0, 0, time.FixedZone("west", -3600)),
		"offset pre epoch": time.Date(1970, 1, 1, 0, 0, 0, 0, time.FixedZone("east", 3600)),
	} {
		t.Run(name, func(t *testing.T) {
			for _, field := range []string{"observed", "published"} {
				a := original
				a.Title = "Rejected change"
				if field == "observed" {
					a.ObservedAt = value
				} else {
					a.PublishedAt = &value
				}
				if err := s.PutArticle(t.Context(), tenantA, a); err == nil {
					t.Errorf("accepted invalid %s timestamp %v", field, value)
				}
			}
			if _, err := s.Search(t.Context(), tenantA, feedID, "PHP", value, 1); err == nil {
				t.Errorf("accepted invalid search timestamp %v", value)
			}
		})
	}
	got, err := s.GetArticle(t.Context(), tenantA, original.ID)
	must(t, err)
	if got.Title != original.Title || !got.ObservedAt.Equal(original.ObservedAt) || got.PublishedAt != nil {
		t.Fatal("rejected timestamp changed stored article", got)
	}
	var versions int
	must(t, s.readers.QueryRowContext(t.Context(), "SELECT count(*) FROM article_versions").Scan(&versions))
	if versions != 1 || len(search(t, s, tenantA, "Rejected")) != 0 {
		t.Fatal("rejected timestamp changed version history or FTS")
	}
}

func TestTimestampBoundaryRoundTrips(t *testing.T) {
	s, _ := fresh(t)
	seed(t, s, tenantA)
	for _, value := range []time.Time{
		time.Unix(0, 0).UTC(),
		time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC),
		time.Date(1969, 12, 31, 23, 0, 0, 0, time.FixedZone("west", -3600)),
		time.Date(10000, 1, 1, 0, 0, 0, 0, time.FixedZone("east", 3600)),
	} {
		a := item()
		a.ObservedAt, a.PublishedAt = value, &value
		must(t, s.PutArticle(t.Context(), tenantA, a))
		got, err := s.GetArticle(t.Context(), tenantA, a.ID)
		must(t, err)
		want := value.UTC().Truncate(time.Microsecond)
		if !got.ObservedAt.Equal(want) || got.PublishedAt == nil || !got.PublishedAt.Equal(want) {
			t.Fatalf("timestamp round trip: input=%v got=%v", value, got)
		}
		rows, err := s.Search(t.Context(), tenantA, feedID, "PHP", value, 1)
		must(t, err)
		if len(rows) != 1 || rows[0].ID != a.ID {
			t.Fatal("search rejected valid timestamp boundary")
		}
	}
}
