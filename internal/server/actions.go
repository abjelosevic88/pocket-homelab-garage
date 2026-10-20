package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
	"sort"
	"time"
)

type inventoryAction struct {
	ItemID int64
	Delta  float64
	Note   string
}

func (s *Server) partsActions(ctx context.Context, u *store.User, v *store.Vehicle, what string, before, after []store.Part) core.CommitHook {
	if u == nil || !s.settingsFor(ctx, u.ID).InventoryTake {
		return nil
	}
	d := partDeltas(before, after)
	return func(tx *sql.Tx) error {
		ids := []int64{}
		for id := range d {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if _, err := core.QueueAction(ctx, tx, u.ID, "inventory.adjust", inventoryAction{ItemID: id, Delta: d[id], Note: "Pocket Garage: " + v.Title() + " · " + what}); err != nil {
				return err
			}
		}
		return nil
	}
}
func (s *Server) actionsLoop(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		if ctx.Err() != nil || s.store.DB.PingContext(ctx) != nil {
			return
		}
		s.drainActions(ctx, 0)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (s *Server) drainActions(ctx context.Context, uid int64) string {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	for n := 0; n < 100; n++ {
		actions, err := core.PendingActions(ctx, s.store.DB, uid)
		if err != nil {
			return err.Error()
		}
		if len(actions) == 0 {
			break
		}
		for _, a := range actions {
			err = s.dispatchAction(ctx, a)
			if err == nil {
				err = core.FinishAction(ctx, s.store.DB, a)
			}
			if err != nil {
				_ = core.RetryAction(ctx, s.store.DB, a, err)
			}
		}
	}
	var n int
	if err := s.store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pocket_actions WHERE done_at=0 AND (?=0 OR user_id=?)`, uid, uid).Scan(&n); err != nil {
		return err.Error()
	}
	if n > 0 {
		return fmt.Sprintf("%d inventory update(s) saved and waiting to retry", n)
	}
	return ""
}
func (s *Server) dispatchAction(ctx context.Context, a core.Action) error {
	u, err := s.store.GetUser(ctx, a.UserID)
	if err != nil {
		return err
	}
	conn, err := s.inventoryFor(ctx, u)
	if err != nil {
		return err
	}
	var in inventoryAction
	if err := json.Unmarshal(a.Payload, &in); err != nil {
		return err
	}
	if a.Kind != "inventory.adjust" {
		return fmt.Errorf("unknown action %s", a.Kind)
	}
	_, err = conn.client.Adjust(ctx, in.ItemID, in.Delta, in.Note, a.ID)
	return err
}
