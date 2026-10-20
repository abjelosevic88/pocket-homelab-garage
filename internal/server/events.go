package server

import (
	"context"
	"database/sql"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
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
