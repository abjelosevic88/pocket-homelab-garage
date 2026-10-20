package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// modesCore is a fake Pocket Core that takes registrations and keeps every
// PUT /api/v1/links/modes it receives.
type modesCore struct {
	mu      sync.Mutex
	srv     *httptest.Server
	reports [][]core.ConnMode
}

func newModesCore(t *testing.T) *modesCore {
	t.Helper()
	m := &modesCore{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/apps/register":
			_, _ = w.Write([]byte(`{"app":{}}`))
		case r.URL.Path == "/api/v1/links/modes" && r.Method == http.MethodPut:
			if r.Header.Get("Authorization") != "Bearer pca_garage" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var in struct {
				Modes []core.ConnMode `json:"modes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			m.mu.Lock()
			m.reports = append(m.reports, in.Modes)
			m.mu.Unlock()
			_, _ = w.Write([]byte(`{"recorded":1}`))
		case r.URL.Path == "/api/v1/links/status":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// waitReport waits for report number n (1-based) and returns it as
// "user target" → mode.
func (m *modesCore) waitReport(t *testing.T, n int) map[string]string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		if len(m.reports) >= n {
			out := map[string]string{}
			for _, c := range m.reports[n-1] {
				out[c.User+" "+c.Target] = c.Mode
			}
			m.mu.Unlock()
			return out
		}
		m.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no report #%d from Garage", n)
	return nil
}

func (m *modesCore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.reports)
}

func TestConnectionModesReportedToCore(t *testing.T) {
	c := newModesCore(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(c.srv.URL, core.Options{Token: "pca_garage"}) })

	// Right after registering: everyone, every target, the mode in effect
	// (the owner's Dawarich key from the environment means Direct; nothing
	// chosen and no token means Pocket Core).
	want := map[string]string{
		"owner@example.com money": "core", "owner@example.com travel": "core", "owner@example.com inventory": "core", "owner@example.com service:dawarich": "direct",
		"member@example.com money": "core", "member@example.com travel": "core", "member@example.com inventory": "core", "member@example.com service:dawarich": "core",
	}
	got := c.waitReport(t, 1)
	if len(got) != len(want) {
		t.Fatalf("first report %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("first report %s = %q, want %q (%v)", k, got[k], v, got)
		}
	}

	// A saved token with no mode chosen is Direct.
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"travel_url": "http://travel.invalid", "travel_token": "pt_owner"}); code != 200 {
		t.Fatalf("save token %d %s", code, body)
	}
	got = c.waitReport(t, 2)
	if got["owner@example.com travel"] != "direct" || got["owner@example.com money"] != "core" || got["member@example.com travel"] != "core" {
		t.Fatalf("after token %v", got)
	}

	// A person switching a connection off.
	if code, body := p.do(p.bTok, "PUT", "/api/v1/settings", map[string]any{"money_mode": "off"}); code != 200 {
		t.Fatalf("mode off %d %s", code, body)
	}
	got = c.waitReport(t, 3)
	if got["member@example.com money"] != "off" || got["owner@example.com travel"] != "direct" {
		t.Fatalf("after off %v", got)
	}

	// Disconnecting removes the token: back to Pocket Core.
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"travel_url": "", "travel_token": ""}); code != 200 {
		t.Fatalf("disconnect %d %s", code, body)
	}
	got = c.waitReport(t, 4)
	if got["owner@example.com travel"] != "core" {
		t.Fatalf("after disconnect %v", got)
	}

	// Saving something unrelated sends nothing.
	n := c.count()
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"home_currency": "EUR"}); code != 200 {
		t.Fatalf("currency %d %s", code, body)
	}
	time.Sleep(100 * time.Millisecond)
	if c.count() != n {
		t.Fatalf("a currency change should not report modes")
	}
}

// Without Pocket Core nothing is reported (and nothing breaks).
func TestConnectionModesWithoutCore(t *testing.T) {
	p := newTwoPeople(t)
	p.s.modesChanged()
	p.s.reportModes(t.Context())
	if code, body := p.do(p.aTok, "PUT", "/api/v1/settings", map[string]any{"money_mode": "off"}); code != 200 {
		t.Fatalf("mode %d %s", code, body)
	}
}
