package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- records ----

type recordInput struct {
	Kind        *string      `json:"kind"`
	Category    *string      `json:"category"`
	Date        *string      `json:"date"`
	Odometer    *float64     `json:"odometer"`
	Description *string      `json:"description"`
	Notes       *string      `json:"notes"`
	Vendor      *string      `json:"vendor"`
	Cost        *int64       `json:"cost"`
	Currency    *string      `json:"currency"`
	FXRate      *float64     `json:"fx_rate"`
	Parts       []store.Part `json:"parts"`
	Tags        *string      `json:"tags"`
}

func (s *Server) applyRecord(ctx context.Context, v *store.Vehicle, rec *store.Record, in recordInput, keys map[string]bool) string {
	str := func(k string, dst *string, src *string) {
		if keys[k] && src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	str("kind", &rec.Kind, in.Kind)
	str("category", &rec.Category, in.Category)
	str("date", &rec.Date, in.Date)
	str("description", &rec.Description, in.Description)
	str("notes", &rec.Notes, in.Notes)
	str("vendor", &rec.Vendor, in.Vendor)
	str("tags", &rec.Tags, in.Tags)
	if keys["odometer"] {
		if in.Odometer != nil && *in.Odometer > 0 {
			o := *in.Odometer
			rec.Odometer = &o
		} else {
			rec.Odometer = nil
		}
	}
	if keys["cost"] && in.Cost != nil {
		rec.Cost = *in.Cost
	}
	if keys["currency"] && in.Currency != nil {
		rec.Currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
	}
	if keys["parts"] {
		rec.Parts = in.Parts
		if rec.Parts == nil {
			rec.Parts = []store.Part{}
		}
	}
	if rec.Kind == "" {
		rec.Kind = "service"
	}
	if !store.ValidRecordKind(rec.Kind) {
		return "kind must be service, repair, upgrade or expense"
	}
	if rec.Description == "" {
		return "a description is required"
	}
	if rec.Date == "" {
		rec.Date = store.Today()
	}
	if !validDate(rec.Date) || len(rec.Date) != 10 {
		return "date must be YYYY-MM-DD"
	}
	if rec.Cost < 0 {
		return "cost cannot be negative"
	}
	if rec.Currency == "" {
		rec.Currency = v.Currency
	}
	if !currency.Valid(rec.Currency) {
		return "unknown currency"
	}
	if keys["fx_rate"] && in.FXRate != nil && *in.FXRate > 0 {
		rec.FXRate = *in.FXRate
	} else if keys["cost"] || keys["currency"] || keys["date"] || rec.FXRate == 0 {
		rec.FXRate, _ = s.rate(ctx, rec.Currency, v.Currency, rec.Date)
	}
	rec.CostHome = currency.Convert(rec.Cost, rec.Currency, v.Currency, rec.FXRate)
	return ""
}

func (s *Server) handleListRecords(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	q := r.URL.Query()
	f := store.RecordFilter{Kind: q.Get("kind"), Query: strings.TrimSpace(q.Get("q")), From: q.Get("from"), To: q.Get("to"), Limit: queryInt(r, "limit")}
	rs, err := s.store.ListRecords(r.Context(), v.ID, f)
	if err != nil {
		s.fail(w, err, "list records")
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) handleCreateRecord(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in recordInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := userFrom(r.Context()).ID
	rec := &store.Record{VehicleID: v.ID, Currency: v.Currency, Source: "manual", CreatedBy: &uid, Parts: []store.Part{}}
	if msg := s.applyRecord(r.Context(), v, rec, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateRecord(r.Context(), rec); err != nil {
		s.fail(w, err, "create record")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, uid, "added "+rec.Kind, rec.Description)
	s.afterRecordChange(r.Context(), v, rec)
	writeJSON(w, http.StatusCreated, rec)
}

// afterRecordChange auto-completes reminders of the same category when a
// service is logged at or after their due point.
func (s *Server) afterRecordChange(ctx context.Context, v *store.Vehicle, rec *store.Record) {
	if rec.Kind == "expense" || rec.Category == "" {
		return
	}
	rems, err := s.store.ListReminders(ctx, v.ID, "active")
	if err != nil {
		return
	}
	for i := range rems {
		rm := &rems[i]
		if !strings.EqualFold(rm.Category, rec.Category) || rm.Kind != rec.Kind {
			continue
		}
		// only when the work is clearly the one the reminder waited for: due within 60 days / 2000 km of the record
		close := false
		if rm.DueDate != "" && store.DaysBetween(rec.Date, rm.DueDate) <= 60 {
			close = true
		}
		if rm.DueOdometer != nil && rec.Odometer != nil && *rm.DueOdometer-*rec.Odometer <= 2000 {
			close = true
		}
		if !close {
			continue
		}
		store.RollForward(rm, rec.Date, rec.Odometer)
		_ = s.store.UpdateReminder(ctx, rm)
		s.store.LogActivity(ctx, v.ID, 0, "reminder rolled forward", rm.Title)
	}
}

func (s *Server) handleGetRecord(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	rec, err := s.store.GetRecord(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "record")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleUpdateRecord(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rec, err := s.store.GetRecord(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "record")
		return
	}
	var in recordInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := s.applyRecord(r.Context(), v, rec, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateRecord(r.Context(), rec); err != nil {
		s.fail(w, err, "update record")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "updated "+rec.Kind, rec.Description)
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleDeleteRecord(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rec, err := s.store.GetRecord(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "record")
		return
	}
	if err := s.store.DeleteRecord(r.Context(), v.ID, rec.ID); err != nil {
		s.fail(w, err, "delete record")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "deleted "+rec.Kind, rec.Description)
	w.WriteHeader(http.StatusNoContent)
}

// ---- fuel ----

type fuelInput struct {
	Date     *string  `json:"date"`
	Odometer *float64 `json:"odometer"`
	Volume   *float64 `json:"volume"`
	Cost     *int64   `json:"cost"`
	Currency *string  `json:"currency"`
	FXRate   *float64 `json:"fx_rate"`
	FullTank *bool    `json:"full_tank"`
	Missed   *bool    `json:"missed"`
	Station  *string  `json:"station"`
	Grade    *string  `json:"grade"`
	SocStart *float64 `json:"soc_start"`
	SocEnd   *float64 `json:"soc_end"`
	Lat      *float64 `json:"lat"`
	Lng      *float64 `json:"lng"`
	Notes    *string  `json:"notes"`
	Tags     *string  `json:"tags"`
	// convenience: price per unit instead of total cost
	PricePer *float64 `json:"price_per"`
}

func (s *Server) applyFuel(ctx context.Context, v *store.Vehicle, f *store.Fuel, in fuelInput, keys map[string]bool) string {
	str := func(k string, dst *string, src *string) {
		if keys[k] && src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	str("date", &f.Date, in.Date)
	str("station", &f.Station, in.Station)
	str("grade", &f.Grade, in.Grade)
	str("notes", &f.Notes, in.Notes)
	str("tags", &f.Tags, in.Tags)
	if keys["odometer"] && in.Odometer != nil {
		f.Odometer = *in.Odometer
	}
	if keys["volume"] && in.Volume != nil {
		f.Volume = *in.Volume
	}
	if keys["cost"] && in.Cost != nil {
		f.Cost = *in.Cost
	}
	if keys["price_per"] && in.PricePer != nil && !keys["cost"] && f.Volume > 0 {
		f.Cost = int64(*in.PricePer*f.Volume + 0.5)
	}
	if keys["currency"] && in.Currency != nil {
		f.Currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
	}
	if keys["full_tank"] && in.FullTank != nil {
		f.FullTank = *in.FullTank
	}
	if keys["missed"] && in.Missed != nil {
		f.Missed = *in.Missed
	}
	if keys["soc_start"] && in.SocStart != nil {
		f.SocStart = *in.SocStart
	}
	if keys["soc_end"] && in.SocEnd != nil {
		f.SocEnd = *in.SocEnd
	}
	if keys["lat"] {
		f.Lat = in.Lat
	}
	if keys["lng"] {
		f.Lng = in.Lng
	}
	if f.Date == "" {
		f.Date = store.Today()
	}
	if !validDate(f.Date) || len(f.Date) != 10 {
		return "date must be YYYY-MM-DD"
	}
	if f.Odometer < 0 {
		return "odometer cannot be negative"
	}
	if f.Volume <= 0 {
		return "volume is required"
	}
	if f.Cost < 0 {
		return "cost cannot be negative"
	}
	if f.Currency == "" {
		f.Currency = v.Currency
	}
	if !currency.Valid(f.Currency) {
		return "unknown currency"
	}
	if keys["fx_rate"] && in.FXRate != nil && *in.FXRate > 0 {
		f.FXRate = *in.FXRate
	} else if keys["cost"] || keys["currency"] || keys["date"] || f.FXRate == 0 {
		f.FXRate, _ = s.rate(ctx, f.Currency, v.Currency, f.Date)
	}
	f.CostHome = currency.Convert(f.Cost, f.Currency, v.Currency, f.FXRate)
	return ""
}

func (s *Server) handleListFuel(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	fs, err := s.store.ListFuel(r.Context(), v.ID)
	if err != nil {
		s.fail(w, err, "list fuel")
		return
	}
	if q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))); q != "" {
		var out []store.Fuel
		for _, f := range fs {
			if strings.Contains(strings.ToLower(f.Station+" "+f.Notes+" "+f.Grade+" "+f.Tags), q) {
				out = append(out, f)
			}
		}
		fs = out
		if fs == nil {
			fs = []store.Fuel{}
		}
	}
	writeJSON(w, http.StatusOK, fs)
}

func (s *Server) handleCreateFuel(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in fuelInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := userFrom(r.Context()).ID
	f := &store.Fuel{VehicleID: v.ID, Currency: v.Currency, FullTank: true, Source: "manual", CreatedBy: &uid}
	if msg := s.applyFuel(r.Context(), v, f, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateFuel(r.Context(), f); err != nil {
		s.fail(w, err, "create fuel")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, uid, "added fill-up", f.Station)
	writeJSON(w, http.StatusCreated, s.fuelWithStats(r.Context(), v.ID, f.ID))
}

// fuelWithStats re-reads one fill with its derived stretch figures.
func (s *Server) fuelWithStats(ctx context.Context, vehicleID, id int64) *store.Fuel {
	fs, _ := s.store.ListFuel(ctx, vehicleID)
	for i := range fs {
		if fs[i].ID == id {
			return &fs[i]
		}
	}
	f, _ := s.store.GetFuel(ctx, vehicleID, id)
	return f
}

func (s *Server) handleGetFuel(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	f := s.fuelWithStats(r.Context(), v.ID, idParam(r, "id"))
	if f == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleUpdateFuel(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	f, err := s.store.GetFuel(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "fuel")
		return
	}
	var in fuelInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := s.applyFuel(r.Context(), v, f, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateFuel(r.Context(), f); err != nil {
		s.fail(w, err, "update fuel")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "updated fill-up", f.Station)
	writeJSON(w, http.StatusOK, s.fuelWithStats(r.Context(), v.ID, f.ID))
}

func (s *Server) handleDeleteFuel(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	if err := s.store.DeleteFuel(r.Context(), v.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete fuel")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "deleted fill-up", "")
	w.WriteHeader(http.StatusNoContent)
}

// ---- odometer ----

func (s *Server) handleOdometerHistory(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	h, err := s.store.OdometerHistory(r.Context(), v.ID)
	if err != nil {
		s.fail(w, err, "odometer")
		return
	}
	reading, date := s.store.LatestOdometer(r.Context(), v.ID)
	est := s.store.DawarichKmSince(r.Context(), v.ID, date)
	writeJSON(w, http.StatusOK, map[string]any{"history": h, "latest": reading, "latest_date": date, "estimate_km": est, "estimate": reading + est})
}

type odometerInput struct {
	Date    string  `json:"date"`
	Reading float64 `json:"reading"`
	Notes   string  `json:"notes"`
}

func (s *Server) handleCreateOdometer(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in odometerInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) || in.Reading <= 0 {
		writeErr(w, http.StatusBadRequest, "a date and a positive reading are required")
		return
	}
	o := &store.OdometerReading{VehicleID: v.ID, Date: in.Date, Reading: in.Reading, Notes: strings.TrimSpace(in.Notes), Source: "manual"}
	if err := s.store.CreateOdometer(r.Context(), o); err != nil {
		s.fail(w, err, "create odometer")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "odometer reading", "")
	writeJSON(w, http.StatusCreated, o)
}

// handleConfirmOdometer turns today's Dawarich estimate into a real reading.
func (s *Server) handleConfirmOdometer(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	reading, date := s.store.LatestOdometer(r.Context(), v.ID)
	est := s.store.DawarichKmSince(r.Context(), v.ID, date)
	if est <= 0 {
		writeErr(w, http.StatusBadRequest, "nothing to confirm: no recorded kilometres since the last reading")
		return
	}
	o := &store.OdometerReading{VehicleID: v.ID, Date: store.Today(), Reading: float64(int64(reading + est + 0.5)), Notes: "Confirmed Dawarich estimate", Source: "dawarich"}
	if err := s.store.CreateOdometer(r.Context(), o); err != nil {
		s.fail(w, err, "confirm")
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (s *Server) handleUpdateOdometer(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	o, err := s.store.GetOdometer(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "odometer")
		return
	}
	var in odometerInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Date != "" {
		o.Date = in.Date
	}
	if in.Reading > 0 {
		o.Reading = in.Reading
	}
	o.Notes = strings.TrimSpace(in.Notes)
	if !validDate(o.Date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if err := s.store.UpdateOdometer(r.Context(), o); err != nil {
		s.fail(w, err, "update odometer")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleDeleteOdometer(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	if err := s.store.DeleteOdometer(r.Context(), v.ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete odometer")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
