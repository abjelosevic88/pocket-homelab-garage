package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// twoPeople is a box with the owner (admin, first account) and a household
// member, each with a password and an API token.
type twoPeople struct {
	t        *testing.T
	s        *Server
	st       *store.Store
	ts       *httptest.Server
	a, b     *store.User
	aTok     string
	bTok     string
	cancel   context.CancelFunc
	password string
}

func newTwoPeople(t *testing.T, mutate ...func(*Server)) *twoPeople {
	t.Helper()
	dir := t.TempDir()
	cfg := testConfig(dir)
	cfg.FireflyURL, cfg.FireflyToken = "http://firefly.invalid", "owner-firefly-token"
	cfg.NotifyURL = "http://apprise.invalid/notify/garage"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	p := &twoPeople{t: t, st: st, password: "correct horse battery"}
	h, _ := bcrypt.GenerateFromPassword([]byte(p.password), bcrypt.MinCost)
	p.a = &store.User{Email: "owner@example.com", Name: "Owner", PasswordHash: string(h), Role: "admin"}
	p.b = &store.User{Email: "member@example.com", Name: "Member", PasswordHash: string(h), Role: "member"}
	if err := st.CreateUser(context.Background(), p.a); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUser(context.Background(), p.b); err != nil {
		t.Fatal(err)
	}
	p.aTok, _, _ = st.CreateAPIToken(context.Background(), p.a.ID, "a")
	p.bTok, _, _ = st.CreateAPIToken(context.Background(), p.b.ID, "b")
	p.s = New(cfg, st, log, nil)
	for _, m := range mutate {
		m(p.s)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	if err := p.s.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	p.ts = httptest.NewServer(p.s.Router())
	t.Cleanup(func() { p.ts.Close(); cancel(); st.Close() })
	return p
}

// do sends a request with a bearer token and returns status + body.
func (p *twoPeople) do(tok, method, path string, body any) (int, string) {
	p.t.Helper()
	var rd io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case string:
		rd, ct = strings.NewReader(b), "application/x-www-form-urlencoded"
	default:
		buf, _ := json.Marshal(b)
		rd, ct = bytes.NewReader(buf), "application/json"
	}
	req, _ := http.NewRequest(method, p.ts.URL+path, rd)
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

// basic sends a LubeLogger-style request with Basic auth.
func (p *twoPeople) basic(email, method, path, form string) (int, string) {
	p.t.Helper()
	var rd io.Reader
	if form != "" {
		rd = strings.NewReader(form)
	}
	req, _ := http.NewRequest(method, p.ts.URL+path, rd)
	if form != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.SetBasicAuth(email, p.password)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(out)
}

func (p *twoPeople) id(body, key string) int64 {
	p.t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		p.t.Fatalf("not json: %s", body)
	}
	n, ok := m[key].(float64)
	if !ok {
		p.t.Fatalf("no %s in %s", key, body)
	}
	return int64(n)
}

// ownerGarage gives the owner one vehicle with a row of every kind.
type ownerGarage struct {
	vehicle, record, fuel, odo, reminder, plan, note, doc int64
}

func (p *twoPeople) seedOwner() ownerGarage {
	p.t.Helper()
	var g ownerGarage
	code, body := p.do(p.aTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Skoda", "model": "Fabia", "plate": "SECRET-1", "firefly_tag": "fabia"})
	if code != http.StatusCreated {
		p.t.Fatalf("create vehicle: %d %s", code, body)
	}
	g.vehicle = p.id(body, "id")
	v := fmt.Sprintf("/api/v1/vehicles/%d", g.vehicle)
	mk := func(path string, in any, want int) int64 {
		code, body := p.do(p.aTok, "POST", v+path, in)
		if code != want {
			p.t.Fatalf("POST %s: %d %s", path, code, body)
		}
		return p.id(body, "id")
	}
	g.record = mk("/records", map[string]any{"kind": "repair", "description": "Owner secret repair", "date": "2026-10-01", "cost": 10000, "odometer": 100000}, http.StatusCreated)
	g.fuel = mk("/fuel", map[string]any{"date": "2026-10-02", "odometer": 100100, "volume": 40, "cost": 9000, "station": "Owner station"}, http.StatusCreated)
	g.odo = mk("/odometer", map[string]any{"date": "2026-10-03", "reading": 100200}, http.StatusCreated)
	g.reminder = mk("/reminders", map[string]any{"title": "Owner oil change", "due_date": "2026-10-10"}, http.StatusCreated)
	g.plan = mk("/plans", map[string]any{"title": "Owner plan"}, http.StatusCreated)
	g.note = mk("/notes", map[string]any{"title": "Owner note", "body": "radio code 1234"}, http.StatusCreated)
	// a document (multipart)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "policy.txt")
	_, _ = fw.Write([]byte("owner insurance policy"))
	_ = mw.WriteField("name", "Owner policy")
	_ = mw.Close()
	req, _ := http.NewRequest("POST", p.ts.URL+v+"/documents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.aTok)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		p.t.Fatalf("upload: %d %s", res.StatusCode, b)
	}
	g.doc = p.id(string(b), "id")
	_ = p.st.PutDawarichDay(context.Background(), g.vehicle, "2026-10-04", 42, 44)
	return g
}

func (p *twoPeople) countOwnerRows() map[string]int {
	out := map[string]int{}
	for _, t := range []string{"records", "fuel", "odometer", "reminders", "plans", "notes", "documents", "dawarich_days"} {
		var n int
		_ = p.st.DB.QueryRow(`SELECT COUNT(1) FROM `+t+` WHERE user_id = ?`, p.a.ID).Scan(&n)
		out[t] = n
	}
	return out
}

func TestMemberCannotSeeOwnersGarage(t *testing.T) {
	p := newTwoPeople(t)
	g := p.seedOwner()
	before := p.countOwnerRows()
	v := fmt.Sprintf("/api/v1/vehicles/%d", g.vehicle)

	// every child row names its owner
	for table, n := range before {
		if n == 0 {
			t.Errorf("%s: no rows owned by the owner", table)
		}
		var orphans int
		_ = p.st.DB.QueryRow(`SELECT COUNT(1) FROM ` + table + ` WHERE user_id IS NULL`).Scan(&orphans)
		if orphans != 0 {
			t.Errorf("%s: %d rows without user_id", table, orphans)
		}
	}

	// lists come back empty
	for _, path := range []string{"/api/v1/vehicles", "/api/v1/vehicles?all=1", "/api/v1/reminders"} {
		code, body := p.do(p.bTok, "GET", path, nil)
		if code != 200 || strings.Contains(body, "Fabia") || strings.Contains(body, "Owner") {
			t.Errorf("GET %s as member: %d %s", path, code, body)
		}
	}
	code, body := p.do(p.bTok, "GET", "/api/v1/home", nil)
	if code != 200 || strings.Contains(body, "Fabia") || strings.Contains(body, "Owner") {
		t.Errorf("home leaks: %d %s", code, body)
	}
	code, body = p.do(p.bTok, "GET", "/api/v1/dashboard", nil)
	if code != 200 || !strings.Contains(body, `"vehicles":0`) {
		t.Errorf("dashboard leaks: %d %s", code, body)
	}

	// reads, searches and exports are 404, never 403
	reads := []string{"", "/overview", "/activity", "/image", "/suggestions", "/records", "/records?q=secret", fmt.Sprintf("/records/%d", g.record),
		"/fuel", fmt.Sprintf("/fuel/%d", g.fuel), "/odometer", "/reminders", "/plans", "/notes", "/documents", fmt.Sprintf("/documents/%d/file", g.doc),
		"/reports", "/trips", "/dawarich/days", "/firefly/unmatched", "/export/calendar.ics", "/export/records.csv", "/export/fuel.csv", "/export/odometer.csv", "/export/vehicle.json"}
	for _, path := range reads {
		if code, body := p.do(p.bTok, "GET", v+path, nil); code != http.StatusNotFound {
			t.Errorf("GET %s as member: %d %s", path, code, body)
		}
	}
	// writes are 404 too
	writes := []struct{ method, path string }{
		{"PUT", ""}, {"DELETE", ""}, {"PUT", "/members"}, {"DELETE", "/image"},
		{"POST", "/records"}, {"PUT", fmt.Sprintf("/records/%d", g.record)}, {"DELETE", fmt.Sprintf("/records/%d", g.record)}, {"POST", fmt.Sprintf("/records/%d/firefly", g.record)},
		{"POST", "/fuel"}, {"PUT", fmt.Sprintf("/fuel/%d", g.fuel)}, {"DELETE", fmt.Sprintf("/fuel/%d", g.fuel)},
		{"POST", "/odometer"}, {"PUT", fmt.Sprintf("/odometer/%d", g.odo)}, {"DELETE", fmt.Sprintf("/odometer/%d", g.odo)}, {"POST", "/odometer/confirm"},
		{"POST", "/reminders"}, {"PUT", fmt.Sprintf("/reminders/%d", g.reminder)}, {"DELETE", fmt.Sprintf("/reminders/%d", g.reminder)}, {"POST", fmt.Sprintf("/reminders/%d/done", g.reminder)},
		{"POST", "/plans"}, {"PUT", fmt.Sprintf("/plans/%d", g.plan)}, {"DELETE", fmt.Sprintf("/plans/%d", g.plan)}, {"POST", fmt.Sprintf("/plans/%d/done", g.plan)},
		{"POST", "/notes"}, {"PUT", fmt.Sprintf("/notes/%d", g.note)}, {"DELETE", fmt.Sprintf("/notes/%d", g.note)},
		{"PUT", fmt.Sprintf("/documents/%d", g.doc)}, {"DELETE", fmt.Sprintf("/documents/%d", g.doc)},
		{"POST", "/firefly/sync"}, {"POST", "/dawarich/sync"},
	}
	for _, w := range writes {
		if code, body := p.do(p.bTok, w.method, v+w.path, map[string]any{"title": "x", "description": "x", "reading": 1, "volume": 1, "members": []any{map[string]any{"user_id": p.b.ID, "role": "editor"}}}); code != http.StatusNotFound {
			t.Errorf("%s %s as member: %d %s", w.method, w.path, code, body)
		}
	}

	// the member's own rows can't be pointed at the owner's vehicle either
	code, body = p.do(p.bTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Fiat", "model": "Panda"})
	if code != http.StatusCreated {
		t.Fatalf("member vehicle: %d %s", code, body)
	}
	bv := p.id(body, "id")
	if code, body := p.do(p.bTok, "PUT", fmt.Sprintf("/api/v1/vehicles/%d/records/%d", bv, g.record), map[string]any{"description": "hijack"}); code != http.StatusNotFound {
		t.Errorf("update owner's record through own vehicle: %d %s", code, body)
	}
	if code, body := p.do(p.bTok, "DELETE", fmt.Sprintf("/api/v1/vehicles/%d/documents/%d", bv, g.doc), nil); code != http.StatusNotFound {
		t.Errorf("delete owner's document through own vehicle: %d %s", code, body)
	}

	// LubeLogger-compatible API with the member's password and token
	compat := []string{"/api/vehicles", "/api/vehicle/info"}
	for _, path := range compat {
		code, body := p.basic(p.b.Email, "GET", path, "")
		if code != 200 || strings.Contains(body, "Fabia") || strings.Contains(body, "SECRET-1") {
			t.Errorf("compat %s as member: %d %s", path, code, body)
		}
	}
	for _, kind := range []string{"gas", "service", "repair", "upgrade", "tax", "odometer"} {
		path := fmt.Sprintf("/api/vehicle/%srecords?vehicleId=%d", kind, g.vehicle)
		if code, body := p.basic(p.b.Email, "GET", path, ""); code != http.StatusNotFound {
			t.Errorf("compat GET %s as member: %d %s", path, code, body)
		}
		if code, body := p.do(p.bTok, "GET", path, nil); code != http.StatusNotFound {
			t.Errorf("compat GET %s with member token: %d %s", path, code, body)
		}
	}
	for _, path := range []string{"/api/vehicle/reminders", "/api/vehicle/planrecords", "/api/vehicle/notes"} {
		if code, body := p.basic(p.b.Email, "GET", fmt.Sprintf("%s?vehicleId=%d", path, g.vehicle), ""); code != http.StatusNotFound {
			t.Errorf("compat %s as member: %d %s", path, code, body)
		}
	}
	// compat writes addressed by record id alone
	if code, body := p.basic(p.b.Email, "POST", "/api/vehicle/repairrecords/update", fmt.Sprintf("id=%d&description=hijack&date=2026-10-05&cost=1", g.record)); code != http.StatusNotFound {
		t.Errorf("compat update owner's record: %d %s", code, body)
	}
	if code, body := p.basic(p.b.Email, "POST", "/api/vehicle/gasrecords/update", fmt.Sprintf("id=%d&vehicleId=%d&fuelConsumed=1&odometer=5", g.fuel, bv)); code != http.StatusNotFound {
		t.Errorf("compat update owner's fuel through own vehicle: %d %s", code, body)
	}
	for _, del := range []struct {
		kind string
		id   int64
	}{{"repair", g.record}, {"gas", g.fuel}, {"odometer", g.odo}} {
		if code, body := p.basic(p.b.Email, "DELETE", fmt.Sprintf("/api/vehicle/%srecords/delete?id=%d", del.kind, del.id), ""); code != http.StatusNotFound {
			t.Errorf("compat delete owner's %s: %d %s", del.kind, code, body)
		}
	}
	if code, body := p.basic(p.b.Email, "POST", fmt.Sprintf("/api/vehicle/gasrecords/add?vehicleId=%d", g.vehicle), "fuelConsumed=10&odometer=1&cost=1"); code != http.StatusNotFound {
		t.Errorf("compat add to owner's vehicle: %d %s", code, body)
	}
	// an email without the password is not a sign-in
	req, _ := http.NewRequest("GET", p.ts.URL+"/api/vehicles", nil)
	req.SetBasicAuth(p.a.Email, "wrong")
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("basic auth with a wrong password: %d", res.StatusCode)
	}

	// calendar subscription with the member's token
	if code, body := p.do("", "GET", fmt.Sprintf("/cal/%d.ics?token=%s", g.vehicle, url.QueryEscape(p.bTok)), nil); code != http.StatusNotFound {
		t.Errorf("calendar feed as member: %d %s", code, body)
	}

	// nothing of the owner's changed
	after := p.countOwnerRows()
	for k, n := range before {
		if after[k] != n {
			t.Errorf("%s: owner had %d rows, now %d", k, n, after[k])
		}
	}
	code, body = p.do(p.aTok, "GET", fmt.Sprintf("%s/records/%d", v, g.record), nil)
	if code != 200 || !strings.Contains(body, "Owner secret repair") {
		t.Errorf("owner's record changed: %d %s", code, body)
	}
}

func TestAdminHasNoReadAccessToMembersGarage(t *testing.T) {
	p := newTwoPeople(t)
	code, body := p.do(p.bTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Fiat", "model": "Panda", "plate": "MEMBER-1"})
	if code != http.StatusCreated {
		t.Fatalf("member vehicle: %d %s", code, body)
	}
	bv := p.id(body, "id")
	code, body = p.do(p.bTok, "POST", fmt.Sprintf("/api/v1/vehicles/%d/notes", bv), map[string]any{"title": "Member diary"})
	if code != http.StatusCreated {
		t.Fatalf("member note: %d %s", code, body)
	}
	for _, path := range []string{"/api/v1/vehicles", "/api/v1/vehicles?all=1", "/api/v1/home"} {
		if code, body := p.do(p.aTok, "GET", path, nil); code != 200 || strings.Contains(body, "Panda") {
			t.Errorf("admin GET %s: %d %s", path, code, body)
		}
	}
	for _, path := range []string{"", "/notes", "/export/vehicle.json"} {
		if code, _ := p.do(p.aTok, "GET", fmt.Sprintf("/api/v1/vehicles/%d%s", bv, path), nil); code != http.StatusNotFound {
			t.Errorf("admin GET member's vehicle%s: %d", path, code)
		}
	}
	if code, _ := p.do(p.aTok, "DELETE", fmt.Sprintf("/api/v1/vehicles/%d", bv), nil); code != http.StatusNotFound {
		t.Errorf("admin deletes member's vehicle: %d", code)
	}
	if code, body := p.basic(p.a.Email, "GET", "/api/vehicles", ""); strings.Contains(body, "MEMBER-1") {
		t.Errorf("admin compat list: %d %s", code, body)
	}
	// the member's new rows belong to the member
	var owner int64
	_ = p.st.DB.QueryRow(`SELECT user_id FROM notes WHERE vehicle_id = ?`, bv).Scan(&owner)
	if owner != p.b.ID {
		t.Errorf("member's note owned by %d", owner)
	}
}

func TestSharedVehicleRolesAndOwnerOnlyIntegrations(t *testing.T) {
	p := newTwoPeople(t)
	g := p.seedOwner()
	v := fmt.Sprintf("/api/v1/vehicles/%d", g.vehicle)
	share := func(role string) {
		if code, body := p.do(p.aTok, "PUT", v+"/members", map[string]any{"members": []any{map[string]any{"user_id": p.b.ID, "role": role}}}); code != 200 {
			t.Fatalf("share: %d %s", code, body)
		}
	}
	share("viewer")
	if code, body := p.do(p.bTok, "GET", v+"/records", nil); code != 200 || !strings.Contains(body, "Owner secret repair") {
		t.Fatalf("viewer can't read shared vehicle: %d %s", code, body)
	}
	if code, _ := p.do(p.bTok, "PUT", fmt.Sprintf("%s/records/%d", v, g.record), map[string]any{"description": "x"}); code != http.StatusForbidden {
		t.Errorf("viewer updated a record: %d", code)
	}
	if code, body := p.basic(p.b.Email, "POST", "/api/vehicle/repairrecords/update", fmt.Sprintf("id=%d&description=hijack&date=2026-10-05", g.record)); code != http.StatusForbidden {
		t.Errorf("viewer updated through compat: %d %s", code, body)
	}
	if code, _ := p.basic(p.b.Email, "DELETE", fmt.Sprintf("/api/vehicle/repairrecords/delete?id=%d", g.record), ""); code != http.StatusNotFound {
		t.Errorf("viewer deleted through compat: %d", code)
	}

	share("editor")
	if code, body := p.do(p.bTok, "PUT", v, map[string]any{"name": "Family car", "firefly_tag": "someone-elses-tag", "dawarich": true}); code != 200 {
		t.Fatalf("editor update: %d %s", code, body)
	}
	code, body := p.do(p.aTok, "GET", v, nil)
	if code != 200 || !strings.Contains(body, `"firefly_tag":"fabia"`) || !strings.Contains(body, `"name":"Family car"`) || strings.Contains(body, `"dawarich":true`) {
		t.Errorf("editor changed the owner's integration binding: %s", body)
	}
	if code, _ := p.do(p.bTok, "POST", fmt.Sprintf("%s/records/%d/firefly", v, g.record), nil); code != http.StatusForbidden {
		t.Errorf("editor pushed into the owner's Firefly: %d", code)
	}
	if code, _ := p.do(p.bTok, "PUT", v+"/members", map[string]any{"members": []any{}}); code != http.StatusForbidden {
		t.Errorf("editor changed members: %d", code)
	}
	if code, _ := p.do(p.bTok, "DELETE", v, nil); code != http.StatusForbidden {
		t.Errorf("editor deleted the vehicle: %d", code)
	}
	// the editor's rows on the owner's car belong to the car's owner, authored by the editor
	code, body = p.do(p.bTok, "POST", v+"/records", map[string]any{"kind": "service", "description": "Editor wash", "date": "2026-10-06"})
	if code != http.StatusCreated {
		t.Fatalf("editor record: %d %s", code, body)
	}
	var uid, by int64
	_ = p.st.DB.QueryRow(`SELECT user_id, created_by FROM records WHERE id = ?`, p.id(body, "id")).Scan(&uid, &by)
	if uid != p.a.ID || by != p.b.ID {
		t.Errorf("editor record: user_id %d created_by %d", uid, by)
	}

	// the member does not see the owner's personal settings
	share("viewer")
	code, body = p.do(p.bTok, "GET", v+"/overview", nil)
	if code != 200 {
		t.Fatalf("overview: %d %s", code, body)
	}
	if strings.Contains(body, "owner-firefly-token") {
		t.Errorf("overview leaks the owner's token")
	}
}

func TestSettingsArePersonal(t *testing.T) {
	p := newTwoPeople(t)
	// env integrations were seeded into the owner's own settings
	code, body := p.do(p.aTok, "GET", "/api/v1/settings", nil)
	if code != 200 || !strings.Contains(body, `"firefly_configured":true`) || !strings.Contains(body, `"notify_configured":true`) {
		t.Fatalf("owner settings: %d %s", code, body)
	}
	code, body = p.do(p.bTok, "GET", "/api/v1/settings", nil)
	if code != 200 || strings.Contains(body, "firefly.invalid") || strings.Contains(body, "apprise.invalid") || !strings.Contains(body, `"firefly_configured":false`) {
		t.Fatalf("member sees the owner's integrations: %d %s", code, body)
	}
	// the member saves their own; box-wide keys are ignored for a member
	code, body = p.do(p.bTok, "PUT", "/api/v1/settings", map[string]any{"title": "Member's box", "fx_auto": false, "home_currency": "USD", "notify_url": "http://apprise.invalid/notify/member", "dawarich_url": "http://dawarich.member"})
	if code != 200 || !strings.Contains(body, `"home_currency":"USD"`) || !strings.Contains(body, `"title":"Pocket Garage"`) {
		t.Fatalf("member save: %d %s", code, body)
	}
	code, body = p.do(p.aTok, "GET", "/api/v1/settings", nil)
	if strings.Contains(body, "member") || strings.Contains(body, `"home_currency":"USD"`) || !strings.Contains(body, `"title":"Pocket Garage"`) || !strings.Contains(body, `"fx_auto":true`) {
		t.Errorf("member's save reached the owner: %s", body)
	}
	// the admin changes the box title; the owner's currency is theirs alone
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"title": "Garage", "home_currency": "BAM"}); code != 200 {
		t.Fatalf("owner save: %d %s", code, body)
	}
	_, body = p.do(p.bTok, "GET", "/api/v1/settings", nil)
	if !strings.Contains(body, `"title":"Garage"`) || !strings.Contains(body, `"home_currency":"USD"`) {
		t.Errorf("member after owner save: %s", body)
	}
	// box-wide tools are the admin's
	if code, _ := p.do(p.bTok, "GET", "/api/v1/backup", nil); code != http.StatusForbidden {
		t.Errorf("member downloaded the database: %d", code)
	}
	if _, body := p.do(p.bTok, "GET", "/api/v1/settings/system", nil); strings.Contains(body, "counts") || strings.Contains(body, "db_path") {
		t.Errorf("member sees box internals: %s", body)
	}
	// each person's integration test and reminder run use their own settings
	if code, body := p.do(p.bTok, "POST", "/api/v1/integrations/test", map[string]any{"which": "firefly"}); code != 200 || !strings.Contains(body, `"ok":false`) {
		t.Errorf("member firefly test: %d %s", code, body)
	}
}

func TestBackgroundJobsStayPerPerson(t *testing.T) {
	var sent []string
	notifySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sent = append(sent, r.URL.Path+" "+string(b))
	}))
	defer notifySrv.Close()
	p := newTwoPeople(t)
	ctx := context.Background()
	g := p.seedOwner()
	_ = g
	// the member has a vehicle with an overdue reminder and their own target
	code, body := p.do(p.bTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Fiat", "model": "Panda"})
	if code != http.StatusCreated {
		t.Fatal(body)
	}
	bv := p.id(body, "id")
	if code, body := p.do(p.bTok, "POST", fmt.Sprintf("/api/v1/vehicles/%d/reminders", bv), map[string]any{"title": "Member tyres", "due_date": "2020-01-01"}); code != http.StatusCreated {
		t.Fatal(body)
	}
	if code, body := p.do(p.bTok, "PUT", "/api/v1/settings", map[string]any{"notify_url": notifySrv.URL + "/member", "notify_format": "json"}); code != 200 {
		t.Fatal(body)
	}
	// the owner's notification target points at the same fake server under /owner
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"notify_url": notifySrv.URL + "/owner", "notify_format": "json"}); code != 200 {
		t.Fatal(body)
	}
	if code, body := p.do(p.aTok, "PUT", fmt.Sprintf("/api/v1/vehicles/%d/reminders/%d", g.vehicle, g.reminder), map[string]any{"due_date": "2020-01-01"}); code != 200 {
		t.Fatal(body)
	}
	n, err := p.s.checkReminders(ctx, p.b.ID, true)
	if err != nil || n != 1 {
		t.Fatalf("member check: %d %v", n, err)
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "/member ") || strings.Contains(sent[0], "Owner") || !strings.Contains(sent[0], "Member tyres") {
		t.Fatalf("member notifications: %v", sent)
	}
	sent = nil
	if n, err := p.s.checkReminders(ctx, p.a.ID, true); err != nil || n != 1 {
		t.Fatalf("owner check: %d %v", n, err)
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "/owner ") || strings.Contains(sent[0], "Member") {
		t.Fatalf("owner notifications: %v", sent)
	}
	// a person without a target gets nothing, and nobody else's target is used
	if code, body := p.do(p.bTok, "PUT", "/api/v1/settings", map[string]any{"notify_url": ""}); code != 200 {
		t.Fatal(body)
	}
	sent = nil
	if _, err := p.s.checkReminders(ctx, p.b.ID, true); err == nil || len(sent) != 0 {
		t.Errorf("member without target: err %v sent %v", err, sent)
	}
	// the vehicle filter used by background jobs never returns everyone's
	if vs, _ := p.st.ListVehicles(ctx, store.VehicleFilter{}); len(vs) != 0 {
		t.Errorf("empty filter returned %d vehicles", len(vs))
	}
	if vs, _ := p.st.ListVehicles(ctx, store.VehicleFilter{OwnerID: p.b.ID}); len(vs) != 1 || vs[0].ID != bv {
		t.Errorf("owner filter: %+v", vs)
	}
}

func TestAutoCreatedAccountsAreMembers(t *testing.T) {
	// on a box that has its owner, anyone arriving through Core / OIDC / the proxy is a member
	p := newTwoPeople(t)
	u, err := p.s.firstOrSystemUser(context.Background(), "core-admin@example.com", "Core admin")
	if err != nil || u.Role != "member" {
		t.Fatalf("auto-created on an owned box: %+v %v", u, err)
	}
	// on an empty box only the very first account becomes admin
	dir := t.TempDir()
	cfg := testConfig(dir)
	cfg.AuthMode = "proxy"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(cfg, st, log, nil)
	ts := httptest.NewServer(s.Router())
	defer ts.Close()
	me := func(email string) string {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/auth/me", nil)
		req.Header.Set("Remote-User", email)
		req.Header.Set("Remote-Email", email)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var u store.User
		_ = json.NewDecoder(res.Body).Decode(&u)
		return u.Role
	}
	if r := me("first@example.com"); r != "admin" {
		t.Errorf("first account: %s", r)
	}
	if r := me("second@example.com"); r != "member" {
		t.Errorf("second account: %s", r)
	}
	if r := me("first@example.com"); r != "admin" {
		t.Errorf("first account again: %s", r)
	}
}
