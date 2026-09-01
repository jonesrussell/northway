package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
)

// Poll reservations, corpus writes and checkpoints use generated SQL within
// short serialized transactions. No handle or callback leaves this package.

func pollURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && link(raw) && u.RawQuery == "" && !u.ForceQuery && (u.Port() == "" || u.Port() == "443") && !strings.ContainsAny(raw, "\r\n\t ")
}

// ConfigurePoll is an operator seam, not a public URL-ingestion endpoint. The
// caller must record approval/rights externally before setting both flags true.
// There is no catalogue loader, default schedule or automatic approval.
func (s *Store) ConfigurePoll(ctx context.Context, p identity.Principal, v ingest.Policy) error {
	tenant, err := access(p, true, v.SourceID)
	if err != nil {
		return err
	}
	if !pollURL(v.URL) || v.Interval < time.Hour || v.Interval > 7*24*time.Hour || v.MaxBytes < 1024 || v.MaxBytes > ingest.MaxResponseBytes || v.Enabled && !v.Approved {
		return ingest.ErrInvalid
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		current, err := q.PollSourceURL(ctx, sqlc.PollSourceURLParams{TenantID: string(tenant), SourceID: v.SourceID})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current != v.URL {
			return ingest.ErrInvalid
		}
		count, err := q.OtherPollSources(ctx, sqlc.OtherPollSourcesParams{TenantID: string(tenant), SourceID: v.SourceID})
		if err != nil {
			return err
		}
		if count >= ingest.MaxSources {
			return ingest.ErrBudget
		}
		now := s.queryTime()
		if !validTimestamp(now) {
			return ingest.ErrInvalid
		}
		var approved, enabled int64
		if v.Approved {
			approved = 1
		}
		if v.Enabled {
			enabled = 1
		}
		return q.ConfigurePoll(ctx, sqlc.ConfigurePollParams{TenantID: string(tenant), SourceID: v.SourceID, ApprovedUrl: v.URL, Approved: approved, Enabled: enabled, IntervalUs: v.Interval.Microseconds(), MaxBytes: v.MaxBytes, NextAt: now.UnixMicro()})
	})
}

func (s *Store) ClaimPoll(ctx context.Context, p identity.Principal) (ingest.Claim, error) {
	tenant, err := p.RequireOperator()
	if err != nil {
		return ingest.Claim{}, err
	}
	var claim ingest.Claim
	var outcome error
	err = s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		if !validTimestamp(now) || !validTimestamp(now.Add(7*24*time.Hour)) {
			return ingest.ErrInvalid
		}
		at := now.UnixMicro()
		// Expired workers remain charged in full; a new claim never accepts their
		// result. Recovery also updates the last error instead of leaving "pending".
		if err := q.AbandonPollSources(ctx, at); err != nil {
			return err
		}
		if err := q.AbandonPollAttempts(ctx, at); err != nil {
			return err
		}
		if err := q.ExpirePollAttempts(ctx, now.Add(-24*time.Hour).UnixMicro()); err != nil {
			return err
		}
		active, err := q.ActivePollAttempts(ctx)
		if err != nil {
			return err
		}
		if active != 0 {
			outcome = ingest.ErrBusy
			return nil
		}
		cursor, err := q.PollCursor(ctx, string(tenant))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// Only due, approved, enabled sources enter this bounded cyclic selection.
		// Held/not-due sources are skipped without pinning the cursor. Budget failure
		// leaves the next due source first, regardless of whether bytes or attempts ran out.
		due, err := q.NextPollSources(ctx, sqlc.NextPollSourcesParams{TenantID: string(tenant), NowAt: at})
		if err != nil {
			return err
		}
		if len(due) == 0 {
			outcome = ingest.ErrIdle
			return nil
		}
		selected := due[0]
		for _, v := range due {
			if v.SourceID > cursor {
				selected = v
				break
			}
		}
		window, err := q.PollWindow(ctx)
		if err != nil {
			return err
		}
		if window.Attempts >= ingest.DailyAttempts || selected.MaxBytes > ingest.DailyBytes-window.Used {
			outcome = ingest.ErrBudget
			return nil
		}
		claim = ingest.Claim{ID: queryID(), SourceID: selected.SourceID, URL: selected.ApprovedUrl, ETag: selected.Etag, LastModified: selected.Modified, MaxBytes: selected.MaxBytes, Until: now.Add(ingest.LeaseDuration)}
		if err := q.InsertPollAttempt(ctx, sqlc.InsertPollAttemptParams{ID: claim.ID, TenantID: string(tenant), SourceID: claim.SourceID, StartedAt: at, LeaseUntil: claim.Until.UnixMicro(), ChargedAt: at, ChargedBytes: claim.MaxBytes, ReservedBytes: claim.MaxBytes}); err != nil {
			return err
		}
		if err := q.MarkPollStarted(ctx, sqlc.MarkPollStartedParams{ClaimID: sql.NullString{String: claim.ID, Valid: true}, LastAttempt: sql.NullInt64{Int64: at, Valid: true}, NextAt: at + selected.IntervalUs, TenantID: string(tenant), SourceID: claim.SourceID}); err != nil {
			return err
		}
		return q.AdvancePollCursor(ctx, sqlc.AdvancePollCursorParams{TenantID: string(tenant), SourceID: claim.SourceID})
	})
	if err != nil {
		if errors.Is(err, ErrStoragePressure) {
			return ingest.Claim{}, ingest.ErrCorpusFull
		}
		return ingest.Claim{}, err
	}
	if outcome != nil {
		return ingest.Claim{}, outcome
	}
	return claim, nil
}

func validHeader(v string) bool {
	return len(v) <= 1024 && text(v, 1024, true) && !strings.ContainsAny(v, "\r\n")
}
func validResult(r ingest.Result) bool {
	if r.Bytes < 0 || r.Bytes > ingest.MaxResponseBytes || len(r.Items) > ingest.MaxItems || !validHeader(r.ETag) || !validHeader(r.LastModified) || (!r.NotBefore.IsZero() && !validTimestamp(r.NotBefore)) {
		return false
	}
	switch r.Failure {
	case "", "transport", "http", "encoding", "body", "parse", "no_store", "corpus_full", "cancelled":
	default:
		return false
	}
	if r.Status < 0 || r.Status > 599 {
		return false
	}
	if r.Failure != "" {
		return len(r.Items) == 0
	}
	if r.Status != 200 && r.Status != 304 || r.Status == 304 && (len(r.Items) != 0 || r.Bytes != 0) {
		return false
	}
	seen := map[string]bool{}
	for _, v := range r.Items {
		if !text(v.OriginID, 2048, false) || !link(v.URL) || !text(v.Title, 512, false) || v.PublishedAt != nil && !validTimestamp(*v.PublishedAt) || seen[v.OriginID] {
			return false
		}
		seen[v.OriginID] = true
	}
	return true
}

func itemIDFor(source, origin string) string {
	sum := sha256.Sum256([]byte(source + "\x00" + origin))
	b := sum[:16]
	b[6] = b[6]&15 | 80
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (s *Store) FinishPoll(ctx context.Context, p identity.Principal, id string, r ingest.Result) error {
	tenant, err := access(p, true, id)
	if err != nil {
		return err
	}
	if !validResult(r) {
		return ingest.ErrInvalid
	}
	err = s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		if !validTimestamp(now) {
			return ingest.ErrInvalid
		}
		w, err := q.PendingPoll(ctx, sqlc.PendingPollParams{TenantID: string(tenant), ID: id})
		if errors.Is(err, sql.ErrNoRows) {
			return ingest.ErrLease
		}
		if err != nil {
			return err
		}
		if w.LeaseUntil <= now.UnixMicro() {
			return ingest.ErrLease
		}
		if r.Bytes > w.ReservedBytes {
			return ingest.ErrInvalid
		}
		if r.Failure == "" && r.Status == 304 && w.Etag == "" && w.Modified == "" {
			return ingest.ErrInvalid
		}
		if r.Failure == "" && r.Status == 200 {
			for _, v := range r.Items {
				if err := putFeedItem(ctx, q, string(tenant), w.SourceID, v, now); err != nil {
					return err
				}
			}
			items, err := q.SourceItemCount(ctx, sqlc.SourceItemCountParams{TenantID: string(tenant), SourceID: w.SourceID})
			if err != nil {
				return err
			}
			versions, err := q.SourceVersionCount(ctx, sqlc.SourceVersionCountParams{TenantID: string(tenant), SourceID: w.SourceID})
			if err != nil {
				return err
			}
			if items > ingest.MaxSourceItems || versions > ingest.MaxSourceVersions {
				return ingest.ErrCorpusFull
			}
		}
		// Failed/uncertain transfer bytes are conservatively charged in full. Known
		// success releases unused bytes, but never releases the attempt itself.
		charged := w.ReservedBytes
		if r.Failure == "" {
			charged = r.Bytes
		}
		if err := q.SettlePollAttempt(ctx, sqlc.SettlePollAttemptParams{ChargedBytes: charged, ChargedAt: now.UnixMicro(), TenantID: string(tenant), ID: id}); err != nil {
			return err
		}
		next := int64(0)
		if !r.NotBefore.IsZero() {
			next = r.NotBefore.UnixMicro()
		}
		if r.Failure != "" {
			return q.MarkPollFailure(ctx, sqlc.MarkPollFailureParams{LastError: r.Failure, LastStatus: int64(r.Status), HoldUntil: next, TenantID: string(tenant), SourceID: w.SourceID})
		}
		if r.Status == 304 {
			if r.ETag == "" {
				r.ETag = w.Etag
			}
			if r.LastModified == "" {
				r.LastModified = w.Modified
			}
		}
		return q.MarkPollSuccess(ctx, sqlc.MarkPollSuccessParams{LastSuccess: sql.NullInt64{Int64: now.UnixMicro(), Valid: true}, LastStatus: int64(r.Status), Etag: r.ETag, Modified: r.LastModified, HoldUntil: next, TenantID: string(tenant), SourceID: w.SourceID})
	})
	if errors.Is(err, ErrStoragePressure) {
		return ingest.ErrCorpusFull
	}
	return err
}

func putFeedItem(ctx context.Context, q *sqlc.Queries, tenant, source string, v ingest.Item, now time.Time) error {
	var published sql.NullInt64
	if v.PublishedAt != nil {
		published = sql.NullInt64{Int64: v.PublishedAt.UnixMicro(), Valid: true}
	}
	// Hash includes URL/date: those edits create versions, while unchanged 200s
	// preserve article observation time, FTS and corpus revision without writes.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v:%d", v.Title, v.URL, published.Valid, published.Int64)))
	hash := hex.EncodeToString(sum[:])
	id := itemIDFor(source, v.OriginID)
	existingID, err := q.PollItemByOrigin(ctx, sqlc.PollItemByOriginParams{TenantID: tenant, SourceID: source, OriginID: v.OriginID})
	if err == nil {
		id = existingID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	old, err := q.PollItemIdentity(ctx, sqlc.PollItemIdentityParams{TenantID: tenant, ID: id})
	if err == nil && (old.SourceID != source || old.OriginID != v.OriginID) {
		return ingest.ErrInvalid
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	n, err := q.PutPollItem(ctx, sqlc.PutPollItemParams{TenantID: tenant, ID: id, SourceID: source, OriginID: v.OriginID, Url: v.URL, Title: v.Title, ContentHash: hash, PublishedAt: published, ObservedAt: now.UnixMicro()})
	if err != nil || n == 0 {
		return err
	}
	return q.RecordVersion(ctx, sqlc.RecordVersionParams{TenantID: tenant, ArticleID: id, ContentHash: hash, Title: v.Title, Body: "", ObservedAt: now.UnixMicro()})
}

var _ ingest.Store = (*Store)(nil)

// ResetPollSchedule is an explicit local-operator recovery seam after review of
// publisher headers/policy. Unlike ConfigurePoll, it may shorten an existing
// hold. It neither enables collection nor refunds attempts/bytes. Pending work
// is fenced, marked reset and remains charged. Minimum spacing from the last
// attempt is preserved. There is deliberately no public reset route.
func (s *Store) ResetPollSchedule(ctx context.Context, p identity.Principal, sourceID string) error {
	tenant, err := access(p, true, sourceID)
	if err != nil {
		return err
	}
	return s.write(ctx, func(q *sqlc.Queries) error {
		now := s.queryTime()
		if !validTimestamp(now) {
			return ingest.ErrInvalid
		}
		n, err := q.ResetPollSchedule(ctx, sqlc.ResetPollScheduleParams{NextAt: now.UnixMicro(), TenantID: string(tenant), SourceID: sourceID})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
		return nil
	})
}
