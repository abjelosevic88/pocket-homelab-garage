package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- third-party services (Dawarich) ----
//
// Like the Pocket app connections, chosen per person in Settings:
//
//   - core:   Pocket Core hands out the service's address and the person's
//     own key there (never the box's key),
//     asked for when needed (the SDK reuses it for a minute); nothing to paste.
//   - direct: the URL + key saved in Settings, the way it always worked.
//   - off:    Garage does not call the service.
//
// With nothing chosen: direct while a key is saved (so nothing changes for
// whoever already set one up), else core when Garage is connected to Pocket
// Core, else direct.

// serviceUses is the manifest's Services: what Garage reaches for a person.
var serviceUses = []core.ServiceUse{
	{Type: "dawarich", Reason: "Kilometres driven, estimated from your location history"},
}

// serviceScope is the single scope Core keeps a service link under.
const serviceScope = "account"

// serviceConn is one person's way into a service right now.
type serviceConn struct {
	url    string
	public string // what a browser opens
	key    string
	via    string // core | direct
	source string // core only: personal | box
}

// serviceMode is the mode in effect for a service.
func (s *Server) serviceMode(mode, key string) string {
	fallback := modeDirect
	if s.core.Configured() {
		fallback = modeCore
	}
	return connModeOr(mode, key, fallback)
}

func serviceReason(name, reason string) string {
	switch reason {
	case "target_missing":
		return name + " is not added to Pocket Core (Services)"
	case "not_in_audience":
		return "you do not have access to " + name + " in Pocket Core"
	}
	return linkReason(name, reason)
}

// serviceFor connects to the person's service the way they chose.
func (s *Server) serviceFor(ctx context.Context, u *store.User, typ, name, mode, url, key, public string) (*serviceConn, error) {
	switch s.serviceMode(mode, key) {
	case modeOff:
		return nil, fmt.Errorf("%s is switched off in Settings → Integrations", name)
	case modeDirect:
		if key == "" {
			if url == "" {
				return nil, fmt.Errorf("%s is not configured: add the URL and an API key in Settings → Integrations", name)
			}
			return nil, fmt.Errorf("%s is set to a direct connection but has no API key: add one in Settings → Integrations, or switch to Pocket Core", name)
		}
		if url == "" {
			return nil, fmt.Errorf("set the %s URL in Settings → Integrations", name)
		}
		return &serviceConn{url: url, public: firstNonEmpty(public, url), key: key, via: modeDirect}, nil
	}
	if !s.core.Configured() {
		return nil, fmt.Errorf("%s through Pocket Core: there is no Pocket Core on this box; use a direct connection", name)
	}
	if u == nil {
		return nil, errors.New("nobody to act for")
	}
	cred, err := s.serviceCred(ctx, u.Email, typ)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			if t := serviceReason(name, le.Reason); t != "" {
				return nil, errors.New(t)
			}
		}
		return nil, err
	}
	if cred.URL == "" {
		return nil, fmt.Errorf("Pocket Core did not say where %s lives", name)
	}
	if cred.Key == "" {
		return nil, fmt.Errorf("Pocket Core has no %s API key for you", name)
	}
	return &serviceConn{url: strings.TrimRight(cred.URL, "/"), public: firstNonEmpty(cred.PublicURL, cred.URL), key: cred.Key, via: modeCore, source: cred.Source}, nil
}

// serviceStatus says how a person reaches a service: the mode in effect,
// whether it works ("core" / "direct" / "" for not connected), why not, and
// whose key Core hands out. Without detail it only asks Core for the link's
// state (no key leaves Core); with detail it asks for the credential too, to
// learn whose key it is.
func (s *Server) serviceStatus(ctx context.Context, u *store.User, typ, name, mode, url, key string, detail bool) (eff, via, why, source string) {
	eff = s.serviceMode(mode, key)
	switch eff {
	case modeOff:
		return eff, "", "switched off", ""
	case modeDirect:
		if key == "" {
			return eff, "", "no API key yet", ""
		}
		if url == "" {
			return eff, "", "no URL yet", ""
		}
		return eff, modeDirect, "", ""
	}
	if u == nil || !s.core.Configured() {
		return eff, "", "there is no Pocket Core on this box", ""
	}
	if detail {
		c, err := s.serviceFor(ctx, u, typ, name, mode, url, key, "")
		if err != nil {
			return eff, "", err.Error(), ""
		}
		return eff, modeCore, "", c.source
	}
	on, reason := s.core.LinkOn(ctx, u.Email, "service:"+typ, serviceScope)
	if on {
		return eff, modeCore, "", ""
	}
	if t := serviceReason(name, reason); t != "" {
		return eff, "", t, ""
	}
	return eff, "", reason, ""
}

// freshKey marks a context whose service credential must come from Core
// right now (Test connection), not the SDK's one-minute cache.
type freshKey struct{}

// freshCreds makes serviceFor ask Core afresh.
func freshCreds(ctx context.Context) context.Context { return context.WithValue(ctx, freshKey{}, true) }

// serviceCred is the person's credential: cached by the SDK for a minute (and
// through a Core outage) for normal work, straight from Core for a test.
func (s *Server) serviceCred(ctx context.Context, email, typ string) (*core.ServiceCred, error) {
	if fresh, _ := ctx.Value(freshKey{}).(bool); fresh {
		return s.core.ServiceForFresh(ctx, email, typ)
	}
	return s.core.ServiceFor(ctx, email, typ)
}

// dawarichFor is userID's Dawarich, the way they chose.
func (s *Server) dawarichFor(ctx context.Context, userID int64) (*dawarich.Client, *serviceConn, error) {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("nobody to act for: %w", err)
	}
	st := s.settingsFor(ctx, userID)
	c, err := s.serviceFor(ctx, u, "dawarich", "Dawarich", st.DawarichMode, st.DawarichURL, st.DawarichAPIKey, "")
	if err != nil {
		return nil, nil, err
	}
	return dawarich.New(c.url, c.key, 60*time.Second), c, nil
}

// dawarichVia is serviceStatus's "does it work" part for userID's Dawarich.
func (s *Server) dawarichVia(ctx context.Context, userID int64) string {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return ""
	}
	st := s.settingsFor(ctx, userID)
	_, via, _, _ := s.serviceStatus(ctx, u, "dawarich", "Dawarich", st.DawarichMode, st.DawarichURL, st.DawarichAPIKey, false)
	return via
}

// viaText is how a working connection reads in a test message.
func viaText(c *serviceConn) string {
	if c.via != modeCore {
		return "direct connection"
	}
	if c.source == "box" {
		return "through Pocket Core (the box's key)"
	}
	return "through Pocket Core (your own key)"
}
