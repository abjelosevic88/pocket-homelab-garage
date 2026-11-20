package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// moveGarage is the two-person box behind a fake Core whose
// pcx_<owner|member>_<export|import|day> tokens are Pocket Move links.
func moveGarage(t *testing.T) *twoPeople {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			_, _ = w.Write([]byte(`{"active":false}`))
			return
		}
		email := parts[0] + "@example.com"
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "day": "vehicles:read"}[parts[1]]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true, "kind": "delegated", "scope": scope, "audience": "garage",
			"user": map[string]any{"id": 9, "sub": email, "email": email, "name": parts[0], "role": "member"},
			"app":  map[string]any{"id": 5, "slug": "move", "name": "Pocket Move"},
		})
	}))
	t.Cleanup(srv.Close)
	return newTwoPeople(t, func(s *Server) { s.core = core.New(srv.URL, core.Options{DB: s.store.DB, Token: "pca_garage"}) })
}

func TestMoveLinkScopes(t *testing.T) {
	p := moveGarage(t)
	g := p.seedOwner()
	ov := fmt.Sprintf("/api/v1/vehicles/%d", g.vehicle)
	exp, imp, day := "pcx_member_export", "pcx_member_import", "pcx_member_day"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/vehicles", 403},
		{exp, "GET", ov + "/export/vehicle.json", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "POST", "/api/v1/vehicles", 403},
		{imp, "POST", "/api/v1/import/vehicle", 403},
		{imp, "POST", "/api/v1/import/lubelogger", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{day, "GET", core.PortablePath, 403},
		{day, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := p.do(c.tok, c.method, c.path, map[string]any{}); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := p.do(exp, "GET", core.PortablePath, nil)
	var pt core.Portable
	if err := json.Unmarshal([]byte(body), &pt); err != nil || len(pt.Exports) != 1 || len(pt.Imports) != 1 || pt.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings the owner's vehicles to the member: they arrive once with
// everything in them, a second run adds nothing, and the export holds only
// the person's own vehicles.
func TestMoveImportsAddOnce(t *testing.T) {
	p := moveGarage(t)
	p.seedOwner()
	code, export := p.do("pcx_owner_export", "GET", "/api/v1/export.json", nil)
	if code != 200 || !strings.Contains(export, "Owner secret repair") {
		t.Fatalf("owner's export: %d %s", code, export)
	}
	if _, mine := p.do("pcx_member_export", "GET", "/api/v1/export.json", nil); strings.Contains(mine, "Owner secret repair") || strings.Contains(mine, "Fabia") {
		t.Fatalf("member's export holds the owner's car: %s", mine)
	}
	var doc map[string]any
	_ = json.Unmarshal([]byte(export), &doc)
	imp := func() map[string]any {
		code, out := p.do("pcx_member_import", "POST", "/api/v1/import", doc)
		if code != 200 {
			t.Fatalf("import → %d %s", code, out)
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(out), &m)
		return m
	}
	first := imp()
	if first["vehicles"] != 1.0 || first["records"].(float64) < 1 || first["fuel"].(float64) < 1 {
		t.Fatalf("first import: %v", first)
	}
	second := imp()
	if second["vehicles"] != 0.0 || second["skipped"] != 1.0 || second["records"] != 0.0 {
		t.Fatalf("second import added: %v", second)
	}
	_, list := p.do(p.bTok, "GET", "/api/v1/vehicles?bare=1", nil)
	var vs []map[string]any
	_ = json.Unmarshal([]byte(list), &vs)
	if len(vs) != 1 || vs[0]["owner_id"] != float64(p.b.ID) {
		t.Fatalf("member's vehicles: %s", list)
	}
	_, list = p.do(p.aTok, "GET", "/api/v1/vehicles?bare=1", nil)
	_ = json.Unmarshal([]byte(list), &vs)
	if len(vs) != 1 || vs[0]["owner_id"] != float64(p.a.ID) {
		t.Fatalf("owner's vehicles changed: %s", list)
	}
	// one vehicle's backup goes in through the same door, also only once
	_, one := p.do(p.aTok, "GET", fmt.Sprintf("/api/v1/vehicles/%d/export/vehicle.json", int64(vs[0]["id"].(float64))), nil)
	var single map[string]any
	_ = json.Unmarshal([]byte(one), &single)
	if code, out := p.do("pcx_member_import", "POST", "/api/v1/import", single); code != 200 || !strings.Contains(out, `"skipped":1`) {
		t.Fatalf("single vehicle import: %d %s", code, out)
	}
}
