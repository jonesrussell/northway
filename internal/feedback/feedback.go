// Package feedback records explicit, tenant-scoped preference events. It does
// not infer interests, change source eligibility or train a ranking model.
package feedback

import (
	"context"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"time"
)

var ErrInvalid = errors.New("invalid feedback event")
var ErrConflict = errors.New("feedback event conflicts with existing state")

type Event struct {
	EventID         string `json:"event_id"`
	SnapshotID      string `json:"snapshot_id"`
	ArticleID       string `json:"article_id"`
	Action          string `json:"action"`
	ReversesEventID string `json:"reverses_event_id,omitempty"`
}

func (e Event) Validate() error {
	for _, id := range []string{e.EventID, e.SnapshotID, e.ArticleID} {
		if identity.ValidateID(id) != nil {
			return ErrInvalid
		}
	}
	switch e.Action {
	case "save", "dismiss", "less_like_this":
		if e.ReversesEventID != "" {
			return ErrInvalid
		}
	case "undo":
		if identity.ValidateID(e.ReversesEventID) != nil || e.EventID == e.ReversesEventID {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

type Store interface {
	RecordFeedback(context.Context, identity.Principal, Event) error
}
type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }
func (s *Service) Submit(ctx context.Context, p identity.Principal, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.store.RecordFeedback(ctx, p, e)
}
