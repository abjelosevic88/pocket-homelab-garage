package server

import (
	"context"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- connection modes on Core's map ----
//
// Core can't see a Direct connection (a URL and key pasted here) or one a
// person switched Off, so Garage tells it how each person connects to each
// target (core / direct / off, the mode in effect, the same one Settings
// shows): shortly after registering, right after someone changes a mode or
// disconnects, and every 30 minutes. Report only; nothing in Core decides on
// it, and a failed report is just logged.

// modesEvery is how often the whole map is sent again.
const modesEvery = 30 * time.Minute

// connModes is every person's mode in effect for every target, in one batch.
func (s *Server) connModes(ctx context.Context) []core.ConnMode {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.log.Warn("pocket core: modes not reported, no users", "err", err)
		return nil
	}
	out := make([]core.ConnMode, 0, 4*len(users))
	for _, u := range users {
		st := s.settingsFor(ctx, u.ID)
		out = append(out,
			core.ConnMode{User: u.Email, Target: "money", Mode: connMode(st.MoneyMode, st.MoneyToken)},
			core.ConnMode{User: u.Email, Target: "travel", Mode: connMode(st.TravelMode, st.TravelToken)},
			core.ConnMode{User: u.Email, Target: "inventory", Mode: connMode(st.InventoryMode, st.InventoryToken)},
			core.ConnMode{User: u.Email, Target: "service:dawarich", Mode: s.serviceMode(st.DawarichMode, st.DawarichAPIKey)},
			core.ConnMode{User: u.Email, Target: "movement", Mode: movementMode(st)},
		)
	}
	return out
}

// movementMode: Pocket Movement is only ever reached through Core; it is off
// for whoever picked Dawarich.
func movementMode(st store.Settings) string {
	if locationChoice(st) == srcDawarich {
		return modeOff
	}
	return modeCore
}

// reportModes sends the map to Core, when Garage is connected to one.
func (s *Server) reportModes(ctx context.Context) {
	s.coreMu.Lock()
	ok := s.coreOK
	s.coreMu.Unlock()
	if !s.core.Configured() || !ok {
		return
	}
	modes := s.connModes(ctx)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.core.ReportModes(ctx, modes); err != nil {
		s.log.Warn("pocket core: reporting connection modes failed", "err", err)
		return
	}
	s.log.Debug("pocket core: connection modes reported", "modes", len(modes))
}

// modesChanged asks for a report soon (a person changed a mode or
// disconnected, or Garage just registered); never blocks.
func (s *Server) modesChanged() {
	select {
	case s.modesKick <- struct{}{}:
	default:
	}
}

// modesLoop sends the report when asked and every modesEvery.
func (s *Server) modesLoop(ctx context.Context) {
	t := time.NewTicker(modesEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.modesKick:
		case <-t.C:
		}
		s.reportModes(ctx)
	}
}
