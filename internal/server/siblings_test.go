package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

func TestTravelAndInventoryThroughCore(t *testing.T) {
	var mu sync.Mutex
	travelAuth := []string{}
	adjusts := []string{}
	travel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		travelAuth = append(travelAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok","version":"9.9"}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"id":1}`))
		case "/api/v1/trips":
			if r.URL.Query().Get("archived") == "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":4,"name":"Rome","destination":"Rome","start_date":"2026-06-01","end_date":"2026-06-05"}]`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(travel.Close)
	inventory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer pcx_inventory_owner@example.com" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/items":
			_, _ = w.Write([]byte(`[{"id":5,"name":"Oil filter","quantity":3,"unit":"pcs"},{"id":6,"name":"5W-30 oil","quantity":10,"unit":"l"}]`))
		case strings.HasSuffix(r.URL.Path, "/adjust"):
			var in struct {
				Delta float64 `json:"delta"`
				Note  string  `json:"note"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			mu.Lock()
			adjusts = append(adjusts, fmt.Sprintf("%s %+g", strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/items/"), "/adjust"), in.Delta))
			mu.Unlock()
			if !strings.HasPrefix(in.Note, "Pocket Garage: ") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(inventory.Close)
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/links/status":
			app := r.URL.Query().Get("app")
			scopes := map[string][]string{"travel": travelScopes, "inventory": inventoryScopes}[app]
			out := []map[string]any{}
			for _, sc := range scopes {
				out = append(out, map[string]any{"app": app, "scope": sc, "state": "on", "on": true})
			}
			_ = json.NewEncoder(w).Encode(out)
		case "/api/v1/links/token":
			var in struct {
				User string `json:"user"`
				App  string `json:"app"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			url := map[string]string{"travel": travel.URL, "inventory": inventory.URL}[in.App]
			if url == "" || in.User != "owner@example.com" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"no","reason":"not_in_audience"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_" + in.App + "_" + in.User, "expires_in": 300, "url": url, "public_url": "https://" + in.App + ".example"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(coreSrv.Close)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(coreSrv.URL, core.Options{DB: s.store.DB, Token: "pca_garage"}) })

	var st map[string]any
	_, body := p.do(p.aTok, "GET", "/api/v1/settings", nil)
	_ = json.Unmarshal([]byte(body), &st)
	if st["travel_via"] != "core" || st["inventory_via"] != "core" || st["inventory_configured"] != true || st["inventory_take"] != true {
		t.Fatalf("settings %s", body)
	}
	_, body = p.do(p.aTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Skoda", "model": "Fabia", "currency": "BAM"})
	var veh struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &veh)
	base := fmt.Sprintf("/api/v1/vehicles/%d", veh.ID)
	p.do(p.aTok, "POST", base+"/fuel", map[string]any{"date": "2026-06-02", "odometer": 1000, "volume": 30, "cost": 7000, "money": false})

	// Trips come from the owner's Pocket Travel through Core.
	_, body = p.do(p.aTok, "GET", base+"/trips", nil)
	if !strings.Contains(body, `"name":"Rome"`) || !strings.Contains(body, `"fills":1`) || !strings.Contains(body, "https://travel.example/trips/4") {
		t.Fatalf("trips %s", body)
	}
	// Parts search.
	_, body = p.do(p.aTok, "GET", "/api/v1/inventory/search?q=oil", nil)
	if !strings.Contains(body, "Oil filter") || !strings.Contains(body, "https://inventory.example") {
		t.Fatalf("inventory search %s", body)
	}

	// Parts picked from Inventory come off the shelf; edits take or return the difference; delete puts them back.
	code, body := p.do(p.aTok, "POST", base+"/records", map[string]any{"kind": "service", "description": "Oil change", "cost": 9000, "money": false,
		"parts": []map[string]any{{"name": "Oil filter", "qty": 1, "inventory_id": 5}, {"name": "5W-30 oil", "qty": 4, "inventory_id": 6}, {"name": "Rag", "qty": 1}}})
	if code != 201 || strings.Contains(body, "inventory_error") {
		t.Fatalf("record %d %s", code, body)
	}
	var rec struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &rec)
	p.do(p.aTok, "PUT", fmt.Sprintf("%s/records/%d", base, rec.ID), map[string]any{"parts": []map[string]any{{"name": "Oil filter", "qty": 1, "inventory_id": 5}, {"name": "5W-30 oil", "qty": 5, "inventory_id": 6}}})
	p.do(p.aTok, "PUT", fmt.Sprintf("%s/records/%d", base, rec.ID), map[string]any{"notes": "no parts change"})
	p.do(p.aTok, "DELETE", fmt.Sprintf("%s/records/%d", base, rec.ID), nil)
	mu.Lock()
	got := strings.Join(sortedCopy(adjusts), ", ")
	mu.Unlock()
	if got != "5 +1, 5 -1, 6 +5, 6 -1, 6 -4" {
		t.Fatalf("stock moves %q", got)
	}

	// Switched off: nothing moves.
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"inventory_take": false})
	p.do(p.aTok, "POST", base+"/records", map[string]any{"kind": "service", "description": "Another", "parts": []map[string]any{{"name": "Oil filter", "qty": 1, "inventory_id": 5}}})
	mu.Lock()
	if len(adjusts) != 5 {
		t.Fatalf("moved although switched off: %v", adjusts)
	}
	mu.Unlock()

	// A token typed into Settings wins over the Core link.
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"travel_url": travel.URL, "travel_token": "manual-travel-token"})
	_, body = p.do(p.aTok, "GET", "/api/v1/settings", nil)
	_ = json.Unmarshal([]byte(body), &st)
	if st["travel_via"] != "direct" || st["travel_mode"] != "direct" {
		t.Fatalf("manual token should win: %s", body)
	}
	p.do(p.aTok, "GET", base+"/trips", nil)
	mu.Lock()
	last := travelAuth[len(travelAuth)-1]
	mu.Unlock()
	if last != "Bearer manual-travel-token" {
		t.Fatalf("travel called with %q", last)
	}

	// Choosing Pocket Core ignores the saved token (it stays, for switching back).
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"travel_mode": "core"})
	p.do(p.aTok, "GET", base+"/trips", nil)
	mu.Lock()
	last = travelAuth[len(travelAuth)-1]
	mu.Unlock()
	if last != "Bearer pcx_travel_owner@example.com" {
		t.Fatalf("core mode called travel with %q", last)
	}
	// Off: no connection at all, and Settings says so.
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"travel_mode": "off"})
	if _, body := p.do(p.aTok, "GET", base+"/trips", nil); strings.Contains(body, "Rome") {
		t.Fatalf("off still reads trips: %s", body)
	}
	_, body = p.do(p.aTok, "GET", "/api/v1/settings", nil)
	_ = json.Unmarshal([]byte(body), &st)
	if st["travel_mode"] != "off" || st["travel_via"] != "" || st["travel_detail"] != "switched off" || st["travel_token"] == "" {
		t.Fatalf("off settings %s", body)
	}
	// Direct without a token says what is missing.
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"inventory_mode": "direct"})
	if _, body := p.do(p.aTok, "GET", "/api/v1/inventory/search?q=oil", nil); !strings.Contains(body, "has no token") {
		t.Fatalf("direct without token %s", body)
	}

	// The member has no link (Core says not in audience): no trips, no error page.
	if code, body := p.do(p.bTok, "GET", "/api/v1/inventory/search?q=oil", nil); code != 200 || !strings.Contains(body, "do not have access to Pocket Inventory") {
		t.Fatalf("member search %d %s", code, body)
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
