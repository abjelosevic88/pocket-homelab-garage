package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/dawarich"
)

// movementRig: a fake Pocket Movement (Dawarich-compatible timeline and
// points, behind pcx_ link tokens) and a fake Pocket Core whose
// movement:read / movement:points links can be switched per scope.
type movementRig struct {
	mu       sync.Mutex
	movement *httptest.Server
	core     *httptest.Server
	state    map[string]string // scope → on | off_admin | …
	calls    []string          // paths Movement served
	dawarich *httptest.Server
	dwCalls  int
}

func newMovementRig(t *testing.T) *movementRig {
	t.Helper()
	g := &movementRig{state: map[string]string{"movement:read": "on", "movement:points": "on"}}
	g.movement = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("api_key")
		if !strings.HasPrefix(key, "pcx_movement_") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		g.mu.Lock()
		g.calls = append(g.calls, r.URL.Path)
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/timeline" && strings.HasSuffix(key, ":read"):
			_, _ = w.Write([]byte(`{"days":[{"date":"2026-10-18","summary":{"total_distance":31,"distance_unit":"km","mode_distances":{"driving":30,"walking":1}}}]}`))
		case r.URL.Path == "/api/v1/points" && strings.HasSuffix(key, ":points"):
			// 10 fixes a minute apart, ~1 km each (60 km/h): 9 km of driving
			base := time.Date(2026, 10, 18, 9, 0, 0, 0, time.Local).Unix()
			var pts []map[string]any
			for i := 0; i < 10; i++ {
				pts = append(pts, map[string]any{"latitude": 44.0 + float64(i)*0.009, "longitude": 17.0, "timestamp": base + int64(i)*60})
			}
			w.Header().Set("X-Total-Pages", "1")
			_ = json.NewEncoder(w).Encode(pts)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(g.movement.Close)
	g.dawarich = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.dwCalls++
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"days":[{"date":"2026-10-18","summary":{"total_distance":44,"distance_unit":"km","mode_distances":{"driving":42}}}]}`))
	}))
	t.Cleanup(g.dawarich.Close)
	g.core = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		g.mu.Lock()
		defer g.mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/apps/register":
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/links/status":
			if r.URL.Query().Get("app") != "movement" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			out := []map[string]any{}
			for sc, st := range g.state {
				out = append(out, map[string]any{"app": "movement", "scope": sc, "state": st, "on": st == "on"})
			}
			_ = json.NewEncoder(w).Encode(out)
		case "/api/v1/links/token":
			var in struct {
				User   string   `json:"user"`
				App    string   `json:"app"`
				Scopes []string `json:"scopes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.App != "movement" || len(in.Scopes) != 1 {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"no","reason":"not_declared"}`))
				return
			}
			if st := g.state[in.Scopes[0]]; st != "on" {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "link off", "reason": st})
				return
			}
			suffix := strings.TrimPrefix(in.Scopes[0], "movement:")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_movement_" + in.User + ":" + suffix, "expires_in": 300, "url": g.movement.URL})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.core.Close)
	return g
}

func (g *movementRig) set(read, points string) {
	g.mu.Lock()
	g.state["movement:read"], g.state["movement:points"] = read, points
	g.calls, g.dwCalls = nil, 0
	g.mu.Unlock()
}

func (g *movementRig) seen() ([]string, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.calls...), g.dwCalls
}

func TestMovementLocationSource(t *testing.T) {
	g := newMovementRig(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(g.core.URL, core.Options{DB: s.store.DB, Token: "pca_garage"}) })
	ctx := context.Background()
	if err := p.s.registerWithCore(ctx); err != nil {
		t.Fatal(err)
	}
	// the owner has Dawarich from the environment (direct, a key saved)
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"dawarich_url": g.dawarich.URL}); code != 200 {
		t.Fatalf("save url %d %s", code, body)
	}
	car := dawarichVehicle(t, p, p.aTok)

	// Movement reachable: it is the default, the saved Dawarich key does not override it.
	st := settingsOf(t, p, p.aTok)
	if st["location_effective"] != "movement" || st["movement_via"] != "core" || st["location_source"] != "" || st["dawarich_mode"] != "direct" {
		t.Fatalf("movement default %v", st)
	}
	if code, body := p.do(p.aTok, "POST", car+"/dawarich/sync", nil); code != 200 || !strings.Contains(body, `"estimate_km":30`) || !strings.Contains(body, `"source":"movement"`) {
		t.Fatalf("movement sync %d %s", code, body)
	}
	if calls, dw := g.seen(); dw != 0 || len(calls) == 0 || calls[0] != "/api/v1/timeline" {
		t.Fatalf("movement: movement saw %v, dawarich %d calls", calls, dw)
	}
	if _, body := p.do(p.aTok, "POST", "/api/v1/integrations/test", map[string]any{"which": "movement"}); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "30 km driven") {
		t.Fatalf("movement test %s", body)
	}

	// The admin switched the link off in Core: Dawarich meanwhile, the
	// days re-read from it, and the exact reason shown.
	g.set("off_admin", "off_admin")
	st = settingsOf(t, p, p.aTok)
	if st["location_effective"] != "dawarich" || st["movement_via"] != "" || !strings.Contains(st["movement_detail"].(string), "the Garage → Pocket Movement link is switched off in Pocket Core (Links)") {
		t.Fatalf("off_admin %v", st)
	}
	if code, body := p.do(p.aTok, "POST", car+"/dawarich/sync", nil); code != 200 || !strings.Contains(body, `"estimate_km":42`) || !strings.Contains(body, `"source":"dawarich"`) {
		t.Fatalf("fallback sync %d %s", code, body)
	}
	if calls, dw := g.seen(); dw == 0 || len(calls) != 0 {
		t.Fatalf("fallback: movement saw %v, dawarich %d calls", calls, dw)
	}

	// Only the raw points allowed: driving worked out from them.
	g.set("off_admin", "on")
	if code, body := p.do(p.aTok, "POST", car+"/dawarich/sync", nil); code != 200 || !strings.Contains(body, `"source":"movement"`) || !strings.Contains(body, `"estimate_km":9`) {
		t.Fatalf("points sync %d %s", code, body)
	}
	if st = settingsOf(t, p, p.aTok); st["movement_points_only"] != true || st["location_effective"] != "movement" {
		t.Fatalf("points only %v", st)
	}

	// Picking Dawarich explicitly wins over a reachable Movement.
	g.set("on", "on")
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"location_source": "dawarich"})
	if st = settingsOf(t, p, p.aTok); st["location_effective"] != "dawarich" || st["location_source"] != "dawarich" {
		t.Fatalf("picked dawarich %v", st)
	}
	p.s.autoSync(ctx)
	if calls, dw := g.seen(); dw == 0 || len(calls) != 0 {
		t.Fatalf("picked dawarich: movement saw %v, dawarich %d calls", calls, dw)
	}
	// The LubeLogger-compatible API is untouched: the odometer still reads.
	if code, _ := p.do(p.aTok, "GET", "/api/vehicles", nil); code != 200 {
		t.Fatalf("compat vehicles %d", code)
	}
}

// Without Pocket Core: Dawarich as before, Movement unavailable with why.
func TestMovementWithoutCore(t *testing.T) {
	p := newTwoPeople(t)
	st := settingsOf(t, p, p.aTok)
	if st["location_effective"] != "dawarich" || st["movement_via"] != "" || st["core_connected"] != false || st["movement_detail"] != "there is no Pocket Core on this box" {
		t.Fatalf("without core %v", st)
	}
}

func TestPointsDriving(t *testing.T) {
	base := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC).Unix()
	pts := []dawarich.Point{
		{Lat: 44, Lng: 17, TS: base},
		{Lat: 44.009, Lng: 17, TS: base + 60},     // ~1 km in a minute: driving
		{Lat: 44.0091, Lng: 17, TS: base + 120},   // ~11 m: standing
		{Lat: 44.5, Lng: 17, TS: base + 3600 + 1}, // a gap: left out
	}
	d := pointsDriving(pts, time.UTC)
	if len(d) != 1 || d[0].km < 0.9 || d[0].km > 1.1 {
		t.Fatalf("points driving %+v", d)
	}
}
