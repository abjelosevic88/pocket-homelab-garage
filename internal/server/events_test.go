package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

type queuedEvent struct {
	Topic   string         `json:"topic"`
	Subject string         `json:"subject"`
	Payload map[string]any `json:"payload"`
}

// outboxEvents reads what Garage queued for Core (the fake Core refuses
// /api/v1/events, so nothing leaves the outbox).
func (p *twoPeople) outboxEvents(topic string) []queuedEvent {
	p.t.Helper()
	rows, err := p.st.DB.Query(`SELECT body FROM pocket_outbox`)
	if err != nil {
		p.t.Fatal(err)
	}
	defer rows.Close()
	var out []queuedEvent
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			p.t.Fatal(err)
		}
		var e queuedEvent
		if err := json.Unmarshal(b, &e); err != nil {
			p.t.Fatal(err)
		}
		if e.Topic == topic {
			out = append(out, e)
		}
	}
	return out
}

func hasEvent(evs []queuedEvent, subject string, want map[string]any) bool {
	for _, e := range evs {
		if e.Subject != subject {
			continue
		}
		ok := true
		for k, v := range want {
			if fmt.Sprint(e.Payload[k]) != fmt.Sprint(v) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// Pocket Renewals learns reminder due dates and paper end dates as they are saved.
func TestReminderAndDocumentEvents(t *testing.T) {
	coreSrv, _ := linkCore(t)
	p := newTwoPeople(t, func(s *Server) { s.core = core.New(coreSrv.URL, core.Options{DB: s.store.DB, Token: "pca_garage"}) })
	g := p.seedOwner()
	v := fmt.Sprintf("/api/v1/vehicles/%d", g.vehicle)
	rsub := fmt.Sprintf("reminder:%d", g.reminder)
	dsub := fmt.Sprintf("document:%d", g.doc)

	evs := p.outboxEvents("garage.reminder_saved")
	if !hasEvent(evs, rsub, map[string]any{"reminder_id": g.reminder, "vehicle_id": g.vehicle, "vehicle": "Skoda Fabia", "title": "Owner oil change", "due_date": "2026-10-10", "user_email": "owner@example.com"}) {
		t.Fatalf("create: %+v", evs)
	}
	if !hasEvent(p.outboxEvents("garage.document_saved"), dsub, map[string]any{"document_id": g.doc, "name": "Owner policy", "kind": "other", "expires_on": "", "user_email": "owner@example.com"}) {
		t.Fatalf("upload: %+v", p.outboxEvents("garage.document_saved"))
	}

	if code, body := p.do(p.aTok, "PUT", fmt.Sprintf("%s/reminders/%d", v, g.reminder), map[string]any{"title": "Registration", "due_date": "2027-02-01"}); code != http.StatusOK {
		t.Fatalf("update reminder: %d %s", code, body)
	}
	if !hasEvent(p.outboxEvents("garage.reminder_saved"), rsub, map[string]any{"title": "Registration", "due_date": "2027-02-01"}) {
		t.Fatalf("update: %+v", p.outboxEvents("garage.reminder_saved"))
	}

	if code, body := p.do(p.aTok, "PUT", fmt.Sprintf("%s/documents/%d", v, g.doc), map[string]any{"kind": "insurance", "expires_on": "2027-03-31"}); code != http.StatusOK {
		t.Fatalf("update document: %d %s", code, body)
	}
	if !hasEvent(p.outboxEvents("garage.document_saved"), dsub, map[string]any{"kind": "insurance", "expires_on": "2027-03-31"}) {
		t.Fatalf("update doc: %+v", p.outboxEvents("garage.document_saved"))
	}

	// a yearly inspection marked done rolls forward and is published again
	code, body := p.do(p.aTok, "POST", v+"/reminders", map[string]any{"title": "Inspection", "due_date": "2026-11-01", "interval_days": 365})
	if code != http.StatusCreated {
		t.Fatalf("create recurring: %d %s", code, body)
	}
	insp := p.id(body, "id")
	if code, body := p.do(p.aTok, "POST", fmt.Sprintf("%s/reminders/%d/done", v, insp), map[string]any{"date": "2026-11-02"}); code != http.StatusOK {
		t.Fatalf("done: %d %s", code, body)
	}
	if !hasEvent(p.outboxEvents("garage.reminder_saved"), fmt.Sprintf("reminder:%d", insp), map[string]any{"due_date": "2027-11-02"}) {
		t.Fatalf("roll forward: %+v", p.outboxEvents("garage.reminder_saved"))
	}

	// a one-off reminder marked done keeps its date: nothing new to publish
	before := len(p.outboxEvents("garage.reminder_saved"))
	if code, body := p.do(p.aTok, "POST", fmt.Sprintf("%s/reminders/%d/done", v, g.reminder), map[string]any{}); code != http.StatusOK {
		t.Fatalf("done one-off: %d %s", code, body)
	}
	if n := len(p.outboxEvents("garage.reminder_saved")); n != before {
		t.Fatalf("one-off done published %d new events", n-before)
	}
}
