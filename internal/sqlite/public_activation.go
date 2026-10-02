package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ActivatePublicCanary applies one explicit metadata-only owner decision to the
// installed immutable ten-source register. It neither imports nor fetches.
func (s *Store) ActivatePublicCanary(ctx context.Context, approval string) (int, error) {
	if !text(approval, 256, false) {
		return 0, errors.New("bounded approval record required")
	}
	r, err := reviewedPublicRegister("curated-canary", PublicCanaryRegisterSHA256)
	if err != nil {
		return 0, err
	}
	entries := []publicRegisterEntry{}
	for _, entry := range r.Entries {
		if entry.Selected {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 10 {
		return 0, errors.New("canary identity mismatch")
	}
	err = s.publicWrite(ctx, func(tx *sql.Tx) error {
		additional := 0
		// Validate the entire set before any change. Do not revive a denied source
		// or override another operator's policy on a replay.
		for _, entry := range entries {
			var url, review, rights, provenance, lastError string
			var enabled int
			if e := tx.QueryRowContext(ctx, `SELECT s.canonical_url,s.review_state,s.rights_basis,s.provenance,s.enabled,p.last_error FROM public_sources s JOIN public_poll_sources p ON p.source_id=s.id WHERE s.id=$1 FOR UPDATE OF s,p`, PublicSourceID(entry.URL)).Scan(&url, &review, &rights, &provenance, &enabled, &lastError); e != nil {
				return e
			}
			expected := fmt.Sprintf("curated-v1 sha256:%s; %s", PublicBroadRegisterSHA256, entry.Basis)
			if url != canonicalPublicURL(entry.URL) || lastError != "" || !((review == "pending" && rights == "not_assessed" && enabled == 0 && provenance == expected) || (review == "approved" && rights == "reviewed_metadata" && enabled == 1 && provenance == expected+"; activation="+approval)) {
				return errors.New("canary policy conflicts; explicit review required")
			}
			if enabled == 0 {
				additional++
			}
		}
		var slots int
		var daily float64
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM poll_sources)+(SELECT count(*) FROM public_poll_sources)`).Scan(&slots); e != nil {
			return e
		}
		if e := tx.QueryRowContext(ctx, `SELECT coalesce(sum(86400000000.0/interval_us),0) FROM (SELECT interval_us FROM poll_sources WHERE enabled=1 UNION ALL SELECT p.interval_us FROM public_poll_sources p JOIN public_sources s ON s.id=p.source_id WHERE s.enabled=1) schedules`).Scan(&daily); e != nil {
			return e
		}
		if slots > 100 || daily+float64(additional) > 180 {
			return errors.New("combined acquisition capacity exceeded")
		}
		for _, entry := range entries {
			id := PublicSourceID(entry.URL)
			provenance := fmt.Sprintf("curated-v1 sha256:%s; %s; activation=%s", PublicBroadRegisterSHA256, entry.Basis, approval)
			if _, e := tx.ExecContext(ctx, `UPDATE public_sources SET review_state='approved',rights_basis='reviewed_metadata',enabled=1,policy_revision=policy_revision+1,provenance=$2 WHERE id=$1 AND enabled=0`, id, provenance); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, `UPDATE public_poll_sources SET interval_us=86400000000,max_bytes=2097152 WHERE source_id=$1`, id); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// PublicPollHealthy reports freshness without requiring a customer workspace.
func (s *Store) PublicPollHealthy(ctx context.Context) (bool, error) {
	var healthy bool
	err := s.readers.QueryRowContext(ctx, `SELECT count(*)>0 AND coalesce(bool_and(s.enabled=1 AND p.last_error='' AND p.last_success>=$1-2*p.interval_us),false) FROM public_sources s JOIN public_poll_sources p ON p.source_id=s.id WHERE s.onboarding=1 AND s.review_state='approved'`, s.queryTime().UnixMicro()).Scan(&healthy)
	return healthy, err
}
