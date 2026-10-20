package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// serviceRig: a fake Dawarich and a fake Pocket Core that hands out Dawarich
// credentials (the box's key for the owner, the member's own key or
// no_account for the member).
type serviceRig struct {
	mu          sync.Mutex
	dawarich    *httptest.Server
	core        *httptest.Server
	keys        []string // api_key values Dawarich saw
	creds       []string // users Core handed a credential for
	memberOwn   bool     // the member has a Dawarich key of their own in Core
	registered  core.Manifest
	statusCalls int
}

func newServiceRig(t *testing.T) *serviceRig {
	t.Helper()
	g := &serviceRig{}
	g.dawarich = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("api_key")
		g.mu.Lock()
		g.keys = append(g.keys, key)
		g.mu.Unlock()
		if key == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/v1/timeline":
			_, _ = w.Write([]byte(`{"days":[{"date":"2026-10-18","summary":{"total_distance":44,"distance_unit":"km","mode_distances":{"driving":42}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.dawarich.Close)
	g.core = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		g.mu.Lock()
		defer g.mu.Unlock()
		state := func(user string) (string, string) { // state, source
			switch {
			case user == "owner@example.com":
				return "on", "box"
			case user == "member@example.com" && g.memberOwn:
				return "on", "personal"
			}
			return "no_account", ""
		}
		switch r.URL.Path {
		case "/api/v1/apps/register":
			_ = json.NewDecoder(r.Body).Decode(&g.registered)
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/links/status":
			if r.URL.Query().Get("app") != "service:dawarich" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			g.statusCalls++
			st, _ := state(r.URL.Query().Get("user"))
			_ = json.NewEncoder(w).Encode([]map[string]any{{"app": "service:dawarich", "scope": "account", "state": st, "on": st == "on"}})
		case "/api/v1/services/credential":
			if r.Header.Get("Authorization") != "Bearer pca_garage" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var in struct{ User, Type string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			g.creds = append(g.creds, in.User)
			st, source := state(in.User)
			if in.Type != "dawarich" || st != "on" {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "dawarich: no account", "reason": st})
				return
			}
			key := map[string]string{"box": "box-dawarich-key", "personal": "member-dawarich-key"}[source]
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "dawarich", "name": "Dawarich", "url": g.dawarich.URL, "public_url": "https://dawarich.example",
				"key": key, "fields": map[string]string{"API_KEY": key}, "source": source})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.core.Close)
	return g
}

func (g *serviceRig) reset() {
	g.mu.Lock()
	g.keys, g.creds, g.statusCalls = nil, nil, 0
	g.mu.Unlock()
}

func (g *serviceRig) seen() (keys, creds []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.keys...), append([]string(nil), g.creds...)
}

func settingsOf(t *testing.T, p *twoPeople, tok string) map[string]any {
	t.Helper()
	code, body := p.do(tok, "GET", "/api/v1/settings", nil)
	if code != 200 {
		t.Fatalf("settings %d %s", code, body)
	}
	var st map[string]any
	_ = json.Unmarshal([]byte(body), &st)
	return st
}

func dawarichVehicle(t *testing.T, p *twoPeople, tok string) string {
	t.Helper()
	_, body := p.do(tok, "POST", "/api/v1/vehicles", map[string]any{"make": "Skoda", "model": "Fabia", "currency": "BAM"})
	var v struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &v)
	base := fmt.Sprintf("/api/v1/vehicles/%d", v.ID)
	if code, body := p.do(tok, "PUT", base, map[string]any{"dawarich": true}); code != 200 {
		t.Fatalf("dawarich on %d %s", code, body)
	}
	return base
}

func overviewDawarich(t *testing.T, p *twoPeople, tok, car string) bool {
	t.Helper()
	_, body := p.do(tok, "GET", car+"/overview", nil)
	var ov struct {
		Integrations map[string]bool `json:"integrations"`
	}
	if err := json.Unmarshal([]byte(body), &ov); err != nil {
		t.Fatalf("overview %s", body)
	}
	return ov.Integrations["dawarich"]
}

func TestDawarichServiceModes(t *testing.T) {
	g := newServiceRig(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(g.core.URL, core.Options{DB: s.store.DB, Token: "pca_garage"}) })

	// The manifest lists Dawarich under Services.
	if err := p.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(g.registered.Services) != 1 || g.registered.Services[0].Type != "dawarich" || g.registered.Services[0].Reason != "Kilometres driven, estimated from your location history" {
		t.Fatalf("manifest services %+v", g.registered.Services)
	}

	// The owner has a saved key (seeded from the environment): Direct, as before.
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_url": g.dawarich.URL}); code != 200 {
		t.Fatalf("save url %d %s", code, body)
	}
	st := settingsOf(t, p, p.aTok)
	if st["dawarich_mode"] != "direct" || st["dawarich_via"] != "direct" || st["dawarich_configured"] != true {
		t.Fatalf("owner default %v", st)
	}
	car := dawarichVehicle(t, p, p.aTok)
	g.reset()
	if code, body := p.do(p.aTok, "POST", car+"/dawarich/sync", nil); code != 200 || !strings.Contains(body, `"estimate_km":42`) {
		t.Fatalf("direct sync %d %s", code, body)
	}
	keys, creds := g.seen()
	if len(keys) == 0 || keys[0] != "owner-dawarich-key" || len(creds) != 0 {
		t.Fatalf("direct: dawarich saw %v, core handed out %v", keys, creds)
	}

	// Pocket Core: Core's address and key, reused for a minute (the SDK's
	// cache), so the settings screens and two syncs ask Core once.
	g.reset()
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_mode": "core"}); code != 200 {
		t.Fatalf("mode %d %s", code, body)
	}
	st = settingsOf(t, p, p.aTok)
	if st["dawarich_mode"] != "core" || st["dawarich_via"] != "core" || st["dawarich_source"] != "box" {
		t.Fatalf("owner core %v", st)
	}
	for i := 0; i < 2; i++ {
		if code, body := p.do(p.aTok, "POST", car+"/dawarich/sync", nil); code != 200 || !strings.Contains(body, `"estimate_km":42`) {
			t.Fatalf("core sync %d %s", code, body)
		}
	}
	keys, creds = g.seen()
	if len(creds) != 1 || creds[0] != "owner@example.com" {
		t.Fatalf("want one credential for the settings and both syncs (cached), got %v", creds)
	}
	for _, k := range keys {
		if k != "box-dawarich-key" {
			t.Fatalf("core: dawarich saw %v", keys)
		}
	}
	if on := overviewDawarich(t, p, p.aTok, car); !on {
		t.Fatal("overview: dawarich should be on through Core")
	}
	if _, body := p.do(p.aTok, "POST", "/api/v1/integrations/test", map[string]any{"which": "dawarich"}); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "through Pocket Core (the box's key)") {
		t.Fatalf("test %s", body)
	}
	if _, creds = g.seen(); len(creds) != 2 {
		t.Fatalf("Test connection must ask Core afresh, Core handed out %v", creds)
	}

	// Off: no calls at all.
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_mode": "off"})
	st = settingsOf(t, p, p.aTok)
	if st["dawarich_mode"] != "off" || st["dawarich_via"] != "" || st["dawarich_configured"] != false {
		t.Fatalf("owner off %v", st)
	}
	g.reset()
	if code, body := p.do(p.aTok, "POST", car+"/dawarich/sync", nil); code != http.StatusBadGateway || !strings.Contains(body, "switched off") {
		t.Fatalf("off sync %d %s", code, body)
	}
	p.s.autoSync(context.Background())
	if on := overviewDawarich(t, p, p.aTok, car); on {
		t.Fatal("overview: dawarich should be off")
	}
	if keys, creds := g.seen(); len(keys) != 0 || len(creds) != 0 || g.statusCalls != 0 {
		t.Fatalf("off: dawarich saw %v, core %v, status %d", keys, creds, g.statusCalls)
	}

	// The member has no key saved: Pocket Core by default, and Core has no
	// Dawarich account of theirs.
	st = settingsOf(t, p, p.bTok)
	if st["dawarich_mode"] != "core" || st["dawarich_via"] != "" || !strings.Contains(st["dawarich_detail"].(string), "you have no Dawarich account of your own yet; ask the admin or let a connector create one") {
		t.Fatalf("member no_account %v", st)
	}
	mine := dawarichVehicle(t, p, p.bTok)
	g.reset()
	if code, body := p.do(p.bTok, "POST", mine+"/dawarich/sync", nil); code != http.StatusBadGateway || !strings.Contains(body, "no Dawarich account of your own") {
		t.Fatalf("member sync %d %s", code, body)
	}
	p.s.autoSync(context.Background())
	if keys, _ := g.seen(); len(keys) != 0 {
		t.Fatalf("no_account: dawarich saw %v", keys)
	}

	// Their own key in Core: connected, with their key.
	g.mu.Lock()
	g.memberOwn = true
	g.mu.Unlock()
	g.reset()
	st = settingsOf(t, p, p.bTok)
	if st["dawarich_via"] != "core" || st["dawarich_source"] != "personal" {
		t.Fatalf("member personal %v", st)
	}
	p.s.autoSync(context.Background()) // the owner is off, the member syncs with their own key (cached from the settings screen)
	keys, creds = g.seen()
	if len(keys) == 0 || len(creds) != 1 || creds[0] != "member@example.com" {
		t.Fatalf("member auto sync: dawarich %v, core %v", keys, creds)
	}
	for _, k := range keys {
		if k != "member-dawarich-key" {
			t.Fatalf("member auto sync used %v", keys)
		}
	}
}

// Without Pocket Core and without a key, Dawarich stays Direct (not connected).
func TestDawarichServiceWithoutCore(t *testing.T) {
	p := newTwoPeople(t)
	st := settingsOf(t, p, p.bTok)
	if st["dawarich_mode"] != "direct" || st["dawarich_via"] != "" || st["dawarich_detail"] != "no API key yet" {
		t.Fatalf("member without core %v", st)
	}
	if code, body := p.do(p.bTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_mode": "core"}); code != 200 || !strings.Contains(body, "there is no Pocket Core on this box") {
		t.Fatalf("core without core %d %s", code, body)
	}
}
