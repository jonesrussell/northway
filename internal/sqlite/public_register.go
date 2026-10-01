package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	assets "github.com/jonesrussell/northway/db"
	"github.com/jonesrussell/northway/internal/ingest"
)

const PublicCanaryRegisterSHA256 = "ea92fb09ec7feba7847b2dbdfe590a10ea354294f143bb3fcf8284bfdf0797ea"
const PublicBroadRegisterSHA256 = "5e14f54cf9f0376689aa0b79c4bc41466bbbd25b1a138dfe0018e7a98d80a4c5"

type publicRegisterEntry struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	Topic     string `json:"topic"`
	Language  string `json:"language"`
	Rights    string `json:"publication_rights_status"`
	Notes     string `json:"notes"`
	Selected  bool   `json:"selected"`
	Basis     string `json:"metadata_basis"`
}
type publicRegister struct {
	Schema   int                   `json:"schema_version"`
	Profile  string                `json:"profile"`
	Interval int64                 `json:"interval_seconds"`
	MaxBytes int64                 `json:"max_bytes"`
	Entries  []publicRegisterEntry `json:"entries"`
}

func reviewedPublicRegister(name, digest string) (publicRegister, error) {
	raw, e := assets.PublicRegisters.ReadFile("registers/" + name + ".json")
	if e != nil {
		return publicRegister{}, e
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != digest {
		return publicRegister{}, errors.New("public register digest mismatch")
	}
	var r publicRegister
	e = json.Unmarshal(raw, &r)
	if e != nil || r.Schema != 1 || r.Profile != "curated-v1" || r.Interval != 86400 || r.MaxBytes != ingest.MaxResponseBytes {
		return r, ingest.ErrInvalid
	}
	return r, nil
}

// InstallReviewedPublicRegister imports immutable reviewed configuration only.
// All sources stay pending/not_assessed/disabled. Top-level enabled fields in
// historical tenant-importer exports deliberately convey no activation here.
// Existing independently reviewed policy is never overwritten by replays.
func (s *Store) InstallReviewedPublicRegister(ctx context.Context) (int, error) {
	broad, e := reviewedPublicRegister("curated-broad", PublicBroadRegisterSHA256)
	if e != nil {
		return 0, e
	}
	canary, e := reviewedPublicRegister("curated-canary", PublicCanaryRegisterSHA256)
	if e != nil {
		return 0, e
	}
	selected := map[string]bool{}
	for _, entry := range canary.Entries {
		if entry.Selected {
			selected[entry.URL] = true
		}
	}
	if len(selected) != 10 {
		return 0, ingest.ErrInvalid
	}
	entries := []publicRegisterEntry{}
	for _, entry := range broad.Entries {
		if entry.Selected {
			if !pollURL(entry.URL) || !text(entry.Title, 512, false) || !text(entry.Publisher, 256, false) || !text(entry.Topic, 128, false) || !text(entry.Language, 32, false) || entry.Rights != "not_assessed" {
				return 0, ingest.ErrInvalid
			}
			entries = append(entries, entry)
		}
	}
	if len(entries) != 88 {
		return 0, ingest.ErrInvalid
	}
	e = s.publicWrite(ctx, func(tx *sql.Tx) error {
		var current int64
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM poll_sources)+(SELECT count(*) FROM public_poll_sources)`).Scan(&current); e != nil {
			return e
		}
		missing := int64(0)
		for _, entry := range entries {
			var exists bool
			if e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM public_poll_sources WHERE source_id=$1)`, PublicSourceID(entry.URL)).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				missing++
			}
		}
		if current+missing > ingest.MaxSources {
			return ingest.ErrBudget
		}
		for _, entry := range broad.Entries {
			if entry.Rights != "not_assessed" {
				if _, e := tx.ExecContext(ctx, `INSERT INTO public_register_exclusions(canonical_url,publisher,reason) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, canonicalPublicURL(entry.URL), entry.Publisher, entry.Rights+": "+entry.Notes); e != nil {
					return e
				}
			}
		}
		for _, entry := range entries {
			onboarding := 0
			if selected[entry.URL] {
				onboarding = 1
			}
			provenance := fmt.Sprintf("curated-v1 sha256:%s; %s", PublicBroadRegisterSHA256, strings.TrimSpace(entry.Basis))
			id := PublicSourceID(entry.URL)
			if _, e := tx.ExecContext(ctx, `INSERT INTO public_sources(id,canonical_url,title,publisher,topic,language,provenance,review_state,rights_basis,enabled,onboarding) VALUES($1,$2,$3,$4,$5,$6,$7,'pending','not_assessed',0,$8) ON CONFLICT DO NOTHING`, id, canonicalPublicURL(entry.URL), entry.Title, entry.Publisher, entry.Topic, entry.Language, provenance, onboarding); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO public_poll_sources(source_id,interval_us,max_bytes,next_at) VALUES($1,86400000000,$2,$3) ON CONFLICT DO NOTHING`, id, ingest.MaxResponseBytes, s.queryTime().UnixMicro()); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return 0, e
	}
	return len(entries), nil
}
