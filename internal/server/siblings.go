package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- Pocket Travel and Pocket Inventory ----
//
// Like Pocket Money: a URL + token typed into Settings wins (the way it has
// always worked, and the only way without Pocket Core); otherwise Garage acts
// for the person through a Pocket Core app link, so every person gets their
// own trips and their own shelf with nothing to paste.

var (
	travelScopes    = []string{"trips:read"}
	inventoryScopes = []string{"items:read", "items:adjust"}
)

// siblingConn is how Garage reaches one person's Travel or Inventory.
type siblingConn struct {
	client *pocket.Client
	via    string // token | core
	public string // what a browser opens
}

func linkReason(app, reason string) string {
	switch reason {
	case "off_person":
		return "you switched the Garage → " + app + " link off in Pocket Core (Settings → Apps acting for me)"
	case "off_admin":
		return "the Garage → " + app + " link is switched off in Pocket Core (Links)"
	case "target_missing":
		return app + " is not registered with Pocket Core"
	case "not_offered":
		return "this " + app + " version does not accept app links yet"
	case "not_declared":
		return "Pocket Core has not seen this Garage version yet; it re-registers within a minute of starting"
	case "not_in_audience":
		return "you do not have access to " + app + " in Pocket Core"
	case "unknown_user":
		return "your email is not a person in Pocket Core"
	}
	return ""
}

func (s *Server) siblingFor(ctx context.Context, u *store.User, slug, app string, scopes []string, url, token, public string) (*siblingConn, error) {
	if u == nil {
		return nil, errors.New("nobody to act for")
	}
	if token != "" {
		base := url
		if base == "" {
			base = s.resolveSibling(ctx, slug)
		}
		if base == "" {
			return nil, fmt.Errorf("set the %s URL in Settings → Integrations", app)
		}
		return &siblingConn{client: pocket.New(base, token, 15*time.Second), via: "token", public: firstNonEmpty(public, base)}, nil
	}
	if !s.core.Configured() {
		return nil, fmt.Errorf("%s is not connected: no Pocket Core on this box and no token in Settings", app)
	}
	tok, err := s.core.ActAs(ctx, u.Email, slug, scopes...)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			if t := linkReason(app, le.Reason); t != "" {
				return nil, errors.New(t)
			}
		}
		return nil, err
	}
	return &siblingConn{client: pocket.New(tok.URL, tok.AccessToken, 15*time.Second), via: "core", public: firstNonEmpty(public, firstNonEmpty(tok.PublicURL, tok.URL))}, nil
}

func (s *Server) travelFor(ctx context.Context, u *store.User) (*siblingConn, error) {
	st := s.settingsFor(ctx, u.ID)
	return s.siblingFor(ctx, u, "travel", "Pocket Travel", travelScopes, st.TravelURL, st.TravelToken, st.TravelPublicURL)
}

func (s *Server) inventoryFor(ctx context.Context, u *store.User) (*siblingConn, error) {
	st := s.settingsFor(ctx, u.ID)
	return s.siblingFor(ctx, u, "inventory", "Pocket Inventory", inventoryScopes, st.InventoryURL, st.InventoryToken, "")
}

// siblingVia says how a person is connected to Travel or Inventory, without
// issuing anything: "token", "core" or "" (not connected).
func (s *Server) siblingVia(ctx context.Context, u *store.User, slug, token string, scopes []string) string {
	if token != "" {
		return "token"
	}
	if u == nil || !s.core.Configured() {
		return ""
	}
	if on, _ := s.core.LinkOn(ctx, u.Email, slug, scopes...); on {
		return "core"
	}
	return ""
}

// ---- parts off the shelf ----

// partDeltas: how much of each Inventory item changes when a record's
// parts go from before to after (negative = taken off the shelf).
func partDeltas(before, after []store.Part) map[int64]float64 {
	qty := func(p store.Part) float64 {
		if p.Qty <= 0 {
			return 1
		}
		return p.Qty
	}
	d := map[int64]float64{}
	for _, p := range before {
		if p.InventoryID > 0 {
			d[p.InventoryID] += qty(p)
		}
	}
	for _, p := range after {
		if p.InventoryID > 0 {
			d[p.InventoryID] -= qty(p)
		}
	}
	for id, v := range d {
		if math.Abs(v) < 1e-9 {
			delete(d, id)
		}
	}
	return d
}

// takeParts moves stock in the person's Pocket Inventory for parts picked
// from it: a new record takes them off the shelf, an edit takes or returns
// the difference, a deleted record puts them back. Returns a message when
// something could not be moved (the record itself is saved either way).
func (s *Server) takeParts(ctx context.Context, u *store.User, v *store.Vehicle, what string, before, after []store.Part) string {
	if u == nil || !s.settingsFor(ctx, u.ID).InventoryTake {
		return ""
	}
	d := partDeltas(before, after)
	if len(d) == 0 {
		return ""
	}
	conn, err := s.inventoryFor(ctx, u)
	if err != nil {
		return "Pocket Inventory not updated: " + err.Error()
	}
	note := "Pocket Garage: " + v.Title() + " · " + what
	var bad []string
	for id, delta := range d {
		if _, err := conn.client.Adjust(ctx, id, delta, note); err != nil {
			bad = append(bad, err.Error())
		}
	}
	if len(bad) > 0 {
		return "Pocket Inventory not updated: " + strings.Join(bad, "; ")
	}
	s.store.LogActivity(ctx, v.ID, u.ID, "Pocket Inventory updated", what)
	return ""
}
