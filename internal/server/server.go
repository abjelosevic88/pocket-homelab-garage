// Package server wires the HTTP API, auth and the embedded SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/config"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/fx"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/lubelogger"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// Version is injected at build time.
var Version = "dev"

// Server holds dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	log      *slog.Logger
	webFS    fs.FS
	started  time.Time
	requests atomic.Int64
	errors   atomic.Int64
	fx       *fx.Client

	syncMu  sync.Mutex
	syncing map[int64]bool

	oidcMu sync.Mutex
	oidc   *oidcProvider

	core     *core.Client
	coreMu   sync.Mutex
	coreOK   bool
	coreErr  string
	resolved map[string]resolvedURL

	modesKick chan struct{} // a connection-mode report is due (modes.go)
}

type resolvedURL struct {
	url string
	at  time.Time
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	return &Server{cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now(), fx: fx.New(cfg.FXProvider, cfg.UserAgent, 8*time.Second), syncing: map[int64]bool{},
		oidc:     &oidcProvider{issuerURL: cfg.OIDCIssuer},
		core:     core.New(cfg.CoreURL, core.Options{DB: st.DB, TokenFile: cfg.DataDir + "/core.token", UserAgent: cfg.UserAgent}),
		resolved: map[string]resolvedURL{}, modesKick: make(chan struct{}, 1)}
}

// Bootstrap seeds the first admin (if configured), integration defaults,
// an initial LubeLogger import and demo data.
func (s *Server) Bootstrap(ctx context.Context) error {
	if err := s.core.Start(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(s.cfg.FilesDir, 0o755); err != nil {
		return fmt.Errorf("files dir: %w", err)
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && s.cfg.AuthMode == "local" && s.cfg.AdminEmail != "" && s.cfg.AdminPassword != "" {
		if len(s.cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD must be at least 8 characters")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
		u := &store.User{Email: s.cfg.AdminEmail, Name: s.cfg.AdminName, PasswordHash: string(h), Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		st, _ := s.store.GetSettings(ctx)
		st.SetupComplete = true
		if err := s.store.SaveSettings(ctx, st); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	// Integrations configured through the environment belong to the box owner
	// (the first admin): they are seeded into that person's own settings.
	if owner := s.firstAdmin(ctx); owner != nil {
		if err := s.seedOwnerSettings(ctx, owner.ID); err != nil {
			return err
		}
	}
	if s.cfg.DemoData {
		if err := s.seedDemo(ctx); err != nil {
			return err
		}
	}
	// First start next to a LubeLogger: bring everything over once.
	if s.cfg.LubeLoggerURL != "" && s.cfg.ImportOnStartup {
		if counts := s.store.Counts(ctx); counts["vehicles"] == 0 {
			if owner, err := s.importOwner(ctx); err == nil {
				st := s.settingsFor(ctx, owner.ID)
				res := s.importLubeLogger(ctx, lubelogger.New(s.cfg.LubeLoggerURL, s.cfg.LubeLoggerUser, s.cfg.LubeLoggerPass, 60*time.Second), owner.ID, st.HomeCurrency)
				s.log.Info("lubelogger import on first start", "vehicles", len(res.Vehicles), "error", res.Error)
			} else {
				s.log.Warn("lubelogger import skipped: no user to own the vehicles yet", "err", err)
			}
		}
	}
	_ = s.store.PurgeSessions(ctx)
	go s.janitor(ctx)
	if s.core.Configured() {
		go s.coreLoop(ctx)
		go s.modesLoop(ctx)
	}
	return nil
}

// importOwner picks the account that owns imported vehicles: the first admin.
func (s *Server) importOwner(ctx context.Context) (*store.User, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].Role == "admin" {
			return &users[i], nil
		}
	}
	if len(users) > 0 {
		return &users[0], nil
	}
	if s.cfg.AuthMode != "local" {
		return s.firstOrSystemUser(ctx, "admin@localhost", "Admin")
	}
	return nil, errors.New("no users")
}

// firstAdmin returns the box owner (the oldest admin) without creating anyone.
func (s *Server) firstAdmin(ctx context.Context) *store.User {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil
	}
	for i := range users {
		if users[i].Role == "admin" {
			return &users[i]
		}
	}
	return nil
}

// seedOwnerSettings fills the owner's empty integration settings from the
// environment (DAWARICH_URL, NOTIFY_URL, ...).
func (s *Server) seedOwnerSettings(ctx context.Context, ownerID int64) error {
	st := s.settingsFor(ctx, ownerID)
	changed := false
	// Each value is copied once: clearing it in Settings (to switch Travel or
	// Inventory over to Pocket Core, say) must not bring it back at the next
	// start. A value already there counts as copied.
	set := func(name string, dst *string, v string) {
		key := fmt.Sprintf("seeded:%d:%s", ownerID, name)
		if _, _, done := s.store.CacheGet(ctx, key); done {
			return
		}
		if *dst == "" && v != "" {
			*dst = v
			changed = true
		}
		if *dst != "" {
			_ = s.store.CachePut(ctx, key, "1")
		}
	}
	set("DawarichURL", &st.DawarichURL, s.cfg.DawarichURL)
	set("DawarichAPIKey", &st.DawarichAPIKey, s.cfg.DawarichAPIKey)
	set("NotifyURL", &st.NotifyURL, s.cfg.NotifyURL)
	set("TravelURL", &st.TravelURL, s.cfg.TravelURL)
	set("TravelToken", &st.TravelToken, s.cfg.TravelToken)
	set("TravelPublicURL", &st.TravelPublicURL, s.cfg.TravelPublicURL)
	set("InventoryURL", &st.InventoryURL, s.cfg.InventoryURL)
	set("InventoryToken", &st.InventoryToken, s.cfg.InventoryToken)
	if s.cfg.NotifyFormat != "" && st.NotifyURL == s.cfg.NotifyURL && st.NotifyFormat != s.cfg.NotifyFormat {
		st.NotifyFormat = s.cfg.NotifyFormat
		changed = true
	}
	if !changed {
		return nil
	}
	return s.store.SaveUserSettings(ctx, ownerID, st)
}

// settingsFor returns what a person sees: the box-wide settings, the server
// defaults from the environment, then their own choices on top.
func (s *Server) settingsFor(ctx context.Context, userID int64) store.Settings {
	base, _ := s.store.GetSettings(ctx)
	if s.cfg.DefaultCurrency != "" {
		base.HomeCurrency = s.cfg.DefaultCurrency
	}
	if s.cfg.DistanceUnit != "" {
		base.DistanceUnit = s.cfg.DistanceUnit
	}
	if s.cfg.VolumeUnit != "" {
		base.VolumeUnit = s.cfg.VolumeUnit
	}
	if s.cfg.NotifyFormat != "" {
		base.NotifyFormat = s.cfg.NotifyFormat
	}
	base.NotifyHour = s.cfg.NotifyHour
	if userID <= 0 {
		return base
	}
	st, _ := s.store.GetUserSettings(ctx, userID, base)
	return st
}

// janitor runs the background work: the Dawarich sync every 30 minutes,
// the reminder check once a day at the configured hour.
func (s *Server) janitor(ctx context.Context) {
	// first pass shortly after boot so a fresh install shows data quickly
	first := time.NewTimer(20 * time.Second)
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			s.autoSync(ctx)
			s.maybeNotify(ctx)
		case <-t.C:
			s.autoSync(ctx)
			s.maybeNotify(ctx)
		}
	}
}

// maybeNotify runs each person's reminder check once per day once their
// hour has passed, with their own notification target.
func (s *Server) maybeNotify(ctx context.Context) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	day := now.Format("2006-01-02")
	for _, u := range users {
		st := s.settingsFor(ctx, u.ID)
		if st.NotifyURL == "" || now.Hour() < st.NotifyHour {
			continue
		}
		key := fmt.Sprintf("notify:last_day:%d", u.ID)
		if last, _, ok := s.store.CacheGet(ctx, key); ok && last == day {
			continue
		}
		_ = s.store.CachePut(ctx, key, day)
		if _, err := s.checkReminders(ctx, u.ID, false); err != nil {
			s.log.Warn("reminder notifications", "user", u.ID, "err", err)
		}
	}
}

// ---- integration clients, built from the current settings ----

type clients struct {
	st        store.Settings
	notify    *notify.Client
	travel    *pocket.Client
	inventory *pocket.Client
}

// clientsFor builds the integration clients from one person's settings.
func (s *Server) clientsFor(ctx context.Context, userID int64) clients {
	st := s.settingsFor(ctx, userID)
	if st.TravelURL == "" && st.TravelToken != "" {
		st.TravelURL = s.resolveSibling(ctx, "travel")
	}
	if st.InventoryURL == "" && st.InventoryToken != "" {
		st.InventoryURL = s.resolveSibling(ctx, "inventory")
	}
	return clients{st: st,
		notify:    notify.New(st.NotifyURL, st.NotifyFormat),
		travel:    pocket.New(st.TravelURL, st.TravelToken, 15*time.Second),
		inventory: pocket.New(st.InventoryURL, st.InventoryToken, 15*time.Second),
	}
}

// rate returns how many units of `to` one unit of `from` is worth on a date.
// Falls back to 1 (and says so) when nothing is available so a cost is never lost.
func (s *Server) rate(ctx context.Context, from, to, date string) (float64, string) {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == to || from == "" || to == "" {
		return 1, "same"
	}
	if date == "" || date > store.Today() {
		date = store.Today()
	}
	if r, src, ok := s.store.GetFXRate(ctx, from, to, date); ok {
		return r, src
	}
	st, _ := s.store.GetSettings(ctx)
	if !st.FXAuto {
		return 1, "disabled"
	}
	r, src, err := s.fx.Rate(ctx, from, to, date)
	if err != nil {
		s.log.Warn("fx lookup failed", "from", from, "to", to, "date", date, "err", err)
		return 1, "unavailable"
	}
	_ = s.store.PutFXRate(ctx, from, to, date, r, src)
	return r, src
}

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(120 * time.Second))

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Get("/auth/oidc/start", s.handleOIDCStart)
		r.Get("/auth/oidc/callback", s.handleOIDCCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			r.Get("/dashboard", s.handleDashboard)
			r.Get("/home", s.handleHome)
			r.Get("/currencies", s.handleCurrencies)
			r.Get("/fx", s.handleFX)
			r.Get("/meta", s.handleMeta)
			r.Get("/reminders", s.handleAllReminders)

			r.Get("/vehicles", s.handleListVehicles)
			r.Post("/vehicles", s.handleCreateVehicle)
			r.Route("/vehicles/{vehicleID}", func(r chi.Router) {
				r.Use(s.vehicleAccess)
				r.Get("/", s.handleGetVehicle)
				r.Put("/", s.handleUpdateVehicle)
				r.Delete("/", s.handleDeleteVehicle)
				r.Get("/overview", s.handleVehicleOverview)
				r.Get("/activity", s.handleVehicleActivity)
				r.Put("/members", s.handleVehicleMembers)
				r.Post("/image", s.handleVehicleImage)
				r.Get("/image", s.handleVehicleImageGet)
				r.Delete("/image", s.handleVehicleImageDelete)
				r.Get("/suggestions", s.handleSuggestions)

				r.Get("/records", s.handleListRecords)
				r.Post("/records", s.handleCreateRecord)
				r.Get("/records/{id}", s.handleGetRecord)
				r.Put("/records/{id}", s.handleUpdateRecord)
				r.Delete("/records/{id}", s.handleDeleteRecord)
				r.Post("/records/{id}/money", s.handleBookRecordMoney)

				r.Get("/fuel", s.handleListFuel)
				r.Post("/fuel", s.handleCreateFuel)
				r.Get("/fuel/{id}", s.handleGetFuel)
				r.Put("/fuel/{id}", s.handleUpdateFuel)
				r.Delete("/fuel/{id}", s.handleDeleteFuel)
				r.Post("/fuel/{id}/money", s.handleBookFuelMoney)

				r.Get("/odometer", s.handleOdometerHistory)
				r.Post("/odometer", s.handleCreateOdometer)
				r.Post("/odometer/confirm", s.handleConfirmOdometer)
				r.Put("/odometer/{id}", s.handleUpdateOdometer)
				r.Delete("/odometer/{id}", s.handleDeleteOdometer)

				r.Get("/reminders", s.handleListReminders)
				r.Post("/reminders", s.handleCreateReminder)
				r.Put("/reminders/{id}", s.handleUpdateReminder)
				r.Delete("/reminders/{id}", s.handleDeleteReminder)
				r.Post("/reminders/{id}/done", s.handleReminderDone)
				r.Post("/reminders/{id}/snooze", s.handleReminderSnooze)

				r.Get("/plans", s.handleListPlans)
				r.Post("/plans", s.handleCreatePlan)
				r.Put("/plans/reorder", s.handleReorderPlans)
				r.Put("/plans/{id}", s.handleUpdatePlan)
				r.Delete("/plans/{id}", s.handleDeletePlan)
				r.Post("/plans/{id}/done", s.handlePlanDone)

				r.Get("/notes", s.handleListNotes)
				r.Post("/notes", s.handleCreateNote)
				r.Put("/notes/{id}", s.handleUpdateNote)
				r.Delete("/notes/{id}", s.handleDeleteNote)

				r.Get("/documents", s.handleListDocuments)
				r.Post("/documents", s.handleUploadDocument)
				r.Put("/documents/{id}", s.handleUpdateDocument)
				r.Get("/documents/{id}/file", s.handleDocumentFile)
				r.Delete("/documents/{id}", s.handleDeleteDocument)

				r.Get("/reports", s.handleReports)
				r.Get("/trips", s.handleVehicleTrips)
				r.Get("/dawarich/days", s.handleDawarichDays)
				r.Post("/dawarich/sync", s.handleDawarichSync)

				r.Get("/export/calendar.ics", s.handleExportICS)
				r.Get("/export/{kind}.csv", s.handleExportCSV)
				r.Get("/export/vehicle.json", s.handleExportJSON)
			})

			r.Get("/money", s.handleMoneyStatus)
			r.Get("/money/categories", s.handleMoneyCategories)
			r.Get("/inventory/search", s.handleInventorySearch)
			r.Post("/integrations/test", s.handleIntegrationTest)
			r.Post("/notify/test", s.handleNotifyTest)
			r.Post("/notify/run", s.handleNotifyRun)
			r.Post("/import/lubelogger/vehicles", s.handleLLVehicles)
			r.Post("/import/lubelogger", s.handleLLImport)
			r.Post("/import/vehicle", s.handleImportJSON)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
		})
	})

	// LubeLogger-compatible API so the Pocket Homelab phone app and the
	// Raycast extension keep working with only the URL changed.
	s.compatRoutes(r)

	// Calendar subscription: token in the query so phones can subscribe.
	r.Get("/cal/{vehicleID}.ics", s.handleCalendarFeed)

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

// ---- Pocket Core ----

// coreLoop registers with Core on start (retrying until it answers) and
// re-registers every 30 minutes so version and last-seen stay fresh.
func (s *Server) coreLoop(ctx context.Context) {
	wait := 5 * time.Second
	for {
		err := s.registerWithCore(ctx)
		s.coreMu.Lock()
		was := s.coreOK
		s.coreOK, s.coreErr = err == nil, ""
		if err != nil {
			s.coreErr = err.Error()
		}
		s.coreMu.Unlock()
		if err == nil && !was {
			s.modesChanged() // first report right after registering
		}
		if err != nil {
			s.log.Warn("pocket core: registration failed, retrying", "err", err, "in", wait.String())
		} else {
			wait = 30 * time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err != nil && wait < 5*time.Minute {
			wait *= 2
		}
	}
}

func (s *Server) registerWithCore(ctx context.Context) error {
	m := core.Manifest{
		Slug: "garage", Name: "Pocket Garage",
		Description: "Every vehicle on one page: fuel with real economy, service and repairs, expenses, odometer, reminders, notes and documents.",
		Icon:        "/favicon.svg", Version: Version,
		URL: s.cfg.CoreAppURL, PublicURL: s.cfg.BaseURL, HealthPath: "/healthz", EnvPrefix: "GARAGE",
		Publishes: []string{"garage.fuel_logged", "garage.service_logged", "garage.odometer_updated"},
		Uses: []core.Use{
			{App: "money", Scopes: moneyScopes, Reason: "Book fuel, service, repairs and other car costs as transactions in Pocket Money"},
			{App: "travel", Scopes: travelScopes, Reason: "Show the fuel, costs and kilometres of each trip on the vehicle"},
			{App: "inventory", Scopes: inventoryScopes, Reason: "Offer parts and fluids from your shelf on a service record and take the ones you fitted off it"},
		},
		Services: serviceUses,
		Offers:   linkOffers,
	}
	if s.cfg.BaseURL != "" {
		// ask Core for a relying-party client so "Sign in with Pocket Core" needs no configuration
		m.OIDC = &core.OIDCRequest{RedirectURIs: []string{s.oidcCallbackURL()}}
	}
	reg, err := s.core.Register(ctx, s.cfg.CoreRegistrationKey, m)
	if err != nil {
		return err
	}
	if reg.Token != "" {
		s.log.Info("pocket core: registered, app token stored", "core", s.cfg.CoreURL)
	} else {
		s.log.Debug("pocket core: re-registered", "core", s.cfg.CoreURL)
	}
	return nil
}

// publish persists an event locally; the SDK retries network delivery. The payload
// names the person the data belongs to (the vehicle owner) by email, the
// identity every Pocket app shares, so consumers can keep it per person.
func (s *Server) publish(ctx context.Context, ownerID int64, topic, subject string, payload map[string]any) {
	if !s.core.Configured() {
		return
	}
	if u, err := s.store.GetUser(ctx, ownerID); err == nil {
		payload["user_email"] = u.Email
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.core.Publish(ctx, topic, subject, payload); err != nil {
		s.log.Warn("pocket core: outbox enqueue failed", "topic", topic, "err", err)
	}
}

// resolveSibling asks Core where a sibling app lives (cached a minute) so the
// *_URL settings may stay empty on a box with Core.
func (s *Server) resolveSibling(ctx context.Context, slug string) string {
	if !s.core.Configured() {
		return ""
	}
	s.coreMu.Lock()
	if r, ok := s.resolved[slug]; ok && time.Since(r.at) < time.Minute {
		s.coreMu.Unlock()
		return r.url
	}
	s.coreMu.Unlock()
	url := ""
	if res, err := s.core.Resolve(ctx, slug); err == nil {
		url = res.URL
	}
	s.coreMu.Lock()
	s.resolved[slug] = resolvedURL{url: url, at: time.Now()}
	s.coreMu.Unlock()
	return url
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_garage_info Build info.\n# TYPE pocket_garage_info gauge\npocket_garage_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_garage_uptime_seconds Process uptime.\n# TYPE pocket_garage_uptime_seconds gauge\npocket_garage_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_garage_http_requests_total HTTP requests served.\n# TYPE pocket_garage_http_requests_total counter\npocket_garage_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_garage_http_errors_total HTTP 5xx responses.\n# TYPE pocket_garage_http_errors_total counter\npocket_garage_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_garage_rows Rows per table.\n# TYPE pocket_garage_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_garage_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}

// ---------- files ----------

// saveFile writes bytes under FILES_DIR/<vehicle>/ with a random name and returns the relative path.
func (s *Server) saveFile(vehicleID int64, ext string, data []byte) (string, error) {
	dir := filepath.Join(s.cfg.FilesDir, strconv.FormatInt(vehicleID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext = strings.ToLower(filepath.Ext("x" + ext))
	if len(ext) > 8 {
		ext = ""
	}
	name := store.RandomToken(12) + ext
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return strconv.FormatInt(vehicleID, 10) + "/" + name, nil
}

func (s *Server) filePath(rel string) string {
	rel = filepath.Clean("/" + rel)
	return filepath.Join(s.cfg.FilesDir, rel)
}

func (s *Server) removeFile(rel string) {
	if rel == "" {
		return
	}
	_ = os.Remove(s.filePath(rel))
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, rel, mime, download string) {
	if rel == "" {
		writeErr(w, http.StatusNotFound, "no file")
		return
	}
	p := s.filePath(rel)
	if _, err := os.Stat(p); err != nil {
		writeErr(w, http.StatusNotFound, "file missing")
		return
	}
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	if download != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(download, `"`, "")+`"`)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}

// readUpload reads one multipart file field (or the raw body) up to the limit.
func (s *Server) readUpload(r *http.Request, field string) ([]byte, string, string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, s.cfg.MaxUploadBytes)
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
			return nil, "", "", err
		}
		f, hdr, err := r.FormFile(field)
		if err != nil {
			return nil, "", "", err
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return nil, "", "", err
		}
		return b, hdr.Filename, hdr.Header.Get("Content-Type"), nil
	}
	b, err := io.ReadAll(r.Body)
	return b, r.URL.Query().Get("name"), ct, err
}
