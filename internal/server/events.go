package server

import (
	"context"
	"database/sql"
	"fmt"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// Resolve identity before acquiring the single SQL connection.
func (s *Server) ownerEvent(ctx context.Context, ownerID int64, build func() (string, string, map[string]any)) core.CommitHook {
	if !s.core.Configured() {
		return nil
	}
	u, err := s.store.GetUser(ctx, ownerID)
	return func(tx *sql.Tx) error {
		if err != nil {
			return err
		}
		topic, subject, payload := build()
		payload["owner_id"] = ownerID
		payload["owner_email"] = u.Email
		payload["user_email"] = u.Email
		return core.QueueEvent(ctx, tx, topic, subject, payload)
	}
}

// reminderEvent queues garage.reminder_saved in the reminder's own write, so
// Pocket Renewals sees registration, inspection and insurance dates at once.
func (s *Server) reminderEvent(ctx context.Context, v *store.Vehicle, rm *store.Reminder) core.CommitHook {
	return s.ownerEvent(ctx, v.OwnerID, func() (string, string, map[string]any) {
		return "garage.reminder_saved", fmt.Sprintf("reminder:%d", rm.ID), map[string]any{
			"reminder_id": rm.ID, "vehicle_id": v.ID, "vehicle": v.Title(), "title": rm.Title, "due_date": rm.DueDate,
		}
	})
}

// documentEvent queues garage.document_saved (a paper and its end date).
func (s *Server) documentEvent(ctx context.Context, v *store.Vehicle, d *store.Document) core.CommitHook {
	return s.ownerEvent(ctx, v.OwnerID, func() (string, string, map[string]any) {
		return "garage.document_saved", fmt.Sprintf("document:%d", d.ID), map[string]any{
			"document_id": d.ID, "vehicle_id": v.ID, "vehicle": v.Title(), "name": d.Name, "kind": d.Kind, "expires_on": d.ExpiresOn,
		}
	})
}
