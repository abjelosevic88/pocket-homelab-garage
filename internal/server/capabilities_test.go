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

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// capCore is a fake Pocket Core that vouches for link tokens
// pcx_<owner|member>_<search|day|widget|capture|read> from Pocket Hub and
// records the manifest Garage registers with.
func capCore(t *testing.T) (*httptest.Server, func() core.Manifest) {
	t.Helper()
	var mu sync.Mutex
	var manifest core.Manifest
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture, "read": "vehicles:read"}
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
			parts := strings.Split(in.Token, "_")
			people := map[string]string{"member": "member@example.com", "owner": "owner@example.com"}
			if len(parts) != 3 || parts[0] != "pcx" || people[parts[1]] == "" || scopes[parts[2]] == "" {
				_, _ = w.Write([]byte(`{"active":false}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": true, "kind": "delegated", "scope": scopes[parts[2]], "audience": "garage",
				"user": map[string]any{"id": 9, "sub": people[parts[1]], "email": people[parts[1]], "name": parts[1], "role": "member"},
				"app":  map[string]any{"id": 5, "slug": "hub", "name": "Pocket Hub"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() core.Manifest { mu.Lock(); defer mu.Unlock(); return manifest }
}

// get calls a capability route and decodes its answer.
func (p *twoPeople) get(tok, path string) map[string]any {
	p.t.Helper()
	code, body := p.do(tok, "GET", path, nil)
	if code != http.StatusOK {
		p.t.Fatalf("GET %s: %d %s", path, code, body)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		p.t.Fatalf("not json: %s", body)
	}
	return m
}

// mk creates something on a vehicle and returns its id.
func (p *twoPeople) mk(tok string, vehicle int64, path string, in any) int64 {
	p.t.Helper()
	code, body := p.do(tok, "POST", fmt.Sprintf("/api/v1/vehicles/%d%s", vehicle, path), in)
	if code != http.StatusCreated {
		p.t.Fatalf("POST %s: %d %s", path, code, body)
	}
	return p.id(body, "id")
}

// vehicle creates a vehicle and returns its id.
func (p *twoPeople) vehicle(tok string, in any) int64 {
	p.t.Helper()
	code, body := p.do(tok, "POST", "/api/v1/vehicles", in)
	if code != http.StatusCreated {
		p.t.Fatalf("create vehicle: %d %s", code, body)
	}
	return p.id(body, "id")
}

func capItems(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

func capItem(t *testing.T, m map[string]any, title string) map[string]any {
	t.Helper()
	for _, it := range capItems(t, m) {
		if it["title"] == title {
			return it
		}
	}
	t.Fatalf("no %q in %v", title, m["items"])
	return nil
}

func TestPocketSearch(t *testing.T) {
	p := newTwoPeople(t)
	g := p.seedOwner()
	vid := fmt.Sprint(g.vehicle)

	got := capItems(t, p.get(p.aTok, "/api/v1/pocket/search?q=fabia"))
	if len(got) != 1 || got[0]["id"] != "vehicle:"+vid || got[0]["kind"] != "vehicle" || got[0]["title"] != "Skoda Fabia" || got[0]["url"] != "/vehicles/"+vid ||
		got[0]["snippet"] != "SECRET-1 · 100,200 km" {
		t.Fatalf("vehicle by model: %v", got)
	}
	if got := capItems(t, p.get(p.aTok, "/api/v1/pocket/search?q=secret-1")); len(got) != 1 || got[0]["kind"] != "vehicle" {
		t.Fatalf("vehicle by plate: %v", got)
	}
	rec := capItem(t, p.get(p.aTok, "/api/v1/pocket/search?q=secret+repair"), "Repair: Owner secret repair")
	if rec["id"] != fmt.Sprintf("record:%d", g.record) || rec["kind"] != "record" || rec["url"] != "/vehicles/"+vid+"/service" || rec["date"] != "2026-10-01" ||
		rec["snippet"] != "Skoda Fabia · 1 Oct 2026 · 100,000 km · 100.00 EUR" {
		t.Fatalf("record: %v", rec)
	}
	fill := capItem(t, p.get(p.aTok, "/api/v1/pocket/search?q=owner+station"), "Fuel: 40 l")
	if fill["id"] != fmt.Sprintf("fill-up:%d", g.fuel) || fill["kind"] != "fill-up" || fill["url"] != "/vehicles/"+vid+"/fuel" || fill["date"] != "2026-10-02" ||
		fill["snippet"] != "Skoda Fabia · 2 Oct 2026 · 100,100 km · 90.00 EUR · Owner station" {
		t.Fatalf("fill-up: %v", fill)
	}
	rem := capItem(t, p.get(p.aTok, "/api/v1/pocket/search?q=oil+change"), "Owner oil change")
	if rem["id"] != fmt.Sprintf("reminder:%d", g.reminder) || rem["kind"] != "reminder" || rem["url"] != "/vehicles/"+vid+"/reminders" || rem["date"] != "2026-10-10" ||
		!strings.HasPrefix(rem["snippet"].(string), "Skoda Fabia · ") || !strings.HasSuffix(rem["snippet"].(string), "overdue") && !strings.HasSuffix(rem["snippet"].(string), "days") {
		t.Fatalf("reminder: %v", rem)
	}

	// a vehicle whose name has the words leads, then titles, then the rest
	p.mk(p.aTok, g.vehicle, "/records", map[string]any{"kind": "service", "description": "Fabia inspection", "date": "2026-11-01"})
	p.mk(p.aTok, g.vehicle, "/records", map[string]any{"kind": "expense", "description": "Parking", "notes": "fabia at the airport", "date": "2026-11-02"})
	got = capItems(t, p.get(p.aTok, "/api/v1/pocket/search?q=fabia"))
	if len(got) != 3 || got[0]["kind"] != "vehicle" || got[1]["title"] != "Service: Fabia inspection" || got[2]["title"] != "Expense: Parking" {
		t.Fatalf("order: %v", got)
	}
	if got := capItems(t, p.get(p.aTok, "/api/v1/pocket/search?q=fabia&limit=2")); len(got) != 2 {
		t.Fatalf("limit 2: %v", got)
	}
	for _, q := range []string{"", "%3F%3F", "*", "%25"} {
		if got := capItems(t, p.get(p.aTok, "/api/v1/pocket/search?q="+q)); len(got) != 0 {
			t.Fatalf("no words (%q): %v", q, got)
		}
	}

	// another person never sees the owner's car or anything on it
	for _, q := range []string{"fabia", "secret", "owner", "oil"} {
		if got := capItems(t, p.get(p.bTok, "/api/v1/pocket/search?q="+q)); len(got) != 0 {
			t.Fatalf("member finds the owner's %q: %v", q, got)
		}
	}
	// once the owner shares the car, the member finds it, as in the vehicle list
	if code, body := p.do(p.aTok, "PUT", "/api/v1/vehicles/"+vid+"/members", map[string]any{"members": []any{map[string]any{"user_id": p.b.ID, "role": "viewer"}}}); code != 200 {
		t.Fatalf("share: %d %s", code, body)
	}
	if got := capItems(t, p.get(p.bTok, "/api/v1/pocket/search?q=secret")); len(got) != 2 {
		t.Fatalf("member on a shared car: %v", got)
	}
	if code, _ := p.do("", "GET", "/api/v1/pocket/search?q=fabia", nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", code)
	}
}

func TestPocketDay(t *testing.T) {
	p := newTwoPeople(t)
	golf := p.vehicle(p.aTok, map[string]any{"name": "Golf", "make": "VW", "model": "Golf", "plate": "G-1"})
	vid := fmt.Sprint(golf)
	today := store.Today()
	fuel := p.mk(p.aTok, golf, "/fuel", map[string]any{"date": today, "odometer": 84210, "volume": 42, "cost": 6240, "station": "OMV", "notes": "cheap day"})
	oil := p.mk(p.aTok, golf, "/records", map[string]any{"kind": "service", "description": "Oil change", "date": today, "odometer": 84200, "cost": 8900, "vendor": "Bosch"})
	p.mk(p.aTok, golf, "/records", map[string]any{"kind": "expense", "category": "insurance", "description": "Insurance", "date": today, "cost": 45000})
	tyres := p.mk(p.aTok, golf, "/reminders", map[string]any{"title": "Tyres", "due_date": today})
	p.mk(p.aTok, golf, "/records", map[string]any{"kind": "repair", "description": "Wipers", "date": addDays(today, -1), "cost": 2000})

	day := p.get(p.aTok, "/api/v1/pocket/day/"+today)
	if day["date"] != today || day["summary"] != "Fuelled the Golf, 62.40 EUR; 2 records for the Golf, 539.00 EUR; Tyres due for the Golf" || len(capItems(t, day)) != 4 {
		t.Fatalf("day: %v", day)
	}
	if ms, _ := day["metrics"].([]any); len(ms) != 1 || ms[0].(map[string]any)["label"] != "Spent" || ms[0].(map[string]any)["value"] != "601.40 EUR" {
		t.Fatalf("metrics: %v", day["metrics"])
	}
	f := capItem(t, day, "Fuel: 42 l")
	if f["id"] != fmt.Sprintf("fill-up:%d", fuel) || f["kind"] != "fuel" || f["sub"] != "Golf · 84,210 km · OMV" || f["amount"] != 6240.0 || f["currency"] != "EUR" ||
		f["note"] != "cheap day" || f["url"] != "/vehicles/"+vid+"/fuel" {
		t.Fatalf("fill-up: %v", f)
	}
	o := capItem(t, day, "Service: Oil change")
	if o["id"] != fmt.Sprintf("record:%d", oil) || o["kind"] != "service" || o["sub"] != "Golf · 84,200 km · Bosch" || o["amount"] != 8900.0 || o["url"] != "/vehicles/"+vid+"/service" {
		t.Fatalf("service: %v", o)
	}
	if e := capItem(t, day, "Expense: Insurance"); e["kind"] != "other" || e["sub"] != "Golf" || e["amount"] != 45000.0 {
		t.Fatalf("expense: %v", e)
	}
	if d := capItem(t, day, "Tyres"); d["id"] != fmt.Sprintf("reminder:%d", tyres) || d["kind"] != "due" || d["sub"] != "Golf" || d["amount"] != nil || d["url"] != "/vehicles/"+vid+"/reminders" {
		t.Fatalf("due: %v", d)
	}
	// one record says what it was; the person's own volume unit
	if d := p.get(p.aTok, "/api/v1/pocket/day/"+addDays(today, -1)); d["summary"] != "Wipers for the Golf, 20.00 EUR" || len(capItems(t, d)) != 1 {
		t.Fatalf("yesterday: %v", d)
	}
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"volume_unit": "gal_us"}); code != 200 {
		t.Fatalf("settings: %d %s", code, body)
	}
	capItem(t, p.get(p.aTok, "/api/v1/pocket/day/"+today), "Fuel: 11.1 gal")

	// a day with nothing, somebody else's day, a bad day
	if d := p.get(p.aTok, "/api/v1/pocket/day/2019-03-04"); d["summary"] != nil || len(capItems(t, d)) != 0 {
		t.Fatalf("an empty day: %v", d)
	}
	if d := p.get(p.bTok, "/api/v1/pocket/day/"+today+"?tz=Europe/Sarajevo"); d["summary"] != nil || len(capItems(t, d)) != 0 {
		t.Fatalf("member sees the owner's day: %v", d)
	}
	if code, _ := p.do(p.aTok, "GET", "/api/v1/pocket/day/yesterday", nil); code != http.StatusBadRequest {
		t.Fatalf("bad day: %d", code)
	}
}

func TestPocketWidget(t *testing.T) {
	p := newTwoPeople(t)
	// no vehicles: nothing to show
	if w := p.get(p.bTok, "/api/v1/pocket/widget"); w["line"] != nil || w["metrics"] != nil || len(capItems(t, w)) != 0 {
		t.Fatalf("without vehicles: %v", w)
	}
	today := store.Today()
	golf := p.vehicle(p.aTok, map[string]any{"name": "Golf", "make": "VW", "model": "Golf"})
	vid := fmt.Sprint(golf)
	p.mk(p.aTok, golf, "/fuel", map[string]any{"date": today, "odometer": 84210, "volume": 42, "cost": 6240})
	late := p.mk(p.aTok, golf, "/reminders", map[string]any{"title": "Oil change", "due_date": addDays(today, -3)})
	p.mk(p.aTok, golf, "/reminders", map[string]any{"title": "Tyres", "due_odometer": 84710, "soon_km": 1000})
	p.mk(p.aTok, golf, "/reminders", map[string]any{"title": "Registration", "due_date": addDays(today, 200), "soon_days": 30})
	old := p.vehicle(p.aTok, map[string]any{"name": "Old van", "make": "Ford", "model": "Transit", "status": "archived"})
	p.mk(p.aTok, old, "/reminders", map[string]any{"title": "Van inspection", "due_date": addDays(today, -1)})

	w := p.get(p.aTok, "/api/v1/pocket/widget")
	if w["line"] != "Oil change is 3 days overdue, and 1 more" || w["url"] != "/reminders" {
		t.Fatalf("widget: %v", w)
	}
	ms := w["metrics"].([]any)
	if len(ms) != 2 || ms[0].(map[string]any)["value"] != "1" || ms[1].(map[string]any)["value"] != "1" {
		t.Fatalf("metrics: %v", ms)
	}
	got := capItems(t, w)
	if len(got) != 3 || got[0]["id"] != fmt.Sprintf("reminder:%d", late) || got[0]["detail"] != "Golf · 3 days overdue" || got[0]["url"] != "/vehicles/"+vid+"/reminders" ||
		got[1]["title"] != "Tyres" || got[1]["detail"] != "Golf · 500 km left" ||
		got[2]["id"] != "vehicle:"+vid || got[2]["title"] != "Golf" || got[2]["detail"] != "84,210 km · filled up today" || got[2]["url"] != "/vehicles/"+vid {
		t.Fatalf("rows: %v", got)
	}
	// only what is coming up: the line is the soonest one
	if code, body := p.do(p.aTok, "DELETE", fmt.Sprintf("/api/v1/vehicles/%d/reminders/%d", golf, late), nil); code >= 300 {
		t.Fatalf("delete: %d %s", code, body)
	}
	if w := p.get(p.aTok, "/api/v1/pocket/widget"); w["line"] != "Tyres due in 500 km" {
		t.Fatalf("after: %v", w)
	}
	// the member sees none of it
	if w := p.get(p.bTok, "/api/v1/pocket/widget"); w["line"] != nil || len(capItems(t, w)) != 0 {
		t.Fatalf("member sees the owner's widget: %v", w)
	}
}

func TestReminderWords(t *testing.T) {
	n := func(v int) *int { return &v }
	f := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		rm         store.Reminder
		when, line string
	}{
		{store.Reminder{Title: "Service", Status: "active", DaysLeft: n(12), SoonDays: 30}, "in 12 days", "Service due in 12 days"},
		{store.Reminder{Title: "Service", Status: "active", DaysLeft: n(1)}, "tomorrow", "Service is due tomorrow"},
		{store.Reminder{Title: "Service", Status: "active", DaysLeft: n(0)}, "today", "Service is due today"},
		{store.Reminder{Title: "Service", Status: "active", DaysLeft: n(-1)}, "1 day overdue", "Service is 1 day overdue"},
		{store.Reminder{Title: "Oil", Status: "active", KmLeft: f(800), SoonKm: 1000}, "800 km left", "Oil due in 800 km"},
		{store.Reminder{Title: "Oil", Status: "active", KmLeft: f(-1200)}, "1,200 km over", "Oil is 1,200 km overdue"},
		// both: the line follows whichever is more pressing
		{store.Reminder{Title: "Oil", Status: "active", DueDate: "2027-02-25", DaysLeft: n(90), SoonDays: 30, KmLeft: f(300), SoonKm: 1000}, "on 25 Feb 2027, 300 km left", "Oil due in 300 km"},
		{store.Reminder{Title: "Oil", Status: "done"}, "Done", "Oil"},
	} {
		got := whenWords(&c.rm, "km")
		if got != c.when {
			t.Errorf("whenWords(%+v) = %q, want %q", c.rm, got, c.when)
		}
		if line := reminderLine(&c.rm, "km"); line != c.line {
			t.Errorf("reminderLine(%+v) = %q, want %q", c.rm, line, c.line)
		}
	}
	if got := volumeWords(41.37, "l", false); got != "41.37 l" {
		t.Errorf("litres: %q", got)
	}
	if got := volumeWords(38.5, "gal_us", true); got != "38.5 kWh" {
		t.Errorf("electric: %q", got)
	}
	if got := odoWords(1234567.4, "mi"); got != "1,234,567 mi" {
		t.Errorf("odometer: %q", got)
	}
}

// Pocket Search, Day and Hub reach exactly their own route with a link
// token for its scope; the old vehicles:read scope opens none of them, and
// the manifest says what Garage offers.
func TestPocketCapabilityLinks(t *testing.T) {
	coreSrv, manifest := capCore(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(coreSrv.URL, core.Options{DB: s.store.DB, Token: "pca_garage"}) })
	g := p.seedOwner()
	today := store.Today()
	routes := map[string]string{
		"search": "/api/v1/pocket/search?q=fabia",
		"day":    "/api/v1/pocket/day/2026-10-01",
		"widget": "/api/v1/pocket/widget",
	}
	for scope, own := range routes {
		tok := "pcx_owner_" + scope
		code, body := p.do(tok, "GET", own, nil)
		if code != 200 {
			t.Fatalf("%s through its link: %d %s", scope, code, body)
		}
		if !strings.Contains(body, "Fabia") && !strings.Contains(body, "Owner secret repair") {
			t.Errorf("%s through its link answers someone else's: %s", scope, body)
		}
		if code, _ := p.do(tok, "GET", "/api/v1/auth/me", nil); code != 200 {
			t.Errorf("auth/me with the %s link: %d", scope, code)
		}
		for other, path := range routes {
			if other != scope {
				if code, _ := p.do(tok, "GET", path, nil); code != http.StatusForbidden {
					t.Errorf("%s with the %s link: %d, want 403", path, scope, code)
				}
			}
		}
		// routes Garage does not have at all answer 404 or 405 before anyone is asked who calls
		for _, c := range []struct{ m, p string }{{"POST", own}, {"POST", "/api/v1/pocket/capture"}} {
			if code, _ := p.do(tok, c.m, c.p, map[string]any{}); code < 400 {
				t.Errorf("%s %s with the %s link: %d, want a refusal", c.m, c.p, scope, code)
			}
		}
		for _, c := range []struct{ m, p string }{
			{"GET", "/api/v1/vehicles"}, {"GET", fmt.Sprintf("/api/v1/vehicles/%d/fuel", g.vehicle)}, {"GET", "/api/v1/reminders"},
			{"GET", "/api/v1/home"}, {"GET", "/api/v1/pocket/day/" + today + "x"},
		} {
			if code, _ := p.do(tok, c.m, c.p, map[string]any{}); code != http.StatusForbidden {
				t.Errorf("%s %s with the %s link: %d, want 403", c.m, c.p, scope, code)
			}
		}
	}
	// the member's link sees only the member's things
	if got := capItems(t, p.get("pcx_member_search", routes["search"])); len(got) != 0 {
		t.Errorf("member's search link finds the owner's car: %v", got)
	}
	if w := p.get("pcx_member_widget", routes["widget"]); len(capItems(t, w)) != 0 {
		t.Errorf("member's widget link: %v", w)
	}
	// no quick capture here, and the old scopes open none of the new routes
	if code, _ := p.do("pcx_owner_capture", "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "abcdefghijklmnop", "kind": "fill-up", "text": "40 l"}); code < 400 {
		t.Errorf("capture link: %d, want a refusal", code)
	}
	for _, path := range routes {
		if code, _ := p.do("pcx_owner_read", "GET", path, nil); code != http.StatusForbidden {
			t.Errorf("%s with vehicles:read: %d, want 403", path, code)
		}
	}
	if code, _ := p.do("pcx_owner_read", "GET", "/api/v1/vehicles", nil); code != 200 {
		t.Errorf("vehicles:read still reads the vehicles: %d", code)
	}

	if err := p.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := manifest()
	if m.Category != "Places" || m.Capabilities.Search == nil || m.Capabilities.Day == nil || m.Capabilities.Widget == nil || m.Capabilities.Capture != nil {
		t.Fatalf("manifest capabilities: %+v %+v", m.Category, m.Capabilities)
	}
	if b := m.Capabilities.Search.Bangs; len(b) != 2 || b[0] != "garage" || b[1] != "car" {
		t.Errorf("bangs: %v", b)
	}
	if d := m.Capabilities.Day; d.Label != "Garage" || d.One != "entry" || d.Many != "entries" || d.Unit != "amount" {
		t.Errorf("day feed: %+v", d)
	}
	offered := map[string]bool{}
	for _, o := range m.Offers {
		offered[o.Scope] = true
	}
	for _, sc := range []string{"vehicles:read", core.ScopeSearch, core.ScopeDay, core.ScopeWidget} {
		if !offered[sc] {
			t.Errorf("manifest does not offer %s: %+v", sc, m.Offers)
		}
	}
	if offered[core.ScopeCapture] {
		t.Errorf("manifest offers capture: %+v", m.Offers)
	}
}
