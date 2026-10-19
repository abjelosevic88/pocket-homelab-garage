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

// Connection modes, chosen per person and per app in Settings.
const (
	modeCore   = "core"
	modeDirect = "direct"
	modeOff    = "off"
)

// connMode is the mode in effect: the chosen one, else direct when a token
// was entered (how it always worked), else Pocket Core.
func connMode(mode, token string) string {
	switch mode {
	case modeCore, modeDirect, modeOff:
		return mode
	}
	if token != "" {
		return modeDirect
	}
	return modeCore
}

// siblingConn is how Garage reaches one person's Travel or Inventory.
type siblingConn struct {
	client *pocket.Client
	via    string // direct | core
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

func (s *Server) siblingFor(ctx context.Context, u *store.User, slug, app string, scopes []string, mode, url, token, public string) (*siblingConn, error) {
	if u == nil {
		return nil, errors.New("nobody to act for")
	}
	switch connMode(mode, token) {
	case modeOff:
		return nil, fmt.Errorf("%s is switched off in Settings → Integrations", app)
	case modeDirect:
		if token == "" {
			return nil, fmt.Errorf("%s is set to a direct connection but has no token: add one in Settings → Integrations, or switch to Pocket Core", app)
		}
		base := url
		if base == "" {
			base = s.resolveSibling(ctx, slug)
		}
		if base == "" {
			return nil, fmt.Errorf("set the %s URL in Settings → Integrations", app)
		}
		return &siblingConn{client: pocket.New(base, token, 15*time.Second), via: modeDirect, public: firstNonEmpty(public, base)}, nil
	}
	if !s.core.Configured() {
		return nil, fmt.Errorf("%s through Pocket Core: there is no Pocket Core on this box; use a direct connection", app)
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
	return s.siblingFor(ctx, u, "travel", "Pocket Travel", travelScopes, st.TravelMode, st.TravelURL, st.TravelToken, st.TravelPublicURL)
}

func (s *Server) inventoryFor(ctx context.Context, u *store.User) (*siblingConn, error) {
	st := s.settingsFor(ctx, u.ID)
	return s.siblingFor(ctx, u, "inventory", "Pocket Inventory", inventoryScopes, st.InventoryMode, st.InventoryURL, st.InventoryToken, "")
}

// connStatus says, without issuing anything, how a person reaches a Pocket
// app: the mode in effect, whether it works ("core" / "direct" / "" for
// not connected) and why not.
func (s *Server) connStatus(ctx context.Context, u *store.User, slug, app, mode, token string, scopes []string) (eff, via, detail string) {
	eff = connMode(mode, token)
	switch eff {
	case modeOff:
		return eff, "", "switched off"
	case modeDirect:
		if token == "" {
			return eff, "", "no token yet"
		}
		return eff, modeDirect, ""
	}
	if u == nil || !s.core.Configured() {
		return eff, "", "there is no Pocket Core on this box"
	}
	on, reason := s.core.LinkOn(ctx, u.Email, slug, scopes...)
	if on {
		return eff, modeCore, ""
	}
	if t := linkReason(app, reason); t != "" {
		return eff, "", t
	}
	return eff, "", reason
}

// siblingVia is connStatus's "does it work" part.
func (s *Server) siblingVia(ctx context.Context, u *store.User, slug, mode, token string, scopes []string) string {
	_, via, _ := s.connStatus(ctx, u, slug, slug, mode, token, scopes)
	return via
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
