package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/currency"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/dawarich"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/export"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/firefly"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/lubelogger"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/money"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

func convert(amount int64, from, to string, rate float64) int64 {
	return currency.Convert(amount, from, to, rate)
}

// ---- settings ----

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

func (s *Server) publicSettings(ctx context.Context, u *store.User, st store.Settings) map[string]any {
	travelVia := s.siblingVia(ctx, u, "travel", st.TravelToken, travelScopes)
	inventoryVia := s.siblingVia(ctx, u, "inventory", st.InventoryToken, inventoryScopes)
	return map[string]any{
		"travel_via": travelVia, "inventory_via": inventoryVia, "inventory_take": st.InventoryTake,
		"title": st.Title, "home_currency": st.HomeCurrency, "date_format": st.DateFormat, "time_format": st.TimeFormat,
		"distance_unit": st.DistanceUnit, "volume_unit": st.VolumeUnit, "economy_unit": st.EconomyUnit, "fx_auto": st.FXAuto, "fx_provider": s.cfg.FXProvider,
		"firefly_url": st.FireflyURL, "firefly_token": mask(st.FireflyToken), "firefly_configured": st.FireflyURL != "" && st.FireflyToken != "",
		"firefly_category": st.FireflyCategory, "firefly_account": st.FireflyAccount, "firefly_auto_sync": st.FireflyAutoSync,
		"dawarich_url": st.DawarichURL, "dawarich_api_key": mask(st.DawarichAPIKey), "dawarich_configured": st.DawarichURL != "" && st.DawarichAPIKey != "",
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_hour": st.NotifyHour, "notify_digest": st.NotifyDigest, "notify_configured": st.NotifyURL != "",
		"travel_url": st.TravelURL, "travel_public_url": st.TravelPublicURL, "travel_token": mask(st.TravelToken), "travel_configured": travelVia != "",
		"inventory_url": st.InventoryURL, "inventory_token": mask(st.InventoryToken), "inventory_configured": inventoryVia != "",
		"money_enabled": st.MoneyEnabled, "money_url": st.MoneyURL, "money_token": mask(st.MoneyToken), "money_account": st.MoneyAccount,
		"money_category_fuel": st.MoneyCategoryFuel, "money_category": st.MoneyCategory,
		"max_upload_bytes": s.cfg.MaxUploadBytes, "setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL,
	}
}

// handleGetSettings returns the caller's own settings over the box-wide ones.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), u, s.settingsFor(r.Context(), u.ID)))
}

// handlePutSettings saves the caller's own preferences and integrations;
// the box-wide keys (title, exchange-rate lookups) only change for an admin.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	cur := s.settingsFor(r.Context(), u.ID)
	var in store.Settings
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	admin := u.Role == "admin"
	if !admin {
		delete(keys, "title")
		delete(keys, "fx_auto")
	}
	if keys["title"] {
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
	}
	if keys["home_currency"] {
		if !currency.Valid(in.HomeCurrency) {
			writeErr(w, http.StatusBadRequest, "unknown currency")
			return
		}
		cur.HomeCurrency = strings.ToUpper(in.HomeCurrency)
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["distance_unit"] && (in.DistanceUnit == "km" || in.DistanceUnit == "mi") {
		cur.DistanceUnit = in.DistanceUnit
	}
	if keys["volume_unit"] && (in.VolumeUnit == "l" || in.VolumeUnit == "gal_us" || in.VolumeUnit == "gal_uk") {
		cur.VolumeUnit = in.VolumeUnit
	}
	if keys["economy_unit"] && in.EconomyUnit != "" {
		cur.EconomyUnit = in.EconomyUnit
	}
	if keys["fx_auto"] {
		cur.FXAuto = in.FXAuto
	}
	secret := func(k string, dst *string, v string) {
		if keys[k] && !strings.Contains(v, "••") {
			*dst = strings.TrimSpace(v)
		}
	}
	url := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimRight(strings.TrimSpace(v), "/")
		}
	}
	url("firefly_url", &cur.FireflyURL, in.FireflyURL)
	secret("firefly_token", &cur.FireflyToken, in.FireflyToken)
	if keys["firefly_category"] {
		cur.FireflyCategory = strings.TrimSpace(in.FireflyCategory)
	}
	if keys["firefly_account"] {
		cur.FireflyAccount = strings.TrimSpace(in.FireflyAccount)
	}
	if keys["firefly_auto_sync"] {
		cur.FireflyAutoSync = in.FireflyAutoSync
	}
	url("dawarich_url", &cur.DawarichURL, in.DawarichURL)
	secret("dawarich_api_key", &cur.DawarichAPIKey, in.DawarichAPIKey)
	if keys["notify_url"] {
		cur.NotifyURL = strings.TrimSpace(in.NotifyURL)
	}
	if keys["notify_format"] && in.NotifyFormat != "" {
		cur.NotifyFormat = in.NotifyFormat
	}
	if keys["notify_hour"] && in.NotifyHour >= 0 && in.NotifyHour <= 23 {
		cur.NotifyHour = in.NotifyHour
	}
	if keys["notify_digest"] {
		cur.NotifyDigest = in.NotifyDigest
	}
	url("travel_url", &cur.TravelURL, in.TravelURL)
	url("travel_public_url", &cur.TravelPublicURL, in.TravelPublicURL)
	secret("travel_token", &cur.TravelToken, in.TravelToken)
	url("inventory_url", &cur.InventoryURL, in.InventoryURL)
	secret("inventory_token", &cur.InventoryToken, in.InventoryToken)
	if keys["inventory_take"] {
		cur.InventoryTake = in.InventoryTake
	}
	if keys["money_enabled"] {
		cur.MoneyEnabled = in.MoneyEnabled
	}
	url("money_url", &cur.MoneyURL, in.MoneyURL)
	secret("money_token", &cur.MoneyToken, in.MoneyToken)
	if keys["money_account"] && in.MoneyAccount >= 0 {
		cur.MoneyAccount = in.MoneyAccount
	}
	if keys["money_category_fuel"] && in.MoneyCategoryFuel >= 0 {
		cur.MoneyCategoryFuel = in.MoneyCategoryFuel
	}
	if keys["money_category"] && in.MoneyCategory >= 0 {
		cur.MoneyCategory = in.MoneyCategory
	}
	if admin && (keys["title"] || keys["fx_auto"]) {
		if err := s.store.SaveSettings(r.Context(), cur); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	if err := s.store.SaveUserSettings(r.Context(), u.ID, cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	_ = s.store.CacheDelete(r.Context(), fmt.Sprintf("travel:trips:%d", u.ID))
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), u, s.settingsFor(r.Context(), u.ID)))
}

// handleIntegrationTest checks a connection with the caller's own saved settings
// (or the values typed into the form).
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Which    string `json:"which"`
		URL      string `json:"url"`
		Key      string `json:"key"`
		Format   string `json:"format"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := s.settingsFor(r.Context(), userFrom(r.Context()).ID)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	keyOr := func(saved string) string {
		if strings.Contains(in.Key, "••") || in.Key == "" {
			return saved
		}
		return in.Key
	}
	urlOr := func(saved string) string {
		if strings.TrimSpace(in.URL) == "" {
			return saved
		}
		return in.URL
	}
	var msg string
	var err error
	switch in.Which {
	case "firefly":
		var v string
		v, err = firefly.New(urlOr(st.FireflyURL), keyOr(st.FireflyToken), 15*time.Second).About(ctx)
		msg = "Firefly III " + v
	case "dawarich":
		c := dawarich.New(urlOr(st.DawarichURL), keyOr(st.DawarichAPIKey), 15*time.Second)
		err = c.Health(ctx)
		if err == nil {
			var days []dawarich.Day
			days, err = c.Timeline(ctx, time.Now().AddDate(0, 0, -7), time.Now())
			km := 0.0
			for _, d := range days {
				km += d.Summary.ModeDistances["driving"]
			}
			msg = fmt.Sprintf("Dawarich reachable, %.0f km driven in the last 7 days", km)
		}
	case "notify":
		f := in.Format
		if f == "" {
			f = st.NotifyFormat
		}
		n := notify.New(urlOr(st.NotifyURL), f)
		err = n.Send(ctx, notify.Message{Title: "Pocket Garage", Body: "Notifications work. Reminders will arrive here.", Priority: "info", Tags: []string{"wrench"}})
		msg = "Test message sent via " + n.Format
	case "travel", "inventory":
		name := map[string]string{"travel": "Pocket Travel", "inventory": "Pocket Inventory"}[in.Which]
		saved := map[string][2]string{"travel": {st.TravelURL, st.TravelToken}, "inventory": {st.InventoryURL, st.InventoryToken}}[in.Which]
		var v string
		if tok := keyOr(saved[1]); tok != "" {
			base := urlOr(saved[0])
			if base == "" {
				base = s.resolveSibling(ctx, in.Which)
			}
			v, err = pocket.New(base, tok, 15*time.Second).Health(ctx)
			msg = name + " " + v + " with your token"
		} else {
			u := userFrom(r.Context())
			var conn *siblingConn
			if in.Which == "travel" {
				conn, err = s.travelFor(ctx, u)
			} else {
				conn, err = s.inventoryFor(ctx, u)
			}
			if err == nil {
				v, err = conn.client.Health(ctx)
				msg = name + " " + v + " through Pocket Core"
			}
		}
	case "money":
		var accts []money.Account
		if tok := keyOr(st.MoneyToken); tok != "" {
			base := urlOr(st.MoneyURL)
			if base == "" {
				base = s.resolveSibling(ctx, "money")
			}
			accts, err = money.New(base, tok, 15*time.Second).Accounts(ctx)
			msg = fmt.Sprintf("Pocket Money reachable with your token, %d accounts to book on", len(accts))
		} else {
			var ml *moneyLink
			if ml, err = s.moneyFor(ctx, userFrom(r.Context())); err == nil {
				accts, err = ml.client.Accounts(ctx)
				msg = fmt.Sprintf("Connected through Pocket Core, %d accounts to book on", len(accts))
			}
		}
	case "lubelogger":
		var vs []lubelogger.Vehicle
		vs, err = lubelogger.New(in.URL, in.Username, in.Password, 15*time.Second).Vehicles(ctx)
		msg = fmt.Sprintf("LubeLogger reachable, %d vehicles", len(vs))
	default:
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// handleSystemInfo: version for everyone; paths, sizes and row counts of the
// whole box only for an admin.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if u := userFrom(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusOK, map[string]any{"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
			"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String()})
		return
	}
	var size, files int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	_ = filepath.WalkDir(s.cfg.FilesDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				files += fi.Size()
			}
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size, "files_bytes": files,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.Counts(r.Context()),
	})
}

// handleBackup downloads the whole database: everyone's data and credentials,
// so it is the box admin's maintenance tool only.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-garage-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	defer os.Remove(dst)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, dst)
}

// handleMeta returns the static lists the UI needs once.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"vehicle_kinds": store.VehicleKinds, "fuel_types": store.FuelTypes, "record_kinds": store.RecordKinds, "plan_stages": store.PlanStages,
		"priorities": store.Priorities, "document_kinds": store.DocumentKinds, "service_categories": store.ServiceCategory, "expense_categories": store.ExpenseCategory,
		"reminder_templates": ReminderTemplates,
	})
}

// ReminderTemplate is a common recurring job.
type ReminderTemplate struct {
	Title        string  `json:"title"`
	Kind         string  `json:"kind"`
	Category     string  `json:"category"`
	IntervalDays int     `json:"interval_days"`
	IntervalKm   float64 `json:"interval_km"`
}

// ReminderTemplates are offered when creating a reminder.
var ReminderTemplates = []ReminderTemplate{
	{"Oil & filter change", "service", "oil", 365, 15000},
	{"Annual inspection / MOT", "expense", "inspection", 365, 0},
	{"Registration renewal", "expense", "registration", 365, 0},
	{"Insurance renewal", "expense", "insurance", 365, 0},
	{"Tyre rotation", "service", "tyres", 180, 10000},
	{"Summer ↔ winter tyres", "service", "tyres", 182, 0},
	{"Brake fluid", "service", "brakes", 730, 0},
	{"Coolant", "service", "coolant", 1095, 60000},
	{"Timing belt", "service", "timing belt", 1825, 120000},
	{"Air & cabin filters", "service", "filters", 365, 20000},
	{"Spark plugs", "service", "spark plugs", 0, 60000},
	{"Battery check", "service", "battery", 365, 0},
	{"Wash & wax", "expense", "wash", 30, 0},
}

func (s *Server) handleCurrencies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currency.All())
}

func (s *Server) handleFX(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to, date := strings.ToUpper(q.Get("from")), strings.ToUpper(q.Get("to")), q.Get("date")
	if !currency.Valid(from) || !currency.Valid(to) {
		writeErr(w, http.StatusBadRequest, "unknown currency")
		return
	}
	rate, src := s.rate(r.Context(), from, to, date)
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "date": date, "rate": rate, "source": src})
}

// ---- home + dashboard ----

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	vs, err := s.visibleVehicles(ctx, u, "")
	if err != nil {
		s.fail(w, err, "vehicles")
		return
	}
	st := s.settingsFor(ctx, u.ID)
	stats := store.HomeStats{}
	ids := []int64{}
	var reminders []store.Reminder
	today := store.Today()
	for i := range vs {
		v := &vs[i]
		if v.Status == "archived" {
			continue
		}
		stats.Vehicles++
		ids = append(ids, v.ID)
		sum, err := s.store.Summary(ctx, v)
		if err != nil {
			continue
		}
		v.Summary = sum
		if v.Status == "active" {
			stats.Active++
		}
		rate, _ := s.rate(ctx, v.Currency, st.HomeCurrency, today)
		stats.CostYear += convert(sum.CostYear, v.Currency, st.HomeCurrency, rate)
		stats.CostMonth += convert(sum.CostMonth, v.Currency, st.HomeCurrency, rate)
		stats.Due += sum.RemindersDue
		stats.Soon += sum.RemindersSoon
		rs, _ := s.store.ListReminders(ctx, v.ID, "active")
		for j := range rs {
			store.ComputeUrgency(&rs[j], today, sum.OdometerEst)
			rs[j].VehicleName = v.Title()
			if rs[j].Urgency != "ok" {
				reminders = append(reminders, rs[j])
			}
		}
	}
	sortReminders(reminders)
	if reminders == nil {
		reminders = []store.Reminder{}
	}
	recent, _ := s.store.RecentEntries(ctx, ids, 12)
	active := []store.Vehicle{}
	for _, v := range vs {
		if v.Status != "archived" {
			active = append(active, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": stats, "vehicles": active, "reminders": reminders, "recent": recent, "currency": st.HomeCurrency})
}

// handleDashboard is the flat shape a Homepage customapi widget reads.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	vs, err := s.visibleVehicles(ctx, u, "active")
	if err != nil {
		s.fail(w, err, "vehicles")
		return
	}
	st := s.settingsFor(ctx, u.ID)
	out := map[string]any{"vehicles": len(vs), "currency": st.HomeCurrency}
	var costYear, costMonth int64
	due, soon := 0, 0
	var next *store.Reminder
	nextVehicle := ""
	var odometer float64
	var economy float64
	today := store.Today()
	for i := range vs {
		v := &vs[i]
		sum, err := s.store.Summary(ctx, v)
		if err != nil {
			continue
		}
		rate, _ := s.rate(ctx, v.Currency, st.HomeCurrency, today)
		costYear += convert(sum.CostYear, v.Currency, st.HomeCurrency, rate)
		costMonth += convert(sum.CostMonth, v.Currency, st.HomeCurrency, rate)
		due += sum.RemindersDue
		soon += sum.RemindersSoon
		if sum.NextReminder != nil && (next == nil || store.UrgencyRank[sum.NextReminder.Urgency] < store.UrgencyRank[next.Urgency]) {
			next = sum.NextReminder
			nextVehicle = v.Title()
		}
		if i == 0 {
			odometer = sum.OdometerEst
			economy = sum.EconomyRecent
			if economy == 0 {
				economy = sum.Economy
			}
			out["vehicle_name"] = v.Title()
			out["odometer_unit"] = v.OdometerUnit
			if sum.LastFuel != nil {
				out["last_fuel_date"] = sum.LastFuel.Date
				out["last_fuel_price"] = majorString(sum.LastFuel.CostHome, v.Currency)
			}
		}
	}
	dec := currency.Decimals(st.HomeCurrency)
	out["cost_year"] = float64(costYear) / math.Pow10(dec)
	out["cost_month"] = float64(costMonth) / math.Pow10(dec)
	out["cost_year_text"] = majorString(costYear, st.HomeCurrency)
	out["cost_month_text"] = majorString(costMonth, st.HomeCurrency)
	out["reminders_due"] = due
	out["reminders_soon"] = soon
	out["odometer"] = math.Round(odometer)
	out["economy"] = math.Round(economy*10) / 10
	if next != nil {
		txt := next.Title
		if next.DaysLeft != nil {
			switch {
			case *next.DaysLeft < 0:
				txt += fmt.Sprintf(" (%d d overdue)", -*next.DaysLeft)
			case *next.DaysLeft == 0:
				txt += " (today)"
			default:
				txt += fmt.Sprintf(" (in %d d)", *next.DaysLeft)
			}
		} else if next.KmLeft != nil {
			txt += fmt.Sprintf(" (in %.0f km)", *next.KmLeft)
		}
		out["next_reminder"] = txt
		out["next_reminder_vehicle"] = nextVehicle
		out["next_reminder_urgency"] = next.Urgency
	} else {
		out["next_reminder"] = "nothing due"
		out["next_reminder_urgency"] = "ok"
	}
	writeJSON(w, http.StatusOK, out)
}

func majorString(minor int64, code string) string {
	dec := currency.Decimals(code)
	return strconv.FormatFloat(float64(minor)/math.Pow10(dec), 'f', dec, 64) + " " + code
}

// ---- Firefly ----

func (s *Server) handleFireflyTags(w http.ResponseWriter, r *http.Request) {
	cl := s.clientsFor(r.Context(), userFrom(r.Context()).ID)
	if !cl.firefly.Configured() {
		writeJSON(w, http.StatusOK, []firefly.Tag{})
		return
	}
	tags, err := cl.firefly.Tags(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleFireflyAccounts(w http.ResponseWriter, r *http.Request) {
	cl := s.clientsFor(r.Context(), userFrom(r.Context()).ID)
	if !cl.firefly.Configured() {
		writeJSON(w, http.StatusOK, []firefly.Account{})
		return
	}
	acc, err := cl.firefly.Accounts(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) handleFireflyCategories(w http.ResponseWriter, r *http.Request) {
	cl := s.clientsFor(r.Context(), userFrom(r.Context()).ID)
	if !cl.firefly.Configured() {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	c, err := cl.firefly.Categories(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// fireflySyncResult says what a sync did.
type fireflySyncResult struct {
	Fetched   int      `json:"fetched"`
	Matched   int      `json:"matched"`
	Created   int      `json:"created"`
	Unmatched int      `json:"unmatched"`
	Error     string   `json:"error,omitempty"`
	Notes     []string `json:"notes,omitempty"`
}

// fuelWords mark a Firefly description as a fill-up.
var fuelWords = []string{"gorivo", "fuel", "benzin", "dizel", "diesel", "petrol", "gas station", "tank", "eurodizel", "ina ", "petrol ", "shell", "omv", "mol ", "nestro", "g-petrol", "hifa", "charging", "supercharger", "ionity"}

func looksLikeFuel(desc string) bool {
	d := strings.ToLower(desc)
	for _, w := range fuelWords {
		if strings.Contains(d, w) {
			return true
		}
	}
	return false
}

// syncFirefly pulls the withdrawals carrying the vehicle's tag. A transaction
// that matches an existing fuel/record row (same date ±1 day, same amount
// and currency) is linked to it; the rest become expense records so nothing
// paid is missing from the vehicle's total.
func (s *Server) syncFirefly(ctx context.Context, vehicleID int64) (*fireflySyncResult, error) {
	s.syncMu.Lock()
	if s.syncing[vehicleID] {
		s.syncMu.Unlock()
		return &fireflySyncResult{Notes: []string{"a sync is already running"}}, nil
	}
	s.syncing[vehicleID] = true
	s.syncMu.Unlock()
	defer func() {
		s.syncMu.Lock()
		delete(s.syncing, vehicleID)
		s.syncMu.Unlock()
	}()
	v, err := s.store.GetVehicle(ctx, vehicleID, 0)
	if err != nil {
		return nil, err
	}
	cl := s.clientsFor(ctx, v.OwnerID) // the owner's Firefly, whoever pressed sync
	if !cl.firefly.Configured() || v.FireflyTag == "" {
		return &fireflySyncResult{Notes: []string{"Firefly is not configured or the vehicle has no tag"}}, nil
	}
	tag, err := cl.firefly.TagByName(ctx, v.FireflyTag)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return &fireflySyncResult{Notes: []string{"tag not found in Firefly: " + v.FireflyTag}}, nil
	}
	start := v.PurchaseDate
	if start == "" {
		start = "2000-01-01"
	}
	txs, err := cl.firefly.TagTransactions(ctx, tag.ID, start, "")
	if err != nil {
		return nil, err
	}
	res := &fireflySyncResult{Fetched: len(txs)}
	records, _ := s.store.ListRecords(ctx, v.ID, store.RecordFilter{})
	fuel, _ := s.store.ListFuel(ctx, v.ID)
	amountMinor := func(t firefly.Transaction) (int64, string) {
		dec := t.Decimals
		if dec == 0 {
			dec = currency.Decimals(t.Currency)
		}
		return int64(math.Round(t.Amount * math.Pow10(dec))), strings.ToUpper(t.Currency)
	}
	near := func(a, b string) bool { return store.DaysBetween(a, b) <= 1 || store.DaysBetween(b, a) <= 1 }
	var unmatched []map[string]any
	for _, t := range txs {
		if _, _, ok := s.store.RecordByFirefly(ctx, v.ID, t.ID); ok {
			res.Matched++
			continue
		}
		amt, cur := amountMinor(t)
		linked := false
		for i := range fuel {
			f := &fuel[i]
			if f.FireflyID == "" && f.Cost == amt && f.Currency == cur && near(f.Date, t.Date) {
				f.FireflyID = t.ID
				_ = s.store.UpdateFuel(ctx, f)
				linked = true
				break
			}
		}
		if !linked {
			for i := range records {
				rc := &records[i]
				if rc.FireflyID == "" && rc.Cost == amt && rc.Currency == cur && near(rc.Date, t.Date) {
					rc.FireflyID = t.ID
					_ = s.store.UpdateRecord(ctx, rc)
					linked = true
					break
				}
			}
		}
		if linked {
			res.Matched++
			continue
		}
		if looksLikeFuel(t.Description) {
			// a fill-up without litres cannot be a fuel row; keep it visible for the user to complete
			unmatched = append(unmatched, map[string]any{"id": t.ID, "date": t.Date, "description": t.Description, "amount": amt, "currency": cur, "category": t.Category, "hint": "fuel"})
			res.Unmatched++
			continue
		}
		rate, _ := s.rate(ctx, cur, v.Currency, t.Date)
		rec := &store.Record{VehicleID: v.ID, Kind: "expense", Category: guessCategory(t), Date: t.Date, Description: t.Description, Notes: strings.TrimSpace(t.Notes), Vendor: t.Destination, Cost: amt, Currency: cur, FXRate: rate, CostHome: convert(amt, cur, v.Currency, rate), Source: "firefly", ExternalID: t.ID, FireflyID: t.ID, Parts: []store.Part{}}
		if err := s.store.CreateRecord(ctx, rec); err == nil {
			res.Created++
			s.store.LogActivity(ctx, v.ID, 0, "imported from Firefly", rec.Description)
		}
	}
	b, _ := json.Marshal(unmatched)
	_ = s.store.CachePut(ctx, fmt.Sprintf("firefly:unmatched:%d", v.ID), string(b))
	_ = s.store.CachePut(ctx, fmt.Sprintf("firefly:synced:%d", v.ID), store.Now())
	return res, nil
}

func guessCategory(t firefly.Transaction) string {
	d := strings.ToLower(t.Description + " " + t.Category)
	switch {
	case strings.Contains(d, "registr"):
		return "registration"
	case strings.Contains(d, "osigur") || strings.Contains(d, "insur"):
		return "insurance"
	case strings.Contains(d, "pranje") || strings.Contains(d, "wash"):
		return "wash"
	case strings.Contains(d, "parking"):
		return "parking"
	case strings.Contains(d, "putarin") || strings.Contains(d, "toll"):
		return "toll"
	case strings.Contains(d, "kazna") || strings.Contains(d, "fine"):
		return "fine"
	case strings.Contains(d, "tehnič") || strings.Contains(d, "tehnic") || strings.Contains(d, "inspection"):
		return "inspection"
	}
	return ""
}

func (s *Server) handleFireflySync(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	res, err := s.syncFirefly(r.Context(), vehicleFrom(r.Context()).vehicle.ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleFireflyUnmatched(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	val, at, _ := s.store.CacheGet(r.Context(), fmt.Sprintf("firefly:unmatched:%d", v.ID))
	var list []map[string]any
	_ = json.Unmarshal([]byte(val), &list)
	if list == nil {
		list = []map[string]any{}
	}
	synced, _, _ := s.store.CacheGet(r.Context(), fmt.Sprintf("firefly:synced:%d", v.ID))
	writeJSON(w, http.StatusOK, map[string]any{"unmatched": list, "fetched_at": at, "synced_at": synced})
}

// autoSync refreshes Firefly and Dawarich person by person: each owner's
// vehicles with that owner's own credentials.
func (s *Server) autoSync(ctx context.Context) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	for _, u := range users {
		st := s.settingsFor(ctx, u.ID)
		fireflyOn := st.FireflyAutoSync && st.FireflyToken != ""
		dawarichOn := st.DawarichURL != "" && st.DawarichAPIKey != ""
		if !fireflyOn && !dawarichOn {
			continue
		}
		vs, err := s.store.ListVehicles(ctx, store.VehicleFilter{OwnerID: u.ID})
		if err != nil {
			continue
		}
		for i := range vs {
			v := &vs[i]
			if v.Status == "archived" {
				continue
			}
			if fireflyOn && v.FireflyTag != "" {
				if _, err := s.syncFirefly(ctx, v.ID); err != nil {
					s.log.Warn("firefly auto sync", "vehicle", v.Title(), "err", err)
				}
			}
			if dawarichOn && v.Dawarich {
				if _, err := s.syncDawarich(ctx, v); err != nil {
					s.log.Warn("dawarich sync", "vehicle", v.Title(), "err", err)
				}
			}
		}
	}
}

// pushFirefly creates a withdrawal for a record or fill-up in the owner's Firefly.
func (s *Server) pushFirefly(ctx context.Context, v *store.Vehicle, date, desc string, amount int64, cur, notes string, kind string) (string, error) {
	cl := s.clientsFor(ctx, v.OwnerID)
	if !cl.firefly.Configured() {
		return "", fmt.Errorf("firefly is not configured")
	}
	acct := v.FireflyAccount
	if acct == "" {
		acct = cl.st.FireflyAccount
	}
	dec := currency.Decimals(cur)
	tags := []string{}
	if v.FireflyTag != "" {
		tags = append(tags, v.FireflyTag)
	}
	w := firefly.Withdrawal{Date: date, Amount: strconv.FormatFloat(float64(amount)/math.Pow10(dec), 'f', dec, 64), Currency: cur, Description: desc, Source: acct, Category: cl.st.FireflyCategory, Tags: tags, Notes: strings.TrimSpace("Pocket Garage: " + kind + "\n" + notes)}
	return cl.firefly.CreateWithdrawal(ctx, w)
}

func (s *Server) handlePushRecordFirefly(w http.ResponseWriter, r *http.Request) {
	if !s.isOwner(r) {
		writeErr(w, http.StatusForbidden, "only the owner can send to their Firefly")
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	rec, err := s.store.GetRecord(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "record")
		return
	}
	if rec.FireflyID != "" {
		writeErr(w, http.StatusConflict, "already in Firefly")
		return
	}
	if rec.Cost <= 0 {
		writeErr(w, http.StatusBadRequest, "nothing to push: the record has no cost")
		return
	}
	id, err := s.pushFirefly(r.Context(), v, rec.Date, v.Title()+" "+rec.Description, rec.Cost, rec.Currency, rec.Notes, rec.Kind)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	rec.FireflyID = id
	_ = s.store.UpdateRecord(r.Context(), rec)
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "sent to Firefly", rec.Description)
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handlePushFuelFirefly(w http.ResponseWriter, r *http.Request) {
	if !s.isOwner(r) {
		writeErr(w, http.StatusForbidden, "only the owner can send to their Firefly")
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	f, err := s.store.GetFuel(r.Context(), v.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "fuel")
		return
	}
	if f.FireflyID != "" {
		writeErr(w, http.StatusConflict, "already in Firefly")
		return
	}
	desc := v.Title() + " gorivo"
	if f.Station != "" {
		desc = v.Title() + " fuel · " + f.Station
	}
	id, err := s.pushFirefly(r.Context(), v, f.Date, desc, f.Cost, f.Currency, fmt.Sprintf("%.2f l at %.0f %s", f.Volume, f.Odometer, v.OdometerUnit), "fuel")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	f.FireflyID = id
	_ = s.store.UpdateFuel(r.Context(), f)
	s.store.LogActivity(r.Context(), v.ID, userFrom(r.Context()).ID, "sent to Firefly", "fill-up")
	writeJSON(w, http.StatusOK, s.fuelWithStats(r.Context(), v.ID, f.ID))
}

// ---- Dawarich ----

// syncDawarich stores the driving km per day since the last stored day
// (re-reading the last 3 days, since Dawarich keeps receiving points).
func (s *Server) syncDawarich(ctx context.Context, v *store.Vehicle) (int, error) {
	cl := s.clientsFor(ctx, v.OwnerID) // the owner's Dawarich, whoever pressed sync
	if !cl.dawarich.Configured() {
		return 0, fmt.Errorf("dawarich is not configured")
	}
	last := s.store.LastDawarichDay(ctx, v.ID)
	from := time.Now().AddDate(0, 0, -90)
	if last != "" {
		if t, err := store.ParseDate(last); err == nil {
			from = t.AddDate(0, 0, -3)
		}
	}
	if v.PurchaseDate != "" {
		if t, err := store.ParseDate(v.PurchaseDate); err == nil && t.After(from) {
			from = t
		}
	}
	// the earliest confirmed reading bounds how far back the estimate can matter
	n := 0
	to := time.Now()
	// Dawarich refuses ranges longer than 31 days; walk in 28-day windows.
	for cur := from; !cur.After(to); cur = cur.AddDate(0, 0, 28) {
		end := cur.AddDate(0, 0, 28)
		if end.After(to) {
			end = to
		}
		days, err := cl.dawarich.Timeline(ctx, cur, end)
		if err != nil {
			return n, err
		}
		for _, d := range days {
			km := d.Summary.ModeDistances["driving"]
			if km == 0 {
				// older Dawarich without modes: fall back to the day total when it is clearly a drive
				if d.Summary.TotalDistance > 15 {
					km = d.Summary.TotalDistance
				}
			}
			if d.Summary.DistanceUnit == "mi" {
				km *= 1.609344
			}
			if v.OdometerUnit == "mi" {
				km /= 1.609344
			}
			if err := s.store.PutDawarichDay(ctx, v.ID, d.Date, km, d.Summary.TotalDistance); err == nil {
				n++
			}
		}
	}
	return n, nil
}

func (s *Server) handleDawarichSync(w http.ResponseWriter, r *http.Request) {
	if !s.canEdit(w, r) {
		return
	}
	v := vehicleFrom(r.Context()).vehicle
	n, err := s.syncDawarich(r.Context(), v)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	reading, date := s.store.LatestOdometer(r.Context(), v.ID)
	est := s.store.DawarichKmSince(r.Context(), v.ID, date)
	writeJSON(w, http.StatusOK, map[string]any{"days": n, "estimate_km": est, "estimate": reading + est})
}

func (s *Server) handleDawarichDays(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if to == "" {
		to = store.Today()
	}
	if from == "" {
		from = addDays(to, -90)
	}
	days, err := s.store.DawarichDays(r.Context(), v.ID, from, to)
	if err != nil {
		s.fail(w, err, "days")
		return
	}
	writeJSON(w, http.StatusOK, days)
}

// ---- Pocket Travel ----

type cachedTrips struct {
	Trips  []pocket.Trip `json:"trips"`
	Public string        `json:"public"`
}

// trips returns a person's cached trip list (refreshed hourly) and where
// their Pocket Travel opens in a browser.
func (s *Server) trips(ctx context.Context, u *store.User) ([]pocket.Trip, string) {
	if u == nil {
		return nil, ""
	}
	key := fmt.Sprintf("travel:trips:%d", u.ID)
	if val, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < time.Hour {
			var c cachedTrips
			if json.Unmarshal([]byte(val), &c) == nil && c.Trips != nil {
				return c.Trips, c.Public
			}
		}
	}
	conn, err := s.travelFor(ctx, u)
	if err != nil {
		return nil, ""
	}
	trips, err := conn.client.Trips(ctx)
	if err != nil {
		s.log.Warn("pocket travel", "via", conn.via, "err", err)
		return nil, ""
	}
	if trips == nil {
		trips = []pocket.Trip{}
	}
	b, _ := json.Marshal(cachedTrips{Trips: trips, Public: conn.public})
	_ = s.store.CachePut(ctx, key, string(b))
	return trips, conn.public
}

// tripsFor lists the viewer's own trips with what this vehicle spent during each.
func (s *Server) tripsFor(ctx context.Context, u *store.User, v *store.Vehicle) []map[string]any {
	trips, public := s.trips(ctx, u)
	out := []map[string]any{}
	if len(trips) == 0 {
		return out
	}
	fuel, _ := s.store.ListFuel(ctx, v.ID)
	recs, _ := s.store.ListRecords(ctx, v.ID, store.RecordFilter{})
	for _, t := range trips {
		if t.StartDate == "" || t.EndDate == "" {
			continue
		}
		var fuelCost, otherCost int64
		var litres, km float64
		fills := 0
		for _, f := range fuel {
			if f.Date >= t.StartDate && f.Date <= t.EndDate {
				fuelCost += f.CostHome
				litres += f.Volume
				fills++
			}
		}
		for _, rc := range recs {
			if rc.Date >= t.StartDate && rc.Date <= t.EndDate {
				otherCost += rc.CostHome
			}
		}
		km = s.store.DawarichKmBetween(ctx, v.ID, t.StartDate, t.EndDate)
		if km == 0 {
			startOdo, startDate := s.store.OdometerAt(ctx, v.ID, t.StartDate)
			endOdo, endDate := s.store.OdometerAt(ctx, v.ID, addDays(t.EndDate, 3))
			if startOdo > 0 && endOdo > startOdo && store.DaysBetween(startDate, t.StartDate) <= 3 && endDate >= t.EndDate {
				km = endOdo - startOdo
			}
		}
		if fills == 0 && otherCost == 0 && km == 0 {
			continue
		}
		link := ""
		if base := public; base != "" {
			link = fmt.Sprintf("%s/trips/%d", base, t.ID)
		}
		out = append(out, map[string]any{"id": t.ID, "name": t.Name, "destination": t.Destination, "start_date": t.StartDate, "end_date": t.EndDate, "color": t.Color, "fuel_cost": fuelCost, "other_cost": otherCost, "litres": litres, "fills": fills, "km": km, "link": link})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["start_date"].(string) > out[j]["start_date"].(string) })
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Server) handleVehicleTrips(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	writeJSON(w, http.StatusOK, s.tripsFor(r.Context(), userFrom(r.Context()), v))
}

// ---- Pocket Inventory ----

func (s *Server) handleInventorySearch(w http.ResponseWriter, r *http.Request) {
	conn, err := s.inventoryFor(r.Context(), userFrom(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []pocket.Item{}, "error": err.Error()})
		return
	}
	items, err := conn.client.SearchItems(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), 15)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if items == nil {
		items = []pocket.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "url": conn.public})
}

// ---- notifications ----

// checkReminders pushes one message per vehicle the person owns for reminders
// whose urgency rose since the last push (ok → soon → due → overdue), to that
// person's own notification target. With force every non-ok reminder is included.
func (s *Server) checkReminders(ctx context.Context, userID int64, force bool) (int, error) {
	cl := s.clientsFor(ctx, userID)
	if !cl.notify.Configured() {
		return 0, fmt.Errorf("notifications are not configured")
	}
	vs, err := s.store.ListVehicles(ctx, store.VehicleFilter{OwnerID: userID})
	if err != nil {
		return 0, err
	}
	today := store.Today()
	sent := 0
	weekday := time.Now().Weekday() == time.Monday
	for i := range vs {
		v := &vs[i]
		if v.Status == "archived" {
			continue
		}
		reading, date := s.store.LatestOdometer(ctx, v.ID)
		est := reading + s.store.DawarichKmSince(ctx, v.ID, date)
		rems, _ := s.store.ListReminders(ctx, v.ID, "active")
		var lines []string
		worst := "ok"
		for j := range rems {
			rm := &rems[j]
			store.ComputeUrgency(rm, today, est)
			if rm.Urgency == "ok" {
				continue
			}
			rose := store.UrgencyRank[rm.Urgency] < store.UrgencyRank[orOK(rm.NotifiedLevel)]
			repeat := rm.Urgency == "overdue" && weekday && cl.st.NotifyDigest
			if !force && !rose && !repeat {
				continue
			}
			lines = append(lines, describeReminder(rm, v))
			if store.UrgencyRank[rm.Urgency] < store.UrgencyRank[worst] {
				worst = rm.Urgency
			}
			rm.NotifiedLevel = rm.Urgency
			rm.NotifiedAt = store.Now()
			_ = s.store.UpdateReminder(ctx, rm)
		}
		docs, _ := s.store.ExpiringDocuments(ctx, v.ID, addDays(today, 30))
		for _, d := range docs {
			key := fmt.Sprintf("notified:doc:%d:%s", d.ID, d.ExpiresOn)
			if _, _, ok := s.store.CacheGet(ctx, key); ok && !force {
				continue
			}
			left := store.DaysBetween(today, d.ExpiresOn)
			if d.ExpiresOn < today {
				lines = append(lines, fmt.Sprintf("📄 %s expired on %s", d.Name, d.ExpiresOn))
				worst = "overdue"
			} else {
				lines = append(lines, fmt.Sprintf("📄 %s expires in %d days (%s)", d.Name, left, d.ExpiresOn))
				if worst == "ok" {
					worst = "soon"
				}
			}
			_ = s.store.CachePut(ctx, key, today)
		}
		if len(lines) == 0 {
			continue
		}
		prio := map[string]string{"overdue": "failure", "due": "warning", "soon": "info"}[worst]
		title := fmt.Sprintf("%s: %d thing%s to do", v.Title(), len(lines), plural(len(lines)))
		link := ""
		if s.cfg.BaseURL != "" {
			link = fmt.Sprintf("%s/vehicles/%d/reminders", s.cfg.BaseURL, v.ID)
		}
		if err := cl.notify.Send(ctx, notify.Message{Title: title, Body: strings.Join(lines, "\n"), Priority: prio, Tags: []string{"wrench"}, Link: link}); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func orOK(s string) string {
	if s == "" {
		return "ok"
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func describeReminder(rm *store.Reminder, v *store.Vehicle) string {
	icon := map[string]string{"overdue": "🔴", "due": "🟠", "soon": "🟡"}[rm.Urgency]
	parts := []string{}
	if rm.DaysLeft != nil {
		switch {
		case *rm.DaysLeft < 0:
			parts = append(parts, fmt.Sprintf("%d days overdue", -*rm.DaysLeft))
		case *rm.DaysLeft == 0:
			parts = append(parts, "due today")
		default:
			parts = append(parts, fmt.Sprintf("in %d days", *rm.DaysLeft))
		}
	}
	if rm.KmLeft != nil {
		if *rm.KmLeft < 0 {
			parts = append(parts, fmt.Sprintf("%.0f %s over", -*rm.KmLeft, v.OdometerUnit))
		} else {
			parts = append(parts, fmt.Sprintf("in %.0f %s", *rm.KmLeft, v.OdometerUnit))
		}
	}
	return fmt.Sprintf("%s %s — %s", icon, rm.Title, strings.Join(parts, ", "))
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	cl := s.clientsFor(r.Context(), userFrom(r.Context()).ID)
	if err := cl.notify.Send(r.Context(), notify.Message{Title: "Pocket Garage", Body: "Test notification. Reminders will arrive like this.", Priority: "info", Tags: []string{"wrench"}}); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleNotifyRun(w http.ResponseWriter, r *http.Request) {
	n, err := s.checkReminders(r.Context(), userFrom(r.Context()).ID, queryBool(r, "force"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"sent": n})
}

// ---- exports ----

func (s *Server) handleExportICS(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	s.writeICS(w, r, v)
}

func (s *Server) writeICS(w http.ResponseWriter, r *http.Request, v *store.Vehicle) {
	rems, _ := s.store.ListReminders(r.Context(), v.ID, "active")
	docs, _ := s.store.ListDocuments(r.Context(), v.ID)
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="`+safeName(v.Title())+`.ics"`)
	_, _ = w.Write([]byte(export.ICS(v, rems, docs, s.cfg.BaseURL)))
}

func (s *Server) handleCalendarFeed(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	u, err := s.store.GetUserByAPIToken(r.Context(), tok)
	if err != nil || u == nil {
		writeErr(w, http.StatusUnauthorized, "a valid API token is required (?token=…)")
		return
	}
	id := idParam(r, "vehicleID")
	if s.store.VehicleRole(r.Context(), id, u) == "" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	v, err := s.store.GetVehicle(r.Context(), id, u.ID)
	if err != nil {
		s.fail(w, err, "vehicle")
		return
	}
	s.writeICS(w, r, v)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	v := vehicleFrom(r.Context()).vehicle
	kind := chi.URLParam(r, "kind")
	dec := currency.Decimals(v.Currency)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(v.Title())+"-"+kind+`.csv"`)
	switch kind {
	case "records":
		rs, _ := s.store.ListRecords(r.Context(), v.ID, store.RecordFilter{Kind: r.URL.Query().Get("kind")})
		_ = export.RecordsCSV(w, rs, dec)
	case "fuel":
		fs, _ := s.store.ListFuel(r.Context(), v.ID)
		_ = export.FuelCSV(w, fs, dec)
	case "odometer":
		os, _ := s.store.OdometerHistory(r.Context(), v.ID)
		_ = export.OdometerCSV(w, os)
	default:
		writeErr(w, http.StatusNotFound, "unknown export; use records, fuel or odometer")
	}
}

func safeName(s string) string {
	s = strings.Map(func(c rune) rune {
		if c == '"' || c == '/' || c == '\\' || c < 32 {
			return '-'
		}
		return c
	}, s)
	if s == "" {
		return "vehicle"
	}
	return s
}

// coreStatus summarises the Pocket Core link for the About page.
func (s *Server) coreStatus() map[string]any {
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return map[string]any{"configured": s.core.Configured(), "url": s.cfg.CoreURL, "registered": s.coreOK, "error": s.coreErr}
}
