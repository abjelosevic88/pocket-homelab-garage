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

// fakeMoney is a Pocket Money that knows one person's accounts and records
// every transaction call with the token it came with.
type fakeMoney struct {
	mu    sync.Mutex
	next  int64
	txs   map[int64]map[string]any
	calls []string
}

func (f *fakeMoney) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer pcx_owner@example.com") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/accounts":
			_, _ = w.Write([]byte(`[{"id":1,"name":"UniCredit","currency":"BAM","everyday":true,"role":"owner"},{"id":2,"name":"Revolut","currency":"EUR","role":"owner"},{"id":3,"name":"Shared","currency":"BAM","role":"viewer"}]`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/categories"):
			_, _ = w.Write([]byte(`[{"id":9,"name":"Plata","kind":"income"},{"id":10,"name":"Gorivo","kind":"expense"},{"id":11,"name":"Auto","kind":"expense"}]`))
		case r.Method == "POST" && r.URL.Path == "/api/v1/transactions":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.next++
			f.txs[f.next] = in
			_ = json.NewEncoder(w).Encode(map[string]any{"id": f.next, "account_id": in["account_id"], "source": in["source"]})
		case strings.HasPrefix(r.URL.Path, "/api/v1/transactions/"):
			var id int64
			fmt.Sscanf(strings.TrimPrefix(r.URL.Path, "/api/v1/transactions/"), "%d", &id)
			tx, ok := f.txs[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == "DELETE" {
				delete(f.txs, id)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			for k, v := range in {
				tx[k] = v
			}
			_ = json.NewEncoder(w).Encode(tx)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})
}

func (f *fakeMoney) tx(id float64) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.txs[int64(id)]
}

func TestPocketMoneyThroughCore(t *testing.T) {
	fm := &fakeMoney{txs: map[int64]map[string]any{}}
	moneySrv := httptest.NewServer(fm.handler(t))
	t.Cleanup(moneySrv.Close)
	var coreMu sync.Mutex
	offFor := map[string]bool{"member@example.com": true} // the member switched the link off in Core
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			_, _ = w.Write([]byte(`{"app":{}}`))
		case "/api/v1/links/token":
			var in struct {
				User   string   `json:"user"`
				App    string   `json:"app"`
				Scopes []string `json:"scopes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			coreMu.Lock()
			off := offFor[in.User]
			coreMu.Unlock()
			if off || in.App != "money" || strings.Join(in.Scopes, " ") != "accounts:read transactions:write" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"off","reason":"off_person"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "pcx_" + in.User, "expires_in": 300, "url": moneySrv.URL, "public_url": "https://money.example"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(coreSrv.Close)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(coreSrv.URL, core.Options{Token: "pca_garage"}) })

	var status map[string]any
	_, body := p.do(p.aTok, "GET", "/api/v1/money", nil)
	_ = json.Unmarshal([]byte(body), &status)
	if status["connected"] != true || status["offer"] != true || status["default_account"] != float64(1) || len(status["accounts"].([]any)) != 2 {
		t.Fatalf("owner money status %s", body)
	}

	_, body = p.do(p.aTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Skoda", "model": "Fabia", "firefly_tag": "fabia", "currency": "BAM"})
	var veh struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &veh)
	base := fmt.Sprintf("/api/v1/vehicles/%d", veh.ID)
	post := func(path string, in map[string]any) map[string]any {
		t.Helper()
		code, body := p.do(p.aTok, "POST", base+path, in)
		if code != http.StatusCreated && code != http.StatusOK {
			t.Fatalf("POST %s: %d %s", path, code, body)
		}
		var out map[string]any
		_ = json.Unmarshal([]byte(body), &out)
		return out
	}

	// Default on: a fill-up is booked on the everyday account, in the fuel category.
	fill := post("/fuel", map[string]any{"date": "2026-10-02", "odometer": 100100, "volume": 40, "cost": 9000, "station": "Hifa"})
	if fill["money_tx_id"] == nil || fill["money_error"] != nil {
		t.Fatalf("fill-up not booked: %+v", fill)
	}
	tx := fm.tx(fill["money_tx_id"].(float64))
	if tx["account_id"] != float64(1) || tx["category_id"] != float64(10) || tx["amount"] != float64(9000) || tx["currency"] != "BAM" ||
		tx["source"] != "garage" || tx["external_id"] != fmt.Sprintf("fuel:%d", int64(fill["id"].(float64))) || tx["payee"] != "Hifa" ||
		!strings.Contains(tx["description"].(string), "fuel · Hifa") || fmt.Sprint(tx["tags"]) != "[fabia]" {
		t.Fatalf("fill-up transaction %+v", tx)
	}

	// Unticked: nothing goes to Money.
	if rec := post("/records", map[string]any{"kind": "expense", "description": "Parking", "cost": 300, "money": false}); rec["money_tx_id"] != nil {
		t.Fatalf("unticked record booked: %+v", rec)
	}
	// Another account: remembered for the vehicle from then on.
	rec := post("/records", map[string]any{"kind": "repair", "description": "Brake pads", "cost": 10000, "vendor": "AutoServis", "money": true, "money_account": 2})
	rtx := fm.tx(rec["money_tx_id"].(float64))
	if rtx["account_id"] != float64(2) || rtx["category_id"] != float64(11) || rtx["description"] != "2014 Skoda Fabia Brake pads" && !strings.HasSuffix(rtx["description"].(string), "Brake pads") {
		t.Fatalf("record transaction %+v", rtx)
	}
	fill2 := post("/fuel", map[string]any{"date": "2026-10-09", "odometer": 100700, "volume": 38, "cost": 8500})
	if fm.tx(fill2["money_tx_id"].(float64))["account_id"] != float64(2) {
		t.Fatalf("the vehicle's last account should be the default now")
	}

	// Edits follow; a cost of zero removes it; deleting the record deletes it.
	recID := int64(rec["id"].(float64))
	if code, body := p.do(p.aTok, "PUT", fmt.Sprintf("%s/records/%d", base, recID), map[string]any{"cost": 12000}); code != 200 || strings.Contains(body, "money_error") {
		t.Fatalf("update %d %s", code, body)
	}
	if fm.tx(rec["money_tx_id"].(float64))["amount"] != float64(12000) {
		t.Fatalf("edit not carried over: %+v", fm.tx(rec["money_tx_id"].(float64)))
	}
	fill2ID := int64(fill2["id"].(float64))
	_, body = p.do(p.aTok, "PUT", fmt.Sprintf("%s/fuel/%d", base, fill2ID), map[string]any{"cost": 0})
	if fm.tx(fill2["money_tx_id"].(float64)) != nil || strings.Contains(body, `"money_tx_id":`+fmt.Sprint(fill2["money_tx_id"])) {
		t.Fatalf("zero cost should remove and unlink: %s", body)
	}
	if code, _ := p.do(p.aTok, "DELETE", fmt.Sprintf("%s/records/%d", base, recID), nil); code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	if fm.tx(rec["money_tx_id"].(float64)) != nil {
		t.Fatalf("deleted record left its transaction")
	}
	// "→ Money" books an existing record.
	park := post("/records", map[string]any{"kind": "expense", "description": "Toll", "cost": 500, "money": false})
	booked := post(fmt.Sprintf("/records/%d/money", int64(park["id"].(float64))), map[string]any{})
	if booked["money_tx_id"] == nil {
		t.Fatalf("→ Money: %+v", booked)
	}

	// Switched off in Garage's settings: not offered, not booked.
	p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"money_enabled": false})
	_, body = p.do(p.aTok, "GET", "/api/v1/money", nil)
	_ = json.Unmarshal([]byte(body), &status)
	if status["offer"] != false || status["connected"] != true {
		t.Fatalf("status with the setting off %s", body)
	}
	if f := post("/fuel", map[string]any{"odometer": 101000, "volume": 30, "cost": 7000}); f["money_tx_id"] != nil {
		t.Fatalf("booked although switched off: %+v", f)
	}

	// The member switched the link off in Core: Garage says why, the fill-up is still saved.
	_, body = p.do(p.bTok, "GET", "/api/v1/money", nil)
	_ = json.Unmarshal([]byte(body), &status)
	if status["connected"] != false || !strings.Contains(status["error"].(string), "switched the Garage → Money link off") {
		t.Fatalf("member status %s", body)
	}
	_, body = p.do(p.bTok, "POST", "/api/v1/vehicles", map[string]any{"make": "Fiat", "model": "Panda"})
	_ = json.Unmarshal([]byte(body), &veh)
	code, body := p.do(p.bTok, "POST", fmt.Sprintf("/api/v1/vehicles/%d/fuel", veh.ID), map[string]any{"odometer": 5000, "volume": 20, "cost": 4000, "money": true})
	if code != http.StatusCreated || !strings.Contains(body, `"money_error":"you switched`) {
		t.Fatalf("member fill-up %d %s", code, body)
	}
	fm.mu.Lock()
	defer fm.mu.Unlock()
	for _, c := range fm.calls {
		if strings.Contains(c, "member") {
			t.Fatalf("member reached Money: %v", fm.calls)
		}
	}
}
