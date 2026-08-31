package feedback

import (
	"context"
	"errors"
	"github.com/jonesrussell/northway/internal/identity"
	"testing"
	"time"
)

func validEvent() Event {
	return Event{EventID: "00000001-1111-4111-8111-111111111111", SnapshotID: "00000002-1111-4111-8111-111111111111", ArticleID: "00000003-1111-4111-8111-111111111111", Action: "save"}
}
func TestEventValidation(t *testing.T) {
	for _, action := range []string{"save", "dismiss", "less_like_this", "undo"} {
		e := validEvent()
		e.Action = action
		if action == "undo" {
			e.ReversesEventID = e.ArticleID
		}
		if err := e.Validate(); err != nil {
			t.Fatal(action, err)
		}
	}
	for name, mutate := range map[string]func(*Event){
		"event ID": func(e *Event) { e.EventID = "bad" }, "snapshot ID": func(e *Event) { e.SnapshotID = "" }, "article ID": func(e *Event) { e.ArticleID = "bad" },
		"action": func(e *Event) { e.Action = "learn" }, "missing target": func(e *Event) { e.Action = "undo" }, "self reversal": func(e *Event) { e.Action = "undo"; e.ReversesEventID = e.EventID },
		"invalid target": func(e *Event) { e.Action = "undo"; e.ReversesEventID = "bad" }, "target on save": func(e *Event) { e.ReversesEventID = e.ArticleID },
	} {
		t.Run(name, func(t *testing.T) {
			e := validEvent()
			mutate(&e)
			if !errors.Is(e.Validate(), ErrInvalid) {
				t.Fatal("accepted invalid event")
			}
		})
	}
}

type deadlineStore struct {
	t      *testing.T
	called bool
}

func (s *deadlineStore) RecordFeedback(ctx context.Context, p identity.Principal, e Event) error {
	s.called = true
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Error("missing service deadline")
	}
	return ctx.Err()
}
func TestServiceDeadlineCancellationAndInvalidInput(t *testing.T) {
	store := &deadlineStore{t: t}
	service := NewService(store)
	if err := service.Submit(t.Context(), identity.Principal{}, Event{}); !errors.Is(err, ErrInvalid) || store.called {
		t.Fatal("invalid input reached storage")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := service.Submit(ctx, identity.Principal{}, validEvent()); !errors.Is(err, context.Canceled) || !store.called {
		t.Fatal("cancellation not propagated", err)
	}
}
