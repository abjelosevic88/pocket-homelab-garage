package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/lubelogger"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// LubeLogger-compatible API.
//
// The Pocket Homelab phone app and the Raycast extension talk LubeLogger's
// API (form-encoded or JSON writes, `{success, message, additionalData}`
// answers, culture-sensitive numbers). These routes answer the same way so
// those clients only need a new base URL. Numbers are accepted with either
// decimal separator; dates come back as ISO, which every client parses.

func (s *Server) compatRoutes(r chi.Router) {
	r.Route("/api", func(r chi.Router) {
		r.Use(s.compatAuth)
		r.Get("/vehicles", s.compatVehicles)
		r.Post("/vehicles/add", s.compatAddVehicle)
		r.Get("/vehicle/info", s.compatInfo)
		r.Get("/vehicle/reminders", s.compatReminders)
		r.Get("/vehicle/planrecords", s.compatPlans)
		r.Get("/vehicle/notes", s.compatNotes)
		for _, k := range []string{"gas", "service", "repair", "upgrade", "tax", "odometer"} {
			kind := k
			r.Get("/vehicle/"+kind+"records", func(w http.ResponseWriter, r *http.Request) { s.compatList(w, r, kind) })
			r.Post("/vehicle/"+kind+"records/add", func(w http.ResponseWriter, r *http.Request) { s.compatAdd(w, r, kind) })
			r.Put("/vehicle/"+kind+"records/update", func(w http.ResponseWriter, r *http.Request) { s.compatUpdate(w, r, kind) })
			r.Post("/vehicle/"+kind+"records/update", func(w http.ResponseWriter, r *http.Request) { s.compatUpdate(w, r, kind) })
			r.Delete("/vehicle/"+kind+"records/delete", func(w http.ResponseWriter, r *http.Request) { s.compatDelete(w, r, kind) })
			r.Post("/vehicle/"+kind+"records/delete", func(w http.ResponseWriter, r *http.Request) { s.compatDelete(w, r, kind) })
			r.Get("/vehicle/"+kind+"records/delete", func(w http.ResponseWriter, r *http.Request) { s.compatDelete(w, r, kind) })
		}
	})
}

// compatAuth accepts Basic auth (email + password), a bearer/API-key token,
// or whatever AUTH_MODE allows.
func (s *Server) compatAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u *store.User
		if email, pass, ok := r.BasicAuth(); ok {
			cand, err := s.store.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(email)))
			if err == nil && cand.PasswordHash != "" && bcrypt.CompareHashAndPassword([]byte(cand.PasswordHash), []byte(pass)) == nil {
				u = cand
			} else if err == nil && s.cfg.AuthMode != "local" {
				u = cand
			}
		}
		if u == nil {
			u, _ = s.resolveUser(r)
		}
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="Pocket Garage"`)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "sign in with your Pocket Garage email and password, or an API token"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func compatOK(w http.ResponseWriter, msg string, extra map[string]any) {
	out := map[string]any{"success": true, "message": msg}
	if extra != nil {
		out["additionalData"] = extra
	}
	writeJSON(w, http.StatusOK, out)
}

func compatFail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"success": false, "message": msg})
}

// compatVehicle loads the vehicle from ?vehicleId= (or the body) and checks access.
func (s *Server) compatVehicle(w http.ResponseWriter, r *http.Request, form map[string]string) *store.Vehicle {
	idStr := r.URL.Query().Get("vehicleId")
	if idStr == "" && form != nil {
		idStr = form["vehicleId"]
	}
	id, _ := strconv.ParseInt(idStr, 10, 64)
	u := userFrom(r.Context())
	if id == 0 {
		// single-vehicle convenience: default to the first visible active vehicle
		vs, _ := s.store.ListVehicles(r.Context(), store.VehicleFilter{UserID: u.ID, Status: "active"})
		if len(vs) == 1 {
			id = vs[0].ID
		}
	}
	if s.store.VehicleRole(r.Context(), id, u) == "" {
		compatFail(w, http.StatusNotFound, "vehicle not found")
		return nil
	}
	v, err := s.store.GetVehicle(r.Context(), id, u.ID)
	if err != nil {
		compatFail(w, http.StatusNotFound, "vehicle not found")
		return nil
	}
	return v
}

// readForm accepts application/x-www-form-urlencoded, multipart or JSON bodies as a flat string map.
func readForm(r *http.Request) map[string]string {
	out := map[string]string{}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "json") {
		b, _ := io.ReadAll(http.MaxBytesReader(nil, r.Body, 4<<20))
		var raw map[string]any
		if json.Unmarshal(b, &raw) == nil {
			for k, v := range raw {
				switch t := v.(type) {
				case string:
					out[k] = t
				case float64:
					out[k] = strconv.FormatFloat(t, 'f', -1, 64)
				case bool:
					out[k] = strconv.FormatBool(t)
				case nil:
				default:
					bb, _ := json.Marshal(t)
					out[k] = string(bb)
				}
			}
		}
		return out
	}
	_ = r.ParseMultipartForm(4 << 20)
	for k, vs := range r.Form {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// cultureInvariant reports whether the client wants JSON numbers (the phone
// app) rather than LubeLogger's localized strings.
func cultureInvariant(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("culture-invariant"), "true")
}

type numFn func(float64) any

func numberer(r *http.Request) numFn {
	if cultureInvariant(r) {
		return func(f float64) any { return f }
	}
	return func(f float64) any { return strconv.FormatFloat(f, 'f', -1, 64) }
}

func major(minor int64, code string) float64 {
	return float64(minor) / math.Pow10(currency.Decimals(code))
}

const currencyMarker = "[Pocket Homelab currency: "

// compatNotes re-attaches the currency marker the phone app reads back.
func compatNotes(notes, source, cur string) string {
	if source != "api" {
		return notes
	}
	if notes != "" {
		notes += "\n"
	}
	return notes + currencyMarker + cur + "]"
}

func (s *Server) compatVehicles(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	vs, err := s.store.ListVehicles(r.Context(), store.VehicleFilter{UserID: u.ID})
	if err != nil {
		compatFail(w, 500, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(vs))
	for _, v := range vs {
		if v.Status == "archived" {
			continue
		}
		out = append(out, s.compatVehicleJSON(&v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) compatVehicleJSON(v *store.Vehicle) map[string]any {
	img := ""
	if v.HasImage {
		img = fmt.Sprintf("/api/v1/vehicles/%d/image", v.ID)
	}
	tags := []string{}
	if v.Tags != "" {
		tags = strings.Fields(strings.ReplaceAll(v.Tags, ",", " "))
	}
	return map[string]any{
		"id": v.ID, "imageLocation": img, "year": v.Year, "make": v.Make, "model": v.Model, "licensePlate": v.Plate,
		"purchaseDate": v.PurchaseDate, "soldDate": v.SoldDate, "purchasePrice": major(v.PurchasePrice, v.Currency), "soldPrice": major(v.SoldPrice, v.Currency),
		"isElectric": v.FuelType == "electric", "isDiesel": v.FuelType == "diesel", "useHours": v.OdometerUnit == "h", "odometerOptional": false,
		"extraFields": []any{}, "tags": tags, "hasOdometerAdjustment": v.OdometerOffset != 0, "odometerMultiplier": "1", "odometerDifference": strconv.FormatFloat(v.OdometerOffset, 'f', -1, 64),
		"dashboardMetrics": []any{}, "vehicleIdentifier": "LicensePlate", "name": v.Title(), "currency": v.Currency, "odometerUnit": v.OdometerUnit,
	}
}

func (s *Server) compatAddVehicle(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	f := readForm(r)
	st, _ := s.store.GetSettings(r.Context())
	year, _ := strconv.Atoi(f["year"])
	v := &store.Vehicle{OwnerID: u.ID, Year: year, Make: strings.TrimSpace(f["make"]), Model: strings.TrimSpace(f["model"]), Plate: strings.TrimSpace(f["licensePlate"]), Currency: st.HomeCurrency, OdometerUnit: st.DistanceUnit, Status: "active", Kind: "car", Tags: strings.TrimSpace(f["tags"])}
	switch strings.ToLower(f["fuelType"]) {
	case "diesel":
		v.FuelType = "diesel"
	case "electric":
		v.FuelType = "electric"
	default:
		v.FuelType = "petrol"
	}
	if strings.EqualFold(f["useEngineHours"], "true") {
		v.OdometerUnit = "h"
	}
	if v.Make == "" || v.Model == "" {
		compatFail(w, http.StatusBadRequest, "make and model are required")
		return
	}
	if err := s.store.CreateVehicle(r.Context(), v); err != nil {
		compatFail(w, 500, err.Error())
		return
	}
	s.store.LogActivity(r.Context(), v.ID, u.ID, "added vehicle", v.Title())
	compatOK(w, "Vehicle added", map[string]any{"vehicleId": v.ID})
}

func (s *Server) compatInfo(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	vs, err := s.store.ListVehicles(r.Context(), store.VehicleFilter{UserID: u.ID})
	if err != nil {
		compatFail(w, 500, err.Error())
		return
	}
	only, _ := strconv.ParseInt(r.URL.Query().Get("vehicleId"), 10, 64)
	out := []map[string]any{}
	for i := range vs {
		v := &vs[i]
		if v.Status == "archived" || (only > 0 && v.ID != only) {
			continue
		}
		sum, err := s.store.Summary(r.Context(), v)
		if err != nil {
			continue
		}
		var next any
		if sum.NextReminder != nil {
			next = s.compatReminderJSON(sum.NextReminder, v)
		}
		pastDue, urgent, veryUrgent, notUrgent := 0, 0, 0, 0
		rems, _ := s.store.ListReminders(r.Context(), v.ID, "active")
		for j := range rems {
			store.ComputeUrgency(&rems[j], store.Today(), sum.OdometerEst)
			switch rems[j].Urgency {
			case "overdue":
				pastDue++
			case "due":
				veryUrgent++
			case "soon":
				urgent++
			default:
				notUrgent++
			}
		}
		plans, _ := s.store.ListPlans(r.Context(), v.ID)
		stage := map[string]int{}
		for _, p := range plans {
			stage[p.Stage]++
		}
		recCount := func(k string) int {
			rs, _ := s.store.ListRecords(r.Context(), v.ID, store.RecordFilter{Kind: k})
			return len(rs)
		}
		out = append(out, map[string]any{
			"vehicleData":             s.compatVehicleJSON(v),
			"veryUrgentReminderCount": veryUrgent, "urgentReminderCount": urgent, "notUrgentReminderCount": notUrgent, "pastDueReminderCount": pastDue, "nextReminder": next,
			"serviceRecordCount": recCount("service"), "serviceRecordCost": major(sum.CostByKind["service"], v.Currency),
			"repairRecordCount": recCount("repair"), "repairRecordCost": major(sum.CostByKind["repair"], v.Currency),
			"upgradeRecordCount": recCount("upgrade"), "upgradeRecordCost": major(sum.CostByKind["upgrade"], v.Currency),
			"taxRecordCount": recCount("expense"), "taxRecordCost": major(sum.CostByKind["expense"], v.Currency),
			"gasRecordCount": sum.Fills, "gasRecordCost": major(sum.CostFuelTotal, v.Currency),
			"lastReportedOdometer": sum.Odometer, "estimatedOdometer": math.Round(sum.OdometerEst),
			"planRecordBackLogCount": stage["backlog"] + stage["planned"], "planRecordInProgressCount": stage["in_progress"], "planRecordTestingCount": stage["testing"], "planRecordDoneCount": stage["done"],
			"currency": v.Currency, "totalCost": major(sum.CostTotal, v.Currency), "costThisYear": major(sum.CostYear, v.Currency), "fuelEconomy": sum.Economy,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) compatReminderJSON(rm *store.Reminder, v *store.Vehicle) map[string]any {
	urg := map[string]string{"overdue": "PastDue", "due": "VeryUrgent", "soon": "Urgent", "ok": "NotUrgent"}[rm.Urgency]
	metric := "Date"
	if rm.DueDate != "" && rm.DueOdometer != nil {
		metric = "Both"
	} else if rm.DueOdometer != nil {
		metric = "Odometer"
	}
	var dueOdo float64
	if rm.DueOdometer != nil {
		dueOdo = *rm.DueOdometer
	}
	days, km := 0, 0.0
	if rm.DaysLeft != nil {
		days = *rm.DaysLeft
	}
	if rm.KmLeft != nil {
		km = *rm.KmLeft
	}
	return map[string]any{"id": rm.ID, "vehicleId": v.ID, "description": rm.Title, "notes": rm.Notes, "urgency": urg, "metric": metric, "dueDate": rm.DueDate, "dueOdometer": dueOdo, "dueDays": days, "dueDistance": km, "tags": rm.Category, "isRecurring": rm.IntervalDays > 0 || rm.IntervalKm > 0}
}

func (s *Server) compatReminders(w http.ResponseWriter, r *http.Request) {
	v := s.compatVehicle(w, r, nil)
	if v == nil {
		return
	}
	rems, _ := s.store.ListReminders(r.Context(), v.ID, "active")
	reading, date := s.store.LatestOdometer(r.Context(), v.ID)
	est := reading + s.store.DawarichKmSince(r.Context(), v.ID, date)
	out := make([]map[string]any, 0, len(rems))
	for i := range rems {
		store.ComputeUrgency(&rems[i], store.Today(), est)
		out = append(out, s.compatReminderJSON(&rems[i], v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) compatPlans(w http.ResponseWriter, r *http.Request) {
	v := s.compatVehicle(w, r, nil)
	if v == nil {
		return
	}
	plans, _ := s.store.ListPlans(r.Context(), v.ID)
	out := make([]map[string]any, 0, len(plans))
	stage := map[string]string{"backlog": "Backlog", "planned": "Backlog", "in_progress": "InProgress", "testing": "Testing", "done": "Done"}
	prio := map[string]string{"low": "Low", "normal": "Normal", "high": "Critical", "critical": "Critical"}
	kind := map[string]string{"service": "ServiceRecord", "repair": "RepairRecord", "upgrade": "UpgradeRecord", "expense": "TaxRecord"}
	for _, p := range plans {
		out = append(out, map[string]any{"id": p.ID, "vehicleId": v.ID, "description": p.Title, "notes": p.Notes, "type": kind[p.Kind], "priority": prio[p.Priority], "progress": stage[p.Stage], "cost": major(p.Estimate, v.Currency), "dateCreated": p.CreatedAt[:10], "dateModified": p.UpdatedAt[:10]})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) compatNotes(w http.ResponseWriter, r *http.Request) {
	v := s.compatVehicle(w, r, nil)
	if v == nil {
		return
	}
	notes, _ := s.store.ListNotes(r.Context(), v.ID)
	out := make([]map[string]any, 0, len(notes))
	for _, n := range notes {
		out = append(out, map[string]any{"id": n.ID, "vehicleId": v.ID, "description": n.Title, "noteText": n.Body, "pinned": n.Pinned})
	}
	writeJSON(w, http.StatusOK, out)
}

func kindToRecord(kind string) string {
	switch kind {
	case "tax":
		return "expense"
	default:
		return kind
	}
}

func (s *Server) compatList(w http.ResponseWriter, r *http.Request, kind string) {
	v := s.compatVehicle(w, r, nil)
	if v == nil {
		return
	}
	num := numberer(r)
	out := []map[string]any{}
	switch kind {
	case "gas":
		fs, _ := s.store.ListFuel(r.Context(), v.ID)
		for _, f := range fs {
			out = append(out, map[string]any{"vehicleId": v.ID, "id": f.ID, "date": f.Date, "odometer": num(f.Odometer), "fuelConsumed": num(f.Volume), "cost": num(major(f.Cost, f.Currency)),
				"fuelEconomy": num(f.Economy), "isFillToFull": strconv.FormatBool(f.FullTank), "missedFuelUp": strconv.FormatBool(f.Missed), "startingSoc": num(f.SocStart), "endingSoc": num(f.SocEnd),
				"notes": compatNotes(f.Notes, f.Source, f.Currency), "tags": f.Tags, "extraFields": []any{}, "files": []any{}, "currency": f.Currency, "station": f.Station, "distance": num(f.Distance)})
		}
	case "odometer":
		hist, _ := s.store.OdometerHistory(r.Context(), v.ID)
		for _, o := range hist {
			if o.Source == "fuel" || o.Source == "record" {
				continue // those live in their own lists; LubeLogger's mirrored rows are exactly what we avoid
			}
			out = append(out, map[string]any{"vehicleId": v.ID, "id": o.ID, "date": o.Date, "initialOdometer": num(o.Reading - o.Delta), "odometer": num(o.Reading), "notes": o.Notes, "tags": "", "extraFields": []any{}, "files": []any{}})
		}
	default:
		rs, _ := s.store.ListRecords(r.Context(), v.ID, store.RecordFilter{Kind: kindToRecord(kind)})
		for _, rc := range rs {
			odo := 0.0
			if rc.Odometer != nil {
				odo = *rc.Odometer
			}
			row := map[string]any{"vehicleId": v.ID, "id": rc.ID, "date": rc.Date, "odometer": num(odo), "description": rc.Description, "notes": compatNotes(rc.Notes, rc.Source, rc.Currency), "cost": num(major(rc.Cost, rc.Currency)), "tags": rc.Tags, "extraFields": []any{}, "files": []any{}, "currency": rc.Currency, "category": rc.Category, "vendor": rc.Vendor}
			if kind == "tax" {
				delete(row, "odometer")
			}
			out = append(out, row)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// compatFill applies a LubeLogger-shaped form to a record / fuel / odometer row.
func (s *Server) compatAdd(w http.ResponseWriter, r *http.Request, kind string) {
	f := readForm(r)
	v := s.compatVehicle(w, r, f)
	if v == nil {
		return
	}
	if s.store.VehicleRole(r.Context(), v.ID, userFrom(r.Context())) == "viewer" {
		compatFail(w, http.StatusForbidden, "read-only access")
		return
	}
	id, msg := s.compatWrite(r.Context(), v, kind, f, 0)
	if msg != "" {
		compatFail(w, http.StatusBadRequest, msg)
		return
	}
	compatOK(w, "Record added", map[string]any{"recordId": id})
}

func (s *Server) compatUpdate(w http.ResponseWriter, r *http.Request, kind string) {
	f := readForm(r)
	if f["vehicleId"] == "" && r.URL.Query().Get("vehicleId") == "" {
		// find the vehicle through the record id
		id, _ := strconv.ParseInt(f["id"], 10, 64)
		var vid int64
		table := map[string]string{"gas": "fuel", "odometer": "odometer"}[kind]
		if table == "" {
			table = "records"
		}
		_ = s.store.DB.QueryRowContext(r.Context(), `SELECT vehicle_id FROM `+table+` WHERE id = ?`, id).Scan(&vid)
		f["vehicleId"] = strconv.FormatInt(vid, 10)
	}
	v := s.compatVehicle(w, r, f)
	if v == nil {
		return
	}
	id, _ := strconv.ParseInt(f["id"], 10, 64)
	if id == 0 {
		compatFail(w, http.StatusBadRequest, "id is required")
		return
	}
	if _, msg := s.compatWrite(r.Context(), v, kind, f, id); msg != "" {
		compatFail(w, http.StatusBadRequest, msg)
		return
	}
	compatOK(w, "Record updated", map[string]any{"recordId": id})
}

func (s *Server) compatDelete(w http.ResponseWriter, r *http.Request, kind string) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if id == 0 {
		f := readForm(r)
		id, _ = strconv.ParseInt(f["id"], 10, 64)
	}
	if id == 0 {
		compatFail(w, http.StatusBadRequest, "id is required")
		return
	}
	u := userFrom(r.Context())
	table := map[string]string{"gas": "fuel", "odometer": "odometer"}[kind]
	if table == "" {
		table = "records"
	}
	var vid int64
	_ = s.store.DB.QueryRowContext(r.Context(), `SELECT vehicle_id FROM `+table+` WHERE id = ?`, id).Scan(&vid)
	role := s.store.VehicleRole(r.Context(), vid, u)
	if role == "" || role == "viewer" {
		compatFail(w, http.StatusNotFound, "record not found")
		return
	}
	var err error
	switch table {
	case "fuel":
		err = s.store.DeleteFuel(r.Context(), vid, id)
	case "odometer":
		err = s.store.DeleteOdometer(r.Context(), vid, id)
	default:
		err = s.store.DeleteRecord(r.Context(), vid, id)
	}
	if err != nil {
		compatFail(w, http.StatusNotFound, "record not found")
		return
	}
	compatOK(w, "Record deleted", nil)
}

// compatWrite creates (id == 0) or replaces a row from a LubeLogger form.
func (s *Server) compatWrite(ctx context.Context, v *store.Vehicle, kind string, f map[string]string, id int64) (int64, string) {
	date := lubelogger.ParseDate(f["date"])
	if date == "" {
		date = store.Today()
	}
	cur, notes := lubelogger.Currency(f["notes"])
	if cur == "" || !currency.Valid(cur) {
		cur = v.Currency
	}
	cost := int64(math.Round(lubelogger.ParseNumber(f["cost"]) * math.Pow10(currency.Decimals(cur))))
	odo := lubelogger.ParseNumber(f["odometer"])
	uid := userFrom(ctx).ID
	truthy := func(k string, def bool) bool {
		val, ok := f[k]
		if !ok || val == "" {
			return def
		}
		val = strings.ToLower(val)
		return val == "true" || val == "1" || val == "on" || val == "yes"
	}
	switch kind {
	case "gas":
		vol := lubelogger.ParseNumber(f["fuelConsumed"])
		if vol <= 0 {
			return 0, "fuelConsumed is required"
		}
		var fu *store.Fuel
		if id > 0 {
			var err error
			fu, err = s.store.GetFuel(ctx, v.ID, id)
			if err != nil {
				return 0, "record not found"
			}
		} else {
			fu = &store.Fuel{VehicleID: v.ID, Source: "api", CreatedBy: &uid}
		}
		fu.Date, fu.Odometer, fu.Volume, fu.Cost, fu.Currency, fu.Notes = date, odo, vol, cost, cur, notes
		fu.FullTank = truthy("isFillToFull", true)
		fu.Missed = truthy("missedFuelUp", false)
		fu.SocStart = lubelogger.ParseNumber(f["startingSoc"])
		fu.SocEnd = lubelogger.ParseNumber(f["endingSoc"])
		if t := strings.TrimSpace(f["tags"]); t != "" {
			fu.Tags = t
		}
		if st := strings.TrimSpace(f["station"]); st != "" {
			fu.Station = st
		}
		fu.FXRate, _ = s.rate(ctx, fu.Currency, v.Currency, fu.Date)
		fu.CostHome = convert(fu.Cost, fu.Currency, v.Currency, fu.FXRate)
		if id > 0 {
			if err := s.store.UpdateFuel(ctx, fu); err != nil {
				return 0, err.Error()
			}
		} else if err := s.store.CreateFuel(ctx, fu); err != nil {
			return 0, err.Error()
		}
		s.store.LogActivity(ctx, v.ID, uid, "added fill-up (API)", fu.Station)
		return fu.ID, ""
	case "odometer":
		if odo <= 0 {
			return 0, "odometer is required"
		}
		var o *store.OdometerReading
		if id > 0 {
			var err error
			o, err = s.store.GetOdometer(ctx, v.ID, id)
			if err != nil {
				return 0, "record not found"
			}
		} else {
			o = &store.OdometerReading{VehicleID: v.ID, Source: "api"}
		}
		o.Date, o.Reading, o.Notes = date, odo, notes
		if id > 0 {
			if err := s.store.UpdateOdometer(ctx, o); err != nil {
				return 0, err.Error()
			}
		} else if err := s.store.CreateOdometer(ctx, o); err != nil {
			return 0, err.Error()
		}
		return o.ID, ""
	default:
		desc := strings.TrimSpace(f["description"])
		if desc == "" {
			return 0, "description is required"
		}
		var rc *store.Record
		if id > 0 {
			var err error
			rc, err = s.store.GetRecord(ctx, v.ID, id)
			if err != nil {
				return 0, "record not found"
			}
		} else {
			rc = &store.Record{VehicleID: v.ID, Kind: kindToRecord(kind), Source: "api", CreatedBy: &uid, Parts: []store.Part{}}
		}
		rc.Date, rc.Description, rc.Notes, rc.Cost, rc.Currency = date, desc, notes, cost, cur
		if kind != "tax" && odo > 0 {
			rc.Odometer = &odo
		}
		if t := strings.TrimSpace(f["tags"]); t != "" {
			rc.Tags = t
		}
		if c := strings.TrimSpace(f["category"]); c != "" {
			rc.Category = c
		}
		if vd := strings.TrimSpace(f["vendor"]); vd != "" {
			rc.Vendor = vd
		}
		rc.FXRate, _ = s.rate(ctx, rc.Currency, v.Currency, rc.Date)
		rc.CostHome = convert(rc.Cost, rc.Currency, v.Currency, rc.FXRate)
		if id > 0 {
			if err := s.store.UpdateRecord(ctx, rc); err != nil {
				return 0, err.Error()
			}
		} else if err := s.store.CreateRecord(ctx, rc); err != nil {
			return 0, err.Error()
		}
		s.store.LogActivity(ctx, v.ID, uid, "added "+rc.Kind+" (API)", rc.Description)
		s.afterRecordChange(ctx, v, rc)
		return rc.ID, ""
	}
}
