package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/lubelogger"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// ---- LubeLogger ----

type llImportResult struct {
	Vehicles []llVehicleResult `json:"vehicles"`
	Error    string            `json:"error,omitempty"`
}

type llVehicleResult struct {
	Source   string         `json:"source"`
	ID       int64          `json:"vehicle_id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Counts   map[string]int `json:"counts,omitempty"`
	Skipped  int            `json:"skipped"`
	Error    string         `json:"error,omitempty"`
	Existing bool           `json:"existing"`
}

type llInput struct {
	URL      string  `json:"url"`
	Username string  `json:"username"`
	Password string  `json:"password"`
	Currency string  `json:"currency"`
	Vehicles []int64 `json:"vehicles"`
	Image    bool    `json:"image"`
}

func (s *Server) handleLLVehicles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in llInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := lubelogger.New(in.URL, in.Username, in.Password, 30*time.Second)
	vs, err := c.Vehicles(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []map[string]any{}
	for _, v := range vs {
		ext := "lubelogger:" + strconv.FormatInt(v.ID, 10)
		_, existsErr := s.store.VehicleByExternalID(r.Context(), ext)
		out = append(out, map[string]any{"id": v.ID, "name": strings.TrimSpace(fmt.Sprintf("%d %s %s", v.Year, v.Make, v.Model)), "plate": v.LicensePlate, "imported": existsErr == nil})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleLLImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in llInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(r.Context())
	cur := strings.ToUpper(in.Currency)
	if !currency.Valid(cur) {
		cur = st.HomeCurrency
	}
	c := lubelogger.New(in.URL, in.Username, in.Password, 60*time.Second)
	res := s.importLubeLoggerSel(r.Context(), c, userFrom(r.Context()).ID, cur, in.Vehicles)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) importLubeLogger(ctx context.Context, c *lubelogger.Client, ownerID int64, cur string) llImportResult {
	return s.importLubeLoggerSel(ctx, c, ownerID, cur, nil)
}

// importLubeLoggerSel imports the chosen vehicles (all when sel is empty).
// Re-running is safe: every row is keyed by its LubeLogger id.
func (s *Server) importLubeLoggerSel(ctx context.Context, c *lubelogger.Client, ownerID int64, cur string, sel []int64) llImportResult {
	var res llImportResult
	vs, err := c.Vehicles(ctx)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	want := map[int64]bool{}
	for _, id := range sel {
		want[id] = true
	}
	for _, lv := range vs {
		if len(want) > 0 && !want[lv.ID] {
			continue
		}
		res.Vehicles = append(res.Vehicles, s.importOneLL(ctx, c, ownerID, cur, lv))
	}
	if res.Vehicles == nil {
		res.Vehicles = []llVehicleResult{}
	}
	return res
}

func (s *Server) importOneLL(ctx context.Context, c *lubelogger.Client, ownerID int64, cur string, lv lubelogger.Vehicle) llVehicleResult {
	out := llVehicleResult{Source: strings.TrimSpace(fmt.Sprintf("%d %s %s", lv.Year, lv.Make, lv.Model)), Counts: map[string]int{}}
	ext := "lubelogger:" + strconv.FormatInt(lv.ID, 10)
	v, err := s.store.VehicleByExternalID(ctx, ext)
	if err != nil {
		fuel := "petrol"
		if lv.IsDiesel {
			fuel = "diesel"
		}
		if lv.IsElectric {
			fuel = "electric"
		}
		unit := "km"
		if lv.UseHours {
			unit = "h"
		}
		st, _ := s.store.GetSettings(ctx)
		if st.DistanceUnit == "mi" && unit == "km" {
			unit = "mi"
		}
		dec := currency.Decimals(cur)
		v = &store.Vehicle{OwnerID: ownerID, Year: lv.Year, Make: lv.Make, Model: lv.Model, Plate: lv.LicensePlate, FuelType: fuel, OdometerUnit: unit, Currency: cur, Kind: "car",
			PurchaseDate: lubelogger.ParseDate(lv.PurchaseDate), SoldDate: lubelogger.ParseDate(lv.SoldDate), PurchasePrice: int64(math.Round(float64(lv.PurchasePrice) * math.Pow10(dec))), SoldPrice: int64(math.Round(float64(lv.SoldPrice) * math.Pow10(dec))),
			Status: "active", Tags: strings.Join(lv.Tags, " "), ExternalID: ext}
		if v.SoldDate != "" {
			v.Status = "sold"
		}
		var extra []string
		for _, f := range lv.ExtraFields {
			if f.Value != "" {
				extra = append(extra, f.Name+": "+f.Value)
			}
		}
		v.Notes = strings.Join(extra, "\n")
		if err := s.store.CreateVehicle(ctx, v); err != nil {
			out.Error = err.Error()
			return out
		}
		if lv.ImageLocation != "" && !strings.HasPrefix(lv.ImageLocation, "/defaults/") {
			if b, ct, err := c.GetBytes(ctx, lv.ImageLocation); err == nil && strings.HasPrefix(ct, "image/") {
				if rel, err := s.saveFile(v.ID, path.Ext(lv.ImageLocation), b); err == nil {
					v.ImagePath = rel
					_ = s.store.UpdateVehicle(ctx, v)
				}
			}
		}
		s.store.LogActivity(ctx, v.ID, ownerID, "imported from LubeLogger", v.Title())
	} else {
		out.Existing = true
	}
	out.ID, out.Name = v.ID, v.Title()
	dec := currency.Decimals(cur)
	minor := func(n lubelogger.Number, code string) int64 {
		return int64(math.Round(float64(n) * math.Pow10(currency.Decimals(code))))
	}
	_ = dec
	for _, kind := range []string{"servicerecords", "repairrecords", "upgraderecords", "taxrecords"} {
		rows, err := c.Records(ctx, kind, lv.ID)
		if err != nil {
			out.Error = kind + ": " + err.Error()
			continue
		}
		rk := map[string]string{"servicerecords": "service", "repairrecords": "repair", "upgraderecords": "upgrade", "taxrecords": "expense"}[kind]
		for _, row := range rows {
			rcur, notes := lubelogger.Currency(row.Notes)
			if rcur == "" || !currency.Valid(rcur) {
				rcur = cur
			}
			rec := &store.Record{VehicleID: v.ID, Kind: rk, Date: lubelogger.ParseDate(row.Date), Description: strings.TrimSpace(row.Description), Notes: notes, Cost: minor(row.Cost, rcur), Currency: rcur, Tags: row.Tags, Source: "lubelogger", ExternalID: fmt.Sprintf("%s:%d", kind, row.ID), Parts: []store.Part{}}
			if rec.Date == "" {
				rec.Date = store.Today()
			}
			if o := float64(row.Odometer); o > 0 && rk != "expense" {
				rec.Odometer = &o
			}
			for _, f := range row.ExtraFields {
				if f.Value != "" {
					rec.Notes = strings.TrimSpace(rec.Notes + "\n" + f.Name + ": " + f.Value)
				}
			}
			rec.Category = guessCategoryText(rec.Description, rk)
			rec.FXRate, _ = s.rate(ctx, rec.Currency, v.Currency, rec.Date)
			rec.CostHome = convert(rec.Cost, rec.Currency, v.Currency, rec.FXRate)
			if err := s.store.CreateRecord(ctx, rec); err != nil {
				out.Skipped++
				continue
			}
			out.Counts[rk]++
		}
	}
	if rows, err := c.Records(ctx, "gasrecords", lv.ID); err == nil {
		for _, row := range rows {
			rcur, notes := lubelogger.Currency(row.Notes)
			if rcur == "" || !currency.Valid(rcur) {
				rcur = cur
			}
			f := &store.Fuel{VehicleID: v.ID, Date: lubelogger.ParseDate(row.Date), Odometer: float64(row.Odometer), Volume: float64(row.FuelConsumed), Cost: minor(row.Cost, rcur), Currency: rcur, FullTank: bool(row.IsFillToFull), Missed: bool(row.MissedFuelUp), Notes: notes, Tags: row.Tags, Source: "lubelogger", ExternalID: fmt.Sprintf("gasrecords:%d", row.ID)}
			if f.Date == "" {
				f.Date = store.Today()
			}
			f.FXRate, _ = s.rate(ctx, f.Currency, v.Currency, f.Date)
			f.CostHome = convert(f.Cost, f.Currency, v.Currency, f.FXRate)
			if err := s.store.CreateFuel(ctx, f); err != nil {
				out.Skipped++
				continue
			}
			out.Counts["fuel"]++
		}
	} else {
		out.Error = "gasrecords: " + err.Error()
	}
	if rows, err := c.Records(ctx, "odometerrecords", lv.ID); err == nil {
		for _, row := range rows {
			// LubeLogger mirrors every record's odometer into this list; those rows are derived here, so skip them.
			if strings.HasPrefix(row.Notes, "Auto Insert From") {
				continue
			}
			if float64(row.Odometer) <= 0 {
				continue
			}
			o := &store.OdometerReading{VehicleID: v.ID, Date: lubelogger.ParseDate(row.Date), Reading: float64(row.Odometer), Notes: strings.TrimSpace(row.Notes), Source: "lubelogger", ExternalID: fmt.Sprintf("odometerrecords:%d", row.ID)}
			if o.Date == "" {
				o.Date = store.Today()
			}
			if err := s.store.CreateOdometer(ctx, o); err != nil {
				out.Skipped++
				continue
			}
			out.Counts["odometer"]++
		}
	}
	if rows, err := c.Reminders(ctx, lv.ID); err == nil {
		for _, row := range rows {
			rm := &store.Reminder{VehicleID: v.ID, Title: strings.TrimSpace(row.Description), Notes: row.Notes, Kind: "service", DueDate: lubelogger.ParseDate(row.DueDate), ExternalID: fmt.Sprintf("reminders:%d", row.ID)}
			if o := float64(row.DueOdometer); o > 0 {
				rm.DueOdometer = &o
			}
			if strings.EqualFold(row.Metric, "Odometer") {
				rm.DueDate = ""
			}
			if rm.DueDate == "" && rm.DueOdometer == nil {
				continue
			}
			var exists int
			_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM reminders WHERE vehicle_id = ? AND external_id = ?`, v.ID, rm.ExternalID).Scan(&exists)
			if exists > 0 {
				out.Skipped++
				continue
			}
			if err := s.store.CreateReminder(ctx, rm); err == nil {
				out.Counts["reminders"]++
			}
		}
	}
	if rows, err := c.Plans(ctx, lv.ID); err == nil {
		stage := map[string]string{"Backlog": "backlog", "InProgress": "in_progress", "Testing": "testing", "Done": "done"}
		prio := map[string]string{"Low": "low", "Normal": "normal", "Critical": "critical"}
		kind := map[string]string{"ServiceRecord": "service", "RepairRecord": "repair", "UpgradeRecord": "upgrade", "TaxRecord": "expense"}
		for _, row := range rows {
			ext := fmt.Sprintf("planrecords:%d", row.ID)
			var exists int
			_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM plans WHERE vehicle_id = ? AND external_id = ?`, v.ID, ext).Scan(&exists)
			if exists > 0 {
				out.Skipped++
				continue
			}
			p := &store.Plan{VehicleID: v.ID, Title: strings.TrimSpace(row.Description), Notes: row.Notes, Kind: orDefault(kind[row.Type], "service"), Priority: orDefault(prio[row.Priority], "normal"), Stage: orDefault(stage[row.Progress], "backlog"), Estimate: minor(row.Cost, cur), ExternalID: ext}
			if err := s.store.CreatePlan(ctx, p); err == nil {
				out.Counts["plans"]++
			}
		}
	}
	if rows, err := c.Notes(ctx, lv.ID); err == nil {
		for _, row := range rows {
			ext := fmt.Sprintf("notes:%d", row.ID)
			var exists int
			_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM notes WHERE vehicle_id = ? AND external_id = ?`, v.ID, ext).Scan(&exists)
			if exists > 0 {
				out.Skipped++
				continue
			}
			n := &store.Note{VehicleID: v.ID, Title: strings.TrimSpace(row.Description), Body: row.NoteText, Pinned: row.Pinned, ExternalID: ext}
			if err := s.store.CreateNote(ctx, n); err == nil {
				out.Counts["notes"]++
			}
		}
	}
	return out
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// guessCategoryText picks a category from a free-text description (Bosnian/Serbian/Croatian and English words).
func guessCategoryText(desc, kind string) string {
	d := strings.ToLower(desc)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(d, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("gum", "tyre", "tire", "pneu"):
		return "tyres"
	case has("ulje", "oil", "mali servis", "veliki servis"):
		return "oil"
	case has("filter", "filtr"):
		return "filters"
	case has("kočn", "kocn", "brake", "disk", "pločic", "plocic"):
		return "brakes"
	case has("akumulator", "battery", "baterij"):
		return "battery"
	case has("zupčast", "zupcast", "remen", "timing", "belt"):
		return "timing belt"
	case has("svjećic", "svjecic", "spark"):
		return "spark plugs"
	case has("amortizer", "stabilizator", "selen", "ležaj", "lezar", "lezaj", "suspension", "shock", "ramen"):
		return "suspension"
	case has("auspuh", "exhaust", "lambda", "dpf", "katalizator"):
		return "exhaust"
	case has("kvačil", "kvacil", "clutch"):
		return "clutch"
	case has("klima", "a/c", "air con"):
		return "ac"
	case has("staklo", "glass", "windshield", "brisač", "brisac"):
		return "glass"
	case has("registrac", "registration"):
		return "registration"
	case has("osigur", "insur", "kasko"):
		return "insurance"
	case has("pranje", "wash"):
		return "wash"
	case has("tehnič", "tehnic", "inspection", "mot"):
		return "inspection"
	case has("parking"):
		return "parking"
	case has("putarin", "toll"):
		return "toll"
	case has("crijev", "hose", "ventil", "valve", "pump", "pumpa", "hladnjak", "coolant", "antifriz"):
		return "coolant"
	}
	if kind == "expense" {
		return "other"
	}
	return ""
}

// ---- JSON backup of one vehicle ----

type vehicleBackup struct {
	Format    string                  `json:"format"`
	Version   string                  `json:"version"`
	Exported  string                  `json:"exported_at"`
	Vehicle   *store.Vehicle          `json:"vehicle"`
	Records   []store.Record          `json:"records"`
	Fuel      []store.Fuel            `json:"fuel"`
	Odometer  []store.OdometerReading `json:"odometer"`
	Reminders []store.Reminder        `json:"reminders"`
	Plans     []store.Plan            `json:"plans"`
	Notes     []store.Note            `json:"notes"`
	Days      []store.DawarichDay     `json:"dawarich_days"`
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := vehicleFrom(ctx).vehicle
	b := vehicleBackup{Format: "pocket-garage/vehicle", Version: Version, Exported: store.Now(), Vehicle: v}
	b.Records, _ = s.store.ListRecords(ctx, v.ID, store.RecordFilter{})
	b.Fuel, _ = s.store.ListFuel(ctx, v.ID)
	hist, _ := s.store.OdometerHistory(ctx, v.ID)
	for _, o := range hist {
		if o.Source != "fuel" && o.Source != "record" {
			b.Odometer = append(b.Odometer, o)
		}
	}
	b.Reminders, _ = s.store.ListReminders(ctx, v.ID, "")
	b.Plans, _ = s.store.ListPlans(ctx, v.ID)
	b.Notes, _ = s.store.ListNotes(ctx, v.ID)
	b.Days, _ = s.store.DawarichDays(ctx, v.ID, "2000-01-01", "2100-01-01")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(v.Title())+`.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(b)
}

func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	var b vehicleBackup
	if err := decode(r, &b); err != nil || b.Vehicle == nil {
		writeErr(w, http.StatusBadRequest, "not a Pocket Garage vehicle backup")
		return
	}
	v := *b.Vehicle
	v.ID, v.OwnerID, v.ImagePath, v.HasImage, v.ExternalID = 0, u.ID, "", false, ""
	if err := s.store.CreateVehicle(ctx, &v); err != nil {
		s.fail(w, err, "create vehicle")
		return
	}
	for _, rc := range b.Records {
		rc.ID, rc.VehicleID, rc.ExternalID = 0, v.ID, ""
		if rc.Parts == nil {
			rc.Parts = []store.Part{}
		}
		_ = s.store.CreateRecord(ctx, &rc)
	}
	for _, f := range b.Fuel {
		f.ID, f.VehicleID, f.ExternalID = 0, v.ID, ""
		_ = s.store.CreateFuel(ctx, &f)
	}
	for _, o := range b.Odometer {
		o.ID, o.VehicleID, o.ExternalID = 0, v.ID, ""
		_ = s.store.CreateOdometer(ctx, &o)
	}
	for _, rm := range b.Reminders {
		rm.ID, rm.VehicleID, rm.ExternalID = 0, v.ID, ""
		_ = s.store.CreateReminder(ctx, &rm)
	}
	for _, p := range b.Plans {
		p.ID, p.VehicleID, p.RecordID, p.ExternalID = 0, v.ID, nil, ""
		_ = s.store.CreatePlan(ctx, &p)
	}
	for _, n := range b.Notes {
		n.ID, n.VehicleID, n.ExternalID = 0, v.ID, ""
		_ = s.store.CreateNote(ctx, &n)
	}
	for _, d := range b.Days {
		_ = s.store.PutDawarichDay(ctx, v.ID, d.Date, d.Km, d.TotalKm)
	}
	s.store.LogActivity(ctx, v.ID, u.ID, "imported from backup", v.Title())
	writeJSON(w, http.StatusCreated, v)
}
