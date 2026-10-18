package server

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

func sortReminders(rs []store.Reminder) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if store.UrgencyRank[a.Urgency] != store.UrgencyRank[b.Urgency] {
			return store.UrgencyRank[a.Urgency] < store.UrgencyRank[b.Urgency]
		}
		da, db := 1<<30, 1<<30
		if a.DaysLeft != nil {
			da = *a.DaysLeft
		}
		if b.DaysLeft != nil {
			db = *b.DaysLeft
		}
		if da != db {
			return da < db
		}
		ka, kb := 1e18, 1e18
		if a.KmLeft != nil {
			ka = *a.KmLeft
		}
		if b.KmLeft != nil {
			kb = *b.KmLeft
		}
		return ka < kb
	})
}

func addDays(iso string, n int) string {
	t, err := store.ParseDate(iso)
	if err != nil {
		t = time.Now()
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func addMonths(iso string, n int) string {
	t, err := store.ParseDate(iso)
	if err != nil {
		t = time.Now()
	}
	return t.AddDate(0, n, 0).Format("2006-01-02")
}

// ---- reminders ----

func (s *Server) remindersWithUrgency(r *http.Request, v *store.Vehicle, status string) ([]store.Reminder, error) {
	rs, err := s.store.ListReminders(r.Context(), v.ID, status)
	if err != nil {
		return nil, err
	}
	reading, date := s.store.LatestOdometer(r.Context(), v.ID)
	est := reading + s.store.DawarichKmSince(r.Context(), v.ID, date)
	today := store.Today()
	for i := range rs {
		store.ComputeUrgency(&rs[i], today, est)
	}
	sortReminders(rs)
	return rs, nil
}

func (s *Server) handleListReminders(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	rs, err := s.remindersWithUrgency(r, v, r.URL.Query().Get("status"))
	if err != nil {
		s.fail(w, err, "reminders")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

// handleAllReminders lists active reminders across every vehicle the user can see.
func (s *Server) handleAllReminders(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	vs, err := s.visibleVehicles(r.Context(), u, "")
	if err != nil {
		s.fail(w, err, "vehicles")
		return
	}
	out := []store.Reminder{}
	for i := range vs {
		if vs[i].Status == "archived" {
			continue
		}
		rs, err := s.remindersWithUrgency(r, &vs[i], "active")
		if err != nil {
			continue
		}
		for j := range rs {
			rs[j].VehicleName = vs[i].Title()
		}
		out = append(out, rs...)
	}
	sortReminders(out)
	writeJSON(w, http.StatusOK, out)
}

type reminderInput struct {
	Title        *string  `json:"title"`
	Notes        *string  `json:"notes"`
	Kind         *string  `json:"kind"`
	Category     *string  `json:"category"`
	DueDate      *string  `json:"due_date"`
	DueOdometer  *float64 `json:"due_odometer"`
	IntervalDays *int     `json:"interval_days"`
	IntervalKm   *float64 `json:"interval_km"`
	SoonDays     *int     `json:"soon_days"`
	SoonKm       *float64 `json:"soon_km"`
	Status       *string  `json:"status"`
}

func applyReminder(rm *store.Reminder, in reminderInput, keys map[string]bool) string {
	str := func(k string, dst *string, src *string) {
		if keys[k] && src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	str("title", &rm.Title, in.Title)
	str("notes", &rm.Notes, in.Notes)
	str("kind", &rm.Kind, in.Kind)
	str("category", &rm.Category, in.Category)
	str("due_date", &rm.DueDate, in.DueDate)
	str("status", &rm.Status, in.Status)
	if keys["due_odometer"] {
		if in.DueOdometer != nil && *in.DueOdometer > 0 {
			o := *in.DueOdometer
			rm.DueOdometer = &o
		} else {
			rm.DueOdometer = nil
		}
	}
	if keys["interval_days"] && in.IntervalDays != nil {
		rm.IntervalDays = *in.IntervalDays
	}
	if keys["interval_km"] && in.IntervalKm != nil {
		rm.IntervalKm = *in.IntervalKm
	}
	if keys["soon_days"] && in.SoonDays != nil {
		rm.SoonDays = *in.SoonDays
	}
	if keys["soon_km"] && in.SoonKm != nil {
		rm.SoonKm = *in.SoonKm
	}
	if rm.Title == "" {
		return "a title is required"
	}
	if rm.DueDate == "" && rm.DueOdometer == nil {
		return "a due date or a due odometer is required"
	}
	if !validDate(rm.DueDate) {
		return "date must be YYYY-MM-DD"
	}
	if rm.Kind == "" {
		rm.Kind = "service"
	}
	if rm.Kind != "none" && !store.ValidRecordKind(rm.Kind) {
		return "kind must be service, repair, upgrade, expense or none"
	}
	if rm.Status == "" {
		rm.Status = "active"
	}
	if rm.IntervalDays < 0 || rm.IntervalKm < 0 || rm.SoonDays < 0 || rm.SoonKm < 0 {
		return "intervals cannot be negative"
	}
	if rm.SoonDays == 0 {
		rm.SoonDays = 14
	}
	if rm.SoonKm == 0 {
		rm.SoonKm = 500
	}
	return ""
}

func (s *Server) handleCreateReminder(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in reminderInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	rm := &store.Reminder{VehicleID: v.ID}
	if msg := applyReminder(rm, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateReminder(r.Context(), rm); err != nil {
		s.fail(w, err, "create reminder")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "added reminder", rm.Title)
	s.finishReminder(r, v, rm)
	writeJSON(w, http.StatusCreated, rm)
}

func (s *Server) finishReminder(r *http.Request, v *store.Vehicle, rm *store.Reminder) {
	reading, date := s.store.LatestOdometer(r.Context(), v.ID)
	store.ComputeUrgency(rm, store.Today(), reading+s.store.DawarichKmSince(r.Context(), v.ID, date))
}

func (s *Server) handleUpdateReminder(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rm, err := s.store.GetReminder(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "reminder")
		return
	}
	var in reminderInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := applyReminder(rm, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if keys["due_date"] || keys["due_odometer"] {
		rm.NotifiedLevel = ""
	}
	if err := s.store.UpdateReminder(r.Context(), rm); err != nil {
		s.fail(w, err, "update reminder")
		return
	}
	s.finishReminder(r, v, rm)
	writeJSON(w, http.StatusOK, rm)
}

func (s *Server) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	if err := s.store.DeleteReminder(r.Context(), v.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete reminder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReminderDone marks a reminder done: recurring ones roll forward, and
// optionally a record is logged with the given cost.
func (s *Server) handleReminderDone(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rm, err := s.store.GetReminder(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "reminder")
		return
	}
	var in struct {
		Date      string   `json:"date"`
		Odometer  *float64 `json:"odometer"`
		LogRecord bool     `json:"log_record"`
		Cost      int64    `json:"cost"`
		Currency  string   `json:"currency"`
		Vendor    string   `json:"vendor"`
		Notes     string   `json:"notes"`
	}
	_ = decode(r, &in)
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if in.Odometer != nil && *in.Odometer <= 0 {
		in.Odometer = nil
	}
	uid := userFrom(r.Context()).ID
	var rec *store.Record
	if in.LogRecord && rm.Kind != "none" {
		rec = &store.Record{VehicleID: v.ID, Kind: rm.Kind, Category: rm.Category, Date: in.Date, Odometer: in.Odometer, Description: rm.Title, Notes: strings.TrimSpace(in.Notes), Vendor: strings.TrimSpace(in.Vendor), Cost: in.Cost, Currency: strings.ToUpper(in.Currency), Source: "reminder", CreatedBy: &uid, Parts: []store.Part{}}
		if rec.Currency == "" {
			rec.Currency = v.Currency
		}
		rec.FXRate, _ = s.rate(r.Context(), rec.Currency, v.Currency, rec.Date)
		rec.CostHome = convert(rec.Cost, rec.Currency, v.Currency, rec.FXRate)
		if err := s.store.CreateRecord(r.Context(), rec); err != nil {
			s.fail(w, err, "log record")
			return
		}
		s.store.LogActivity(r.Context(), v.ID, uid, "added "+rec.Kind, rec.Description)
	}
	store.RollForward(rm, in.Date, in.Odometer)
	if err := s.store.UpdateReminder(r.Context(), rm); err != nil {
		s.fail(w, err, "update reminder")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, uid, "reminder done", rm.Title)
	s.finishReminder(r, v, rm)
	writeJSON(w, http.StatusOK, map[string]any{"reminder": rm, "record": rec})
}

func (s *Server) handleReminderSnooze(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rm, err := s.store.GetReminder(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "reminder")
		return
	}
	var in struct {
		Days int     `json:"days"`
		Km   float64 `json:"km"`
	}
	_ = decode(r, &in)
	if in.Days <= 0 && in.Km <= 0 {
		in.Days = 7
	}
	if in.Days > 0 {
		base := rm.DueDate
		if base == "" || base < store.Today() {
			base = store.Today()
		}
		rm.DueDate = addDays(base, in.Days)
	}
	if in.Km > 0 && rm.DueOdometer != nil {
		n := *rm.DueOdometer + in.Km
		rm.DueOdometer = &n
	}
	rm.NotifiedLevel = ""
	if err := s.store.UpdateReminder(r.Context(), rm); err != nil {
		s.fail(w, err, "snooze")
		return
	}
	s.finishReminder(r, v, rm)
	writeJSON(w, http.StatusOK, rm)
}

// ---- plans ----

func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListPlans(r.Context(), vehicleFrom(r.Context()).vehicle.ID)
	if err != nil {
		s.fail(w, err, "plans")
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

type planInput struct {
	Title      *string `json:"title"`
	Notes      *string `json:"notes"`
	Kind       *string `json:"kind"`
	Category   *string `json:"category"`
	Priority   *string `json:"priority"`
	Stage      *string `json:"stage"`
	Estimate   *int64  `json:"estimate"`
	TargetDate *string `json:"target_date"`
}

func applyPlan(p *store.Plan, in planInput, keys map[string]bool) string {
	str := func(k string, dst *string, src *string) {
		if keys[k] && src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	str("title", &p.Title, in.Title)
	str("notes", &p.Notes, in.Notes)
	str("kind", &p.Kind, in.Kind)
	str("category", &p.Category, in.Category)
	str("priority", &p.Priority, in.Priority)
	str("stage", &p.Stage, in.Stage)
	str("target_date", &p.TargetDate, in.TargetDate)
	if keys["estimate"] && in.Estimate != nil {
		p.Estimate = *in.Estimate
	}
	if p.Title == "" {
		return "a title is required"
	}
	if p.Kind == "" {
		p.Kind = "service"
	}
	if !store.ValidRecordKind(p.Kind) {
		return "kind must be service, repair, upgrade or expense"
	}
	if p.Priority == "" {
		p.Priority = "normal"
	}
	if p.Stage == "" {
		p.Stage = "backlog"
	}
	ok := false
	for _, st := range store.PlanStages {
		if st == p.Stage {
			ok = true
		}
	}
	if !ok {
		return "unknown stage"
	}
	if !validDate(p.TargetDate) {
		return "date must be YYYY-MM-DD"
	}
	return ""
}

func (s *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in planInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := &store.Plan{VehicleID: v.ID}
	if msg := applyPlan(p, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreatePlan(r.Context(), p); err != nil {
		s.fail(w, err, "create plan")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "planned", p.Title)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	p, err := s.store.GetPlan(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	var in planInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := applyPlan(p, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdatePlan(r.Context(), p); err != nil {
		s.fail(w, err, "update plan")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleReorderPlans(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in struct {
		Stage string  `json:"stage"`
		IDs   []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.ReorderPlans(r.Context(), v.ID, in.Stage, in.IDs); err != nil {
		s.fail(w, err, "reorder")
		return
	}
	ps, _ := s.store.ListPlans(r.Context(), v.ID)
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	if err := s.store.DeletePlan(r.Context(), vehicleFrom(r.Context()).vehicle.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete plan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePlanDone moves a card to done and logs the work as a record.
func (s *Server) handlePlanDone(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	p, err := s.store.GetPlan(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	var in struct {
		Date     string   `json:"date"`
		Odometer *float64 `json:"odometer"`
		Cost     *int64   `json:"cost"`
		Currency string   `json:"currency"`
		Vendor   string   `json:"vendor"`
		Notes    string   `json:"notes"`
	}
	_ = decode(r, &in)
	if in.Date == "" {
		in.Date = store.Today()
	}
	cost := p.Estimate
	cur := v.Currency
	if in.Cost != nil {
		cost = *in.Cost
	}
	if in.Currency != "" {
		cur = strings.ToUpper(in.Currency)
	}
	uid := userFrom(r.Context()).ID
	notes := strings.TrimSpace(in.Notes)
	if notes == "" {
		notes = p.Notes
	}
	rec := &store.Record{VehicleID: v.ID, Kind: p.Kind, Category: p.Category, Date: in.Date, Odometer: in.Odometer, Description: p.Title, Notes: notes, Vendor: strings.TrimSpace(in.Vendor), Cost: cost, Currency: cur, Source: "plan", CreatedBy: &uid, Parts: []store.Part{}}
	rec.FXRate, _ = s.rate(r.Context(), rec.Currency, v.Currency, rec.Date)
	rec.CostHome = convert(rec.Cost, rec.Currency, v.Currency, rec.FXRate)
	if err := s.store.CreateRecord(r.Context(), rec); err != nil {
		s.fail(w, err, "log record")
		return
	}
	p.Stage = "done"
	p.RecordID = &rec.ID
	_ = s.store.UpdatePlan(r.Context(), p)
	s.store.LogActivity(r.Context(), v.ID, uid, "completed plan", p.Title)
	s.afterRecordChange(r.Context(), v, rec)
	writeJSON(w, http.StatusOK, map[string]any{"plan": p, "record": rec})
}

// ---- notes ----

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	ns, err := s.store.ListNotes(r.Context(), vehicleFrom(r.Context()).vehicle.ID)
	if err != nil {
		s.fail(w, err, "notes")
		return
	}
	writeJSON(w, http.StatusOK, ns)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in store.Note
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	n := &store.Note{VehicleID: v.ID, Title: strings.TrimSpace(in.Title), Body: strings.TrimSpace(in.Body), Pinned: in.Pinned}
	if n.Title == "" && n.Body == "" {
		writeErr(w, http.StatusBadRequest, "write something first")
		return
	}
	if err := s.store.CreateNote(r.Context(), n); err != nil {
		s.fail(w, err, "create note")
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	n, err := s.store.GetNote(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "note")
		return
	}
	var in struct {
		Title  *string `json:"title"`
		Body   *string `json:"body"`
		Pinned *bool   `json:"pinned"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Title != nil {
		n.Title = strings.TrimSpace(*in.Title)
	}
	if in.Body != nil {
		n.Body = strings.TrimSpace(*in.Body)
	}
	if in.Pinned != nil {
		n.Pinned = *in.Pinned
	}
	if err := s.store.UpdateNote(r.Context(), n); err != nil {
		s.fail(w, err, "update note")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	if err := s.store.DeleteNote(r.Context(), vehicleFrom(r.Context()).vehicle.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- documents ----

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	ds, err := s.store.ListDocuments(r.Context(), vehicleFrom(r.Context()).vehicle.ID)
	if err != nil {
		s.fail(w, err, "documents")
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	b, name, ct, err := s.readUpload(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "upload failed: "+err.Error())
		return
	}
	if len(b) == 0 {
		writeErr(w, http.StatusBadRequest, "empty file")
		return
	}
	rel, err := s.saveFile(v.ID, name, b)
	if err != nil {
		s.fail(w, err, "save file")
		return
	}
	uid := userFrom(r.Context()).ID
	d := &store.Document{VehicleID: v.ID, UserID: &uid, Name: name, Filename: rel, Mime: ct, Bytes: int64(len(b)), Kind: "other"}
	if r.MultipartForm != nil {
		get := func(k string) string {
			if vs := r.MultipartForm.Value[k]; len(vs) > 0 {
				return strings.TrimSpace(vs[0])
			}
			return ""
		}
		if n := get("name"); n != "" {
			d.Name = n
		}
		if k := get("kind"); k != "" {
			d.Kind = k
		}
		d.ExpiresOn = get("expires_on")
		d.LinkKind = get("link_kind")
		if id := get("link_id"); id != "" {
			d.LinkID = atoi64(id)
		}
	}
	if d.Name == "" {
		d.Name = "document"
	}
	if !validDate(d.ExpiresOn) {
		d.ExpiresOn = ""
	}
	if err := s.store.CreateDocument(r.Context(), d); err != nil {
		s.fail(w, err, "create document")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, uid, "uploaded", d.Name)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleUpdateDocument(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	d, err := s.store.GetDocument(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "document")
		return
	}
	var in struct {
		Name      *string `json:"name"`
		Kind      *string `json:"kind"`
		ExpiresOn *string `json:"expires_on"`
		LinkKind  *string `json:"link_kind"`
		LinkID    *int64  `json:"link_id"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		d.Name = strings.TrimSpace(*in.Name)
	}
	if in.Kind != nil {
		d.Kind = *in.Kind
	}
	if in.ExpiresOn != nil && validDate(*in.ExpiresOn) {
		d.ExpiresOn = *in.ExpiresOn
	}
	if in.LinkKind != nil {
		d.LinkKind = *in.LinkKind
	}
	if in.LinkID != nil {
		d.LinkID = *in.LinkID
	}
	if err := s.store.UpdateDocument(r.Context(), d); err != nil {
		s.fail(w, err, "update document")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDocumentFile(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	d, err := s.store.GetDocument(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "document")
		return
	}
	dl := ""
	if queryBool(r, "download") {
		dl = d.Name
	}
	s.serveFile(w, r, d.Filename, d.Mime, dl)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	file, err := s.store.DeleteDocument(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "delete document")
		return
	}
	s.removeFile(file)
	w.WriteHeader(http.StatusNoContent)
}

func atoi64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
