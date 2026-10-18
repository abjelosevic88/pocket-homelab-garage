package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

type vehicleKeyT int

const vehicleKey vehicleKeyT = 2

type vehicleCtx struct {
	vehicle *store.Vehicle
	role    string // owner | editor | viewer | admin
}

func vehicleFrom(ctx context.Context) *vehicleCtx {
	v, _ := ctx.Value(vehicleKey).(*vehicleCtx)
	return v
}

// vehicleAccess loads the vehicle and checks the user can see it.
func (s *Server) vehicleAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		id := idParam(r, "vehicleID")
		role := s.store.VehicleRole(r.Context(), id, u)
		if role == "" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		v, err := s.store.GetVehicle(r.Context(), id, u.ID)
		if err != nil {
			s.fail(w, err, "get vehicle")
			return
		}
		if v.Role == "" {
			v.Role = role
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), vehicleKey, &vehicleCtx{vehicle: v, role: role})))
	})
}

// canEdit rejects viewers.
func (s *Server) canEdit(w http.ResponseWriter, r *http.Request) bool {
	vc := vehicleFrom(r.Context())
	if vc == nil || vc.role == "viewer" {
		writeErr(w, http.StatusForbidden, "you can only view this vehicle")
		return false
	}
	return true
}

func (s *Server) isOwner(r *http.Request) bool {
	vc := vehicleFrom(r.Context())
	return vc != nil && (vc.role == "owner" || vc.role == "admin")
}

// visibleVehicleIDs lists the ids a user may see.
func (s *Server) visibleVehicles(ctx context.Context, u *store.User, status string) ([]store.Vehicle, error) {
	f := store.VehicleFilter{UserID: u.ID, Status: status}
	return s.store.ListVehicles(ctx, f)
}

func (s *Server) handleListVehicles(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	f := store.VehicleFilter{UserID: u.ID, Query: strings.TrimSpace(r.URL.Query().Get("q")), Status: r.URL.Query().Get("status")}
	if u.Role == "admin" && queryBool(r, "all") {
		f.UserID = 0
	}
	vs, err := s.store.ListVehicles(r.Context(), f)
	if err != nil {
		s.fail(w, err, "list vehicles")
		return
	}
	if !queryBool(r, "bare") {
		for i := range vs {
			vs[i].Summary, _ = s.store.Summary(r.Context(), &vs[i])
		}
	}
	writeJSON(w, http.StatusOK, vs)
}

type vehicleInput struct {
	Name           *string  `json:"name"`
	Year           *int     `json:"year"`
	Make           *string  `json:"make"`
	Model          *string  `json:"model"`
	Trim           *string  `json:"trim"`
	Plate          *string  `json:"plate"`
	VIN            *string  `json:"vin"`
	Kind           *string  `json:"kind"`
	FuelType       *string  `json:"fuel_type"`
	OdometerUnit   *string  `json:"odometer_unit"`
	TankCapacity   *float64 `json:"tank_capacity"`
	Currency       *string  `json:"currency"`
	Color          *string  `json:"color"`
	PurchaseDate   *string  `json:"purchase_date"`
	PurchasePrice  *int64   `json:"purchase_price"`
	PurchaseOdo    *float64 `json:"purchase_odo"`
	SoldDate       *string  `json:"sold_date"`
	SoldPrice      *int64   `json:"sold_price"`
	Status         *string  `json:"status"`
	OdometerOffset *float64 `json:"odometer_offset"`
	Notes          *string  `json:"notes"`
	Tags           *string  `json:"tags"`
	FireflyTag     *string  `json:"firefly_tag"`
	FireflyAccount *string  `json:"firefly_account"`
	Dawarich       *bool    `json:"dawarich"`
	SortOrder      *int     `json:"sort_order"`
	InitialOdo     *float64 `json:"initial_odometer"` // create only: first reading
}

func applyVehicle(v *store.Vehicle, in vehicleInput, keys map[string]bool) string {
	str := func(k string, dst *string, src *string) {
		if keys[k] && src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	str("name", &v.Name, in.Name)
	str("make", &v.Make, in.Make)
	str("model", &v.Model, in.Model)
	str("trim", &v.Trim, in.Trim)
	str("plate", &v.Plate, in.Plate)
	str("vin", &v.VIN, in.VIN)
	str("kind", &v.Kind, in.Kind)
	str("fuel_type", &v.FuelType, in.FuelType)
	str("odometer_unit", &v.OdometerUnit, in.OdometerUnit)
	str("color", &v.Color, in.Color)
	str("purchase_date", &v.PurchaseDate, in.PurchaseDate)
	str("sold_date", &v.SoldDate, in.SoldDate)
	str("status", &v.Status, in.Status)
	str("notes", &v.Notes, in.Notes)
	str("tags", &v.Tags, in.Tags)
	str("firefly_tag", &v.FireflyTag, in.FireflyTag)
	str("firefly_account", &v.FireflyAccount, in.FireflyAccount)
	if keys["year"] && in.Year != nil {
		v.Year = *in.Year
	}
	if keys["tank_capacity"] && in.TankCapacity != nil {
		v.TankCapacity = *in.TankCapacity
	}
	if keys["currency"] && in.Currency != nil {
		v.Currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
	}
	if keys["purchase_price"] && in.PurchasePrice != nil {
		v.PurchasePrice = *in.PurchasePrice
	}
	if keys["purchase_odo"] && in.PurchaseOdo != nil {
		v.PurchaseOdo = *in.PurchaseOdo
	}
	if keys["sold_price"] && in.SoldPrice != nil {
		v.SoldPrice = *in.SoldPrice
	}
	if keys["odometer_offset"] && in.OdometerOffset != nil {
		v.OdometerOffset = *in.OdometerOffset
	}
	if keys["dawarich"] && in.Dawarich != nil {
		v.Dawarich = *in.Dawarich
	}
	if keys["sort_order"] && in.SortOrder != nil {
		v.SortOrder = *in.SortOrder
	}
	if v.Make == "" && v.Model == "" && v.Name == "" {
		return "a name, or make and model, is required"
	}
	if v.Kind == "" {
		v.Kind = "car"
	}
	if v.FuelType == "" {
		v.FuelType = "petrol"
	}
	if v.OdometerUnit != "km" && v.OdometerUnit != "mi" && v.OdometerUnit != "h" {
		v.OdometerUnit = "km"
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if v.Status != "active" && v.Status != "sold" && v.Status != "archived" {
		return "status must be active, sold or archived"
	}
	if v.Currency == "" {
		v.Currency = "EUR"
	}
	if !currency.Valid(v.Currency) {
		return "unknown currency"
	}
	if !validDate(v.PurchaseDate) || !validDate(v.SoldDate) {
		return "dates must be YYYY-MM-DD"
	}
	if v.Year != 0 && (v.Year < 1886 || v.Year > 2100) {
		return "year looks wrong"
	}
	return ""
}

func (s *Server) handleCreateVehicle(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in vehicleInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(r.Context())
	v := &store.Vehicle{OwnerID: u.ID, Currency: st.HomeCurrency, OdometerUnit: st.DistanceUnit, Status: "active"}
	if msg := applyVehicle(v, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateVehicle(r.Context(), v); err != nil {
		s.fail(w, err, "create vehicle")
		return
	}
	if in.InitialOdo != nil && *in.InitialOdo > 0 {
		date := v.PurchaseDate
		if date == "" {
			date = store.Today()
		}
		_ = s.store.CreateOdometer(r.Context(), &store.OdometerReading{VehicleID: v.ID, Date: date, Reading: *in.InitialOdo, Notes: "Starting reading", Source: "manual"})
	}
	s.store.LogActivity(r.Context(), v.ID, u.ID, "added vehicle", v.Title())
	v.Role = "owner"
	v.Summary, _ = s.store.Summary(r.Context(), v)
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) handleGetVehicle(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	v.Summary, _ = s.store.Summary(r.Context(), v)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUpdateVehicle(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in vehicleInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	oldTag := v.FireflyTag
	if msg := applyVehicle(v, in, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateVehicle(r.Context(), v); err != nil {
		s.fail(w, err, "update vehicle")
		return
	}
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "updated vehicle", v.Title())
	if v.FireflyTag != "" && v.FireflyTag != oldTag {
		go func(id int64) { _, _ = s.syncFirefly(context.Background(), id) }(v.ID)
	}
	v.Summary, _ = s.store.Summary(r.Context(), v)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteVehicle(w http.ResponseWriter, r *http.Request) {
	if !s.isOwner(r) {
		writeErr(w, http.StatusForbidden, "only the owner can delete a vehicle")
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	docs, _ := s.store.ListDocuments(r.Context(), v.ID)
	if err := s.store.DeleteVehicle(r.Context(), v.ID); err != nil {
		s.fail(w, err, "delete vehicle")
		return
	}
	for _, d := range docs {
		s.removeFile(d.Filename)
	}
	s.removeFile(v.ImagePath)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVehicleMembers(w http.ResponseWriter, r *http.Request) {
	if !s.isOwner(r) {
		writeErr(w, http.StatusForbidden, "only the owner can share a vehicle")
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	var in struct {
		Members []store.VehicleMember `json:"members"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var keep []store.VehicleMember
	for _, m := range in.Members {
		if m.UserID != v.OwnerID {
			keep = append(keep, m)
		}
	}
	if err := s.store.SetVehicleMembers(r.Context(), v.ID, keep); err != nil {
		s.fail(w, err, "members")
		return
	}
	v.Members, _ = s.store.VehicleMembers(r.Context(), v.ID)
	writeJSON(w, http.StatusOK, v.Members)
}

func (s *Server) handleVehicleImage(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	b, name, ct, err := s.readUpload(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "upload failed: "+err.Error())
		return
	}
	if !strings.HasPrefix(ct, "image/") {
		writeErr(w, http.StatusBadRequest, "only images, please")
		return
	}
	rel, err := s.saveFile(v.ID, name, b)
	if err != nil {
		s.fail(w, err, "save image")
		return
	}
	s.removeFile(v.ImagePath)
	v.ImagePath = rel
	v.HasImage = true
	if err := s.store.UpdateVehicle(r.Context(), v); err != nil {
		s.fail(w, err, "save vehicle")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleVehicleImageGet(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	s.serveFile(w, r, v.ImagePath, "", "")
}

func (s *Server) handleVehicleImageDelete(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	s.removeFile(v.ImagePath)
	v.ImagePath = ""
	v.HasImage = false
	_ = s.store.UpdateVehicle(r.Context(), v)
	w.WriteHeader(http.StatusNoContent)
}

// handleVehicleOverview returns everything the vehicle page needs in one call.
func (s *Server) handleVehicleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := vehicleFrom(ctx).vehicle
	sum, err := s.store.Summary(ctx, v)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	v.Summary = sum
	today := store.Today()
	rems, _ := s.store.ListReminders(ctx, v.ID, "active")
	for i := range rems {
		store.ComputeUrgency(&rems[i], today, sum.OdometerEst)
	}
	sortReminders(rems)
	recent, _ := s.store.ListRecords(ctx, v.ID, store.RecordFilter{Limit: 8})
	if recent == nil {
		recent = []store.Record{}
	}
	fuel, _ := s.store.ListFuel(ctx, v.ID)
	if fuel == nil {
		fuel = []store.Fuel{}
	}
	if len(fuel) > 8 {
		fuel = fuel[:8]
	}
	plans, _ := s.store.ListPlans(ctx, v.ID)
	open := 0
	for _, p := range plans {
		if p.Stage != "done" {
			open++
		}
	}
	notes, _ := s.store.ListNotes(ctx, v.ID)
	pinned := []store.Note{}
	for _, n := range notes {
		if n.Pinned {
			pinned = append(pinned, n)
		}
	}
	docs, _ := s.store.ExpiringDocuments(ctx, v.ID, addDays(today, 60))
	months, _ := s.store.CostsByMonth(ctx, v.ID, addMonths(today[:7]+"-01", -11), today)
	activity, _ := s.store.ListActivity(ctx, v.ID, 10)
	if activity == nil {
		activity = []store.Activity{}
	}
	if rems == nil {
		rems = []store.Reminder{}
	}
	if docs == nil {
		docs = []store.Document{}
	}
	if months == nil {
		months = []store.MonthlyCost{}
	}
	cl := s.clients(ctx)
	writeJSON(w, http.StatusOK, map[string]any{
		"vehicle": v, "reminders": rems, "recent_records": recent, "recent_fuel": fuel, "open_plans": open, "pinned_notes": pinned,
		"expiring_documents": docs, "months": months, "activity": activity, "trips": s.tripsFor(ctx, cl, v),
		"integrations": map[string]bool{"firefly": cl.firefly.Configured() && v.FireflyTag != "", "dawarich": cl.dawarich.Configured() && v.Dawarich, "travel": cl.travel.Configured(), "inventory": cl.inventory.Configured(), "notify": cl.notify.Configured()},
	})
}

func (s *Server) handleVehicleActivity(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.ListActivity(r.Context(), vehicleFrom(r.Context()).vehicle.ID, 100)
	if err != nil {
		s.fail(w, err, "activity")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleSuggestions returns the categories, vendors and stations used so far.
func (s *Server) handleSuggestions(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	cats, vendors, stations := s.store.Categories(r.Context(), v.ID)
	writeJSON(w, http.StatusOK, map[string]any{"categories": merge(cats, append(append([]string{}, store.ServiceCategory...), store.ExpenseCategory...)), "vendors": orEmpty(vendors), "stations": orEmpty(stations)})
}

func merge(first, rest []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, l := range [][]string{first, rest} {
		for _, x := range l {
			if x != "" && !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
