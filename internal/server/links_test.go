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

// linkCore is a fake Pocket Core that answers introspection of pcx_ link
// tokens as delegated (pcx_<who>_<scopes>) and records the manifest.
func linkCore(t *testing.T) (*httptest.Server, func() core.Manifest) {
	t.Helper()
	var mu sync.Mutex
	var manifest core.Manifest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&manifest)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			people := map[string]string{"member": "member@example.com", "owner": "owner@example.com"}
			parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
			email, ok := people[parts[0]]
			if !strings.HasPrefix(in.Token, "pcx_") || !ok || len(parts) != 2 {
				_, _ = w.Write([]byte(`{"active":false}`))
				return
			}
			scope := map[string]string{"day": "vehicles:read", "other": "notes:read"}[parts[1]]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": true, "kind": "delegated", "scope": scope, "audience": "garage",
				"user": map[string]any{"id": 9, "email": email, "name": parts[0], "role": "member"},
				"app":  map[string]any{"id": 4, "slug": "day", "name": "Pocket Day"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() core.Manifest { mu.Lock(); defer mu.Unlock(); return manifest }
}

// Pocket Day reads a person's vehicles, fill-ups and service records with a
// link token: exactly what that person sees, nothing else in Garage.
func TestCoreLinkVehiclesRead(t *testing.T) {
	coreSrv, manifest := linkCore(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(coreSrv.URL, core.Options{Token: "pca_garage"}) })
	g := p.seedOwner()
	ov := fmt.Sprintf("/api/v1/vehicles/%d", g.vehicle)

	// the member's own car
	code, body := p.do(p.bTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Fiat", "model": "Panda", "plate": "MEMBER-1"})
	if code != http.StatusCreated {
		t.Fatalf("member vehicle: %d %s", code, body)
	}
	mv := fmt.Sprintf("/api/v1/vehicles/%d", p.id(body, "id"))
	if code, body := p.do(p.bTok, "POST", mv+"/fuel", map[string]any{"date": "2026-10-05", "odometer": 5000, "volume": 30, "cost": 7000, "station": "Member station"}); code != http.StatusCreated {
		t.Fatalf("member fuel: %d %s", code, body)
	}
	if code, body := p.do(p.bTok, "POST", mv+"/records", map[string]any{"kind": "service", "description": "Member oil change", "date": "2026-10-05"}); code != http.StatusCreated {
		t.Fatalf("member record: %d %s", code, body)
	}

	day := "pcx_member_day"
	if code, body := p.do(day, "GET", "/api/v1/auth/me", nil); code != 200 || !strings.Contains(body, "member@example.com") {
		t.Fatalf("auth/me through the link: %d %s", code, body)
	}
	for _, path := range []string{"/api/v1/vehicles?bare=1", "/api/v1/vehicles"} {
		if code, body := p.do(day, "GET", path, nil); code != 200 || !strings.Contains(body, "Panda") || strings.Contains(body, "Fabia") {
			t.Fatalf("GET %s through the link: %d %s", path, code, body)
		}
	}
	if code, body := p.do(day, "GET", mv, nil); code != 200 || !strings.Contains(body, "MEMBER-1") {
		t.Fatalf("vehicle through the link: %d %s", code, body)
	}
	if code, body := p.do(day, "GET", mv+"/fuel", nil); code != 200 || !strings.Contains(body, "Member station") {
		t.Fatalf("fuel through the link: %d %s", code, body)
	}
	if code, body := p.do(day, "GET", mv+"/records?from=2026-10-05&to=2026-10-05", nil); code != 200 || !strings.Contains(body, "Member oil change") {
		t.Fatalf("records through the link: %d %s", code, body)
	}

	// the owner's car is invisible until it is shared with the member
	for _, path := range []string{ov, ov + "/fuel", ov + "/records"} {
		if code, _ := p.do(day, "GET", path, nil); code != http.StatusNotFound {
			t.Fatalf("GET %s (someone else's car) through the link: %d, want 404", path, code)
		}
	}
	if code, body := p.do(p.aTok, "PUT", ov+"/members", map[string]any{"members": []any{map[string]any{"user_id": p.b.ID, "role": "editor"}}}); code != 200 {
		t.Fatalf("share: %d %s", code, body)
	}
	if code, body := p.do(day, "GET", "/api/v1/vehicles?bare=1", nil); code != 200 || !strings.Contains(body, "Fabia") {
		t.Fatalf("shared car missing through the link: %d %s", code, body)
	}
	if code, body := p.do(day, "GET", ov+"/records", nil); code != 200 || !strings.Contains(body, "Owner secret repair") {
		t.Fatalf("shared car's records through the link: %d %s", code, body)
	}

	// read-only: no writes even on a car the person may edit, nothing outside the scope
	for _, c := range []struct{ m, p string }{
		{"POST", "/api/v1/vehicles"}, {"PUT", mv}, {"DELETE", mv}, {"POST", mv + "/fuel"}, {"POST", mv + "/records"},
		{"PUT", fmt.Sprintf("%s/records/%d", ov, g.record)}, {"GET", fmt.Sprintf("%s/records/%d", ov, g.record)},
		{"GET", fmt.Sprintf("%s/fuel/%d", ov, g.fuel)}, {"POST", fmt.Sprintf("%s/fuel/%d/money", ov, g.fuel)},
		{"GET", ov + "/notes"}, {"GET", ov + "/documents"}, {"GET", ov + "/overview"}, {"GET", ov + "/odometer"},
		{"GET", ov + "/reminders"}, {"GET", ov + "/export/vehicle.json"}, {"PUT", ov + "/members"},
		{"GET", "/api/v1/dashboard"}, {"GET", "/api/v1/home"}, {"GET", "/api/v1/reminders"}, {"GET", "/api/v1/money"},
		{"GET", "/api/v1/inventory/search"}, {"PUT", "/api/v1/auth/me"}, {"POST", "/api/v1/auth/logout"},
	} {
		if code, _ := p.do(day, c.m, c.p, map[string]any{}); code != http.StatusForbidden {
			t.Errorf("%s %s with a vehicles:read link: %d, want 403", c.m, c.p, code)
		}
	}

	// a link without vehicles:read only reaches auth/me
	other := "pcx_member_other"
	if code, _ := p.do(other, "GET", "/api/v1/auth/me", nil); code != 200 {
		t.Errorf("auth/me with another scope: %d", code)
	}
	for _, path := range []string{"/api/v1/vehicles", mv, mv + "/fuel", mv + "/records"} {
		if code, _ := p.do(other, "GET", path, nil); code != http.StatusForbidden {
			t.Errorf("GET %s without vehicles:read: %d, want 403", path, code)
		}
	}
	if code, _ := p.do("pcx_nobody_day", "GET", "/api/v1/vehicles", nil); code != http.StatusUnauthorized {
		t.Errorf("inactive link token: %d, want 401", code)
	}

	// the LubeLogger-compatible API and the calendar feed never take a link token
	for _, path := range []string{"/api/vehicles", fmt.Sprintf("/api/vehicle/gasrecords?vehicleId=%d", g.vehicle), "/api/vehicle/info"} {
		if code, body := p.do(day, "GET", path, nil); code != http.StatusUnauthorized {
			t.Errorf("compat GET %s with a link token: %d %s", path, code, body)
		}
	}
	if code, _ := p.do("", "GET", fmt.Sprintf("/cal/%d.ics?token=%s", g.vehicle, day), nil); code != http.StatusUnauthorized {
		t.Errorf("calendar feed with a link token: %d", code)
	}

	// the manifest offers the scope
	if err := p.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m := manifest(); len(m.Offers) != 1 || m.Offers[0].Scope != "vehicles:read" {
		t.Errorf("manifest offers: %+v", m.Offers)
	}
}

// A link acting for the box owner (admin) still never reaches admin or
// settings routes, nor members' cars.
func TestCoreLinkAdminRefused(t *testing.T) {
	coreSrv, _ := linkCore(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(coreSrv.URL, core.Options{Token: "pca_garage"}) })
	code, body := p.do(p.bTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Fiat", "model": "Panda"})
	if code != http.StatusCreated {
		t.Fatalf("member vehicle: %d %s", code, body)
	}
	mv := fmt.Sprintf("/api/v1/vehicles/%d", p.id(body, "id"))
	owner := "pcx_owner_day"
	if code, body := p.do(owner, "GET", "/api/v1/vehicles", nil); code != 200 || strings.Contains(body, "Panda") {
		t.Fatalf("owner's link lists the member's car: %d %s", code, body)
	}
	if code, _ := p.do(owner, "GET", mv, nil); code != http.StatusNotFound {
		t.Errorf("owner's link reads the member's car: %d", code)
	}
	for _, c := range []struct{ m, p string }{
		{"GET", "/api/v1/users"}, {"POST", "/api/v1/users"}, {"GET", "/api/v1/settings"}, {"PUT", "/api/v1/settings"},
		{"GET", "/api/v1/settings/system"}, {"GET", "/api/v1/tokens"}, {"POST", "/api/v1/tokens"}, {"GET", "/api/v1/backup"},
		{"POST", "/api/v1/import/vehicle"}, {"POST", "/api/v1/import/lubelogger"}, {"POST", "/api/v1/integrations/test"},
		{"POST", "/api/v1/notify/run"},
	} {
		if code, _ := p.do(owner, c.m, c.p, map[string]any{}); code != http.StatusForbidden {
			t.Errorf("%s %s with the owner's link: %d, want 403", c.m, c.p, code)
		}
	}
}
