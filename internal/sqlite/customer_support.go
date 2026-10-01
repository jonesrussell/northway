package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"github.com/jonesrussell/northway/internal/sqlite/sqlc"
	"io"
)

// CustomerSupport is an explicitly offline operator path. Open's exclusive file
// lock refuses a running server; no in-flight poll/query can race deletion.
// Export is NDJSON, paged to bound memory, and contains no credential digests.
func CustomerSupport(ctx context.Context, path string, tenant identity.TenantID, action string, out io.Writer) error {
	if tenant.Validate() != nil {
		return identity.ErrForbidden
	}
	if action != "export" && action != "suspend" && action != "delete" {
		return errors.New("invalid customer support action")
	}
	s, err := Open(ctx, path)
	if err != nil {
		return err
	}
	defer s.Close()
	q := sqlc.New(s.readers)
	if _, err = q.CustomerWorkspaceState(ctx, string(tenant)); err != nil {
		return identity.ErrNotFound
	}
	if action == "export" {
		encoder := json.NewEncoder(out)
		if err = encoder.Encode(map[string]any{"schema": "northcloud.customer-export.v1", "tenant_id": tenant}); err != nil {
			return err
		}
		emit := func(kind string, values any) error {
			return encoder.Encode(map[string]any{"type": kind, "records": values})
		}
		for offset := int64(0); ; offset += 100 {
			rows, e := q.ExportCustomerFeeds(ctx, sqlc.ExportCustomerFeedsParams{TenantID: string(tenant), Offset: offset})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				break
			}
			if e = emit("feeds", rows); e != nil {
				return e
			}
		}
		for offset := int64(0); ; offset += 100 {
			rows, e := q.ExportCustomerSources(ctx, sqlc.ExportCustomerSourcesParams{TenantID: string(tenant), Offset: offset})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				break
			}
			if e = emit("sources", rows); e != nil {
				return e
			}
		}
		for offset := int64(0); ; offset += 100 {
			rows, e := q.ExportCustomerArticles(ctx, sqlc.ExportCustomerArticlesParams{TenantID: string(tenant), Offset: offset})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				break
			}
			if e = emit("articles", rows); e != nil {
				return e
			}
		}
		for offset := int64(0); ; offset += 10 {
			rows, e := q.ExportCustomerSnapshots(ctx, sqlc.ExportCustomerSnapshotsParams{TenantID: string(tenant), Offset: offset})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				break
			}
			if e = emit("snapshots", rows); e != nil {
				return e
			}
		}
		for offset := int64(0); ; offset += 100 {
			rows, e := q.ExportCustomerFeedback(ctx, sqlc.ExportCustomerFeedbackParams{TenantID: string(tenant), Offset: offset})
			if e != nil {
				return e
			}
			if len(rows) == 0 {
				break
			}
			if e = emit("feedback", rows); e != nil {
				return e
			}
		}
		return nil
	}
	return s.writeOperational(ctx, func(q *sqlc.Queries) error {
		if action == "suspend" {
			_, err := q.SuspendCustomer(ctx, string(tenant))
			return err
		}
		if err := q.PreserveErasedAcquisitionUsage(ctx, string(tenant)); err != nil {
			return err
		}
		for _, remove := range []func(context.Context, string) error{q.DeleteCustomerKeyExpiries, q.DeleteCustomerKeys, q.DeleteCustomerFeedback, q.DeleteCustomerWork, q.DeleteCustomerSnapshots, q.DeleteCustomerPollAttempts, q.DeleteCustomerPollSources, q.DeleteCustomerPollCursors, q.DeleteCustomerFeedSources, q.DeleteCustomerFeeds, q.DeleteCustomerArticles, q.DeleteCollectionEvents, q.DeleteCollectionItems, q.DeleteCustomerSources, q.DeleteCustomerBudgets, q.DeleteCustomerRequests, q.DeleteCustomerMarker} {
			if err := remove(ctx, string(tenant)); err != nil {
				return err
			}
		}
		return nil
	})
}
