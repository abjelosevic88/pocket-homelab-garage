package store

import (
	"context"
	"strconv"
)

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Garage"

// GlobalKeys are the box-wide settings (admin-only). Every other key is a
// person's own choice and lives in user_settings.
var GlobalKeys = []string{"title", "fx_auto", "setup_complete"}

// PersonalKeys are what each person sets for themselves: display preferences
// and the credentials of the integrations they connected.
var PersonalKeys = []string{"home_currency", "date_format", "time_format", "distance_unit", "volume_unit", "economy_unit",
	"firefly_url", "firefly_token", "firefly_category", "firefly_account", "firefly_auto_sync",
	"dawarich_url", "dawarich_api_key", "notify_url", "notify_format", "notify_hour", "notify_digest",
	"travel_url", "travel_token", "travel_public_url", "inventory_url", "inventory_token",
	"inventory_take", "travel_mode", "inventory_mode", "money_mode", "dawarich_mode", "money_enabled", "money_url", "money_token", "money_account", "money_category_fuel", "money_category", "money_vehicle_accounts"}

// DefaultSettings returns the built-in defaults.
func DefaultSettings() Settings {
	return Settings{Title: DefaultTitle, HomeCurrency: "EUR", DateFormat: "auto", TimeFormat: "auto", DistanceUnit: "km", VolumeUnit: "l", EconomyUnit: "l100", FXAuto: true, FireflyAutoSync: true, NotifyFormat: "apprise", NotifyHour: 8, NotifyDigest: true, MoneyEnabled: true, InventoryTake: true}
}

func applySetting(st *Settings, k, v string, global bool) {
	if global {
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "fx_auto":
			st.FXAuto = v != "0"
		case "setup_complete":
			st.SetupComplete = v == "1"
		}
		return
	}
	switch k {
	case "home_currency":
		if v != "" {
			st.HomeCurrency = v
		}
	case "date_format":
		if v != "" {
			st.DateFormat = v
		}
	case "time_format":
		if v != "" {
			st.TimeFormat = v
		}
	case "distance_unit":
		if v == "mi" || v == "km" {
			st.DistanceUnit = v
		}
	case "volume_unit":
		if v != "" {
			st.VolumeUnit = v
		}
	case "economy_unit":
		if v != "" {
			st.EconomyUnit = v
		}
	case "firefly_url":
		st.FireflyURL = v
	case "firefly_token":
		st.FireflyToken = v
	case "firefly_category":
		st.FireflyCategory = v
	case "firefly_account":
		st.FireflyAccount = v
	case "firefly_auto_sync":
		st.FireflyAutoSync = v != "0"
	case "dawarich_url":
		st.DawarichURL = v
	case "dawarich_api_key":
		st.DawarichAPIKey = v
	case "notify_url":
		st.NotifyURL = v
	case "notify_format":
		if v != "" {
			st.NotifyFormat = v
		}
	case "notify_hour":
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			st.NotifyHour = n
		}
	case "notify_digest":
		st.NotifyDigest = v != "0"
	case "travel_url":
		st.TravelURL = v
	case "travel_token":
		st.TravelToken = v
	case "travel_public_url":
		st.TravelPublicURL = v
	case "inventory_url":
		st.InventoryURL = v
	case "inventory_token":
		st.InventoryToken = v
	case "travel_mode":
		st.TravelMode = v
	case "inventory_mode":
		st.InventoryMode = v
	case "money_mode":
		st.MoneyMode = v
	case "dawarich_mode":
		st.DawarichMode = v
	case "inventory_take":
		st.InventoryTake = v != "0"
	case "money_enabled":
		st.MoneyEnabled = v != "0"
	case "money_url":
		st.MoneyURL = v
	case "money_token":
		st.MoneyToken = v
	case "money_account":
		st.MoneyAccount, _ = strconv.ParseInt(v, 10, 64)
	case "money_category_fuel":
		st.MoneyCategoryFuel, _ = strconv.ParseInt(v, 10, 64)
	case "money_category":
		st.MoneyCategory, _ = strconv.ParseInt(v, 10, 64)
	case "money_vehicle_accounts":
		st.MoneyVehicleAccts = v
	}
}

// GetSettings loads the box-wide settings (title, exchange-rate lookups,
// setup flag) over the defaults. Personal fields carry defaults only; use
// GetUserSettings for a person's view.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := DefaultSettings()
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		applySetting(&st, k, v, true)
	}
	return st, rows.Err()
}

// GetUserSettings overlays a person's own settings on base (the box-wide
// settings with the server defaults applied).
func (s *Store) GetUserSettings(ctx context.Context, userID int64, base Settings) (Settings, error) {
	st := base
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM user_settings WHERE user_id = ?`, userID)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		applySetting(&st, k, v, false)
	}
	return st, rows.Err()
}

func settingsKV(st Settings) map[string]string {
	b := map[bool]string{true: "1", false: "0"}
	return map[string]string{
		"title":                  st.Title,
		"fx_auto":                b[st.FXAuto],
		"setup_complete":         b[st.SetupComplete],
		"home_currency":          st.HomeCurrency,
		"date_format":            st.DateFormat,
		"time_format":            st.TimeFormat,
		"distance_unit":          st.DistanceUnit,
		"volume_unit":            st.VolumeUnit,
		"economy_unit":           st.EconomyUnit,
		"firefly_url":            st.FireflyURL,
		"firefly_token":          st.FireflyToken,
		"firefly_category":       st.FireflyCategory,
		"firefly_account":        st.FireflyAccount,
		"firefly_auto_sync":      b[st.FireflyAutoSync],
		"dawarich_url":           st.DawarichURL,
		"dawarich_api_key":       st.DawarichAPIKey,
		"notify_url":             st.NotifyURL,
		"notify_format":          st.NotifyFormat,
		"notify_hour":            strconv.Itoa(st.NotifyHour),
		"notify_digest":          b[st.NotifyDigest],
		"travel_url":             st.TravelURL,
		"travel_token":           st.TravelToken,
		"travel_public_url":      st.TravelPublicURL,
		"inventory_url":          st.InventoryURL,
		"inventory_token":        st.InventoryToken,
		"inventory_take":         b[st.InventoryTake],
		"travel_mode":            st.TravelMode,
		"inventory_mode":         st.InventoryMode,
		"money_mode":             st.MoneyMode,
		"dawarich_mode":          st.DawarichMode,
		"money_enabled":          b[st.MoneyEnabled],
		"money_url":              st.MoneyURL,
		"money_token":            st.MoneyToken,
		"money_account":          strconv.FormatInt(st.MoneyAccount, 10),
		"money_category_fuel":    strconv.FormatInt(st.MoneyCategoryFuel, 10),
		"money_category":         strconv.FormatInt(st.MoneyCategory, 10),
		"money_vehicle_accounts": st.MoneyVehicleAccts,
	}
}

// SaveSettings persists the box-wide settings only.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	kv := settingsKV(st)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, k := range GlobalKeys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, kv[k]); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// SaveUserSettings persists a person's own settings.
func (s *Store) SaveUserSettings(ctx context.Context, userID int64, st Settings) error {
	kv := settingsKV(st)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, k := range PersonalKeys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?) ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value`, userID, k, kv[k]); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---- generic cache ----

// CacheGet returns a cached value and when it was fetched.
func (s *Store) CacheGet(ctx context.Context, key string) (string, string, bool) {
	var v, at string
	if err := s.DB.QueryRowContext(ctx, `SELECT value, fetched_at FROM cache WHERE key = ?`, key).Scan(&v, &at); err != nil {
		return "", "", false
	}
	return v, at, true
}

// CachePut stores a value.
func (s *Store) CachePut(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cache (key, value, fetched_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, fetched_at = excluded.fetched_at`, key, value, Now())
	return err
}

// CacheDelete removes every key with the prefix.
func (s *Store) CacheDelete(ctx context.Context, prefix string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM cache WHERE key LIKE ? || '%'`, prefix)
	return err
}

// ---- fx cache ----

// GetFXRate returns a cached rate.
func (s *Store) GetFXRate(ctx context.Context, base, quote, date string) (float64, string, bool) {
	var r float64
	var src string
	if err := s.DB.QueryRowContext(ctx, `SELECT rate, source FROM fx_cache WHERE base = ? AND quote = ? AND date = ?`, base, quote, date).Scan(&r, &src); err != nil {
		return 0, "", false
	}
	return r, src, true
}

// PutFXRate caches a rate.
func (s *Store) PutFXRate(ctx context.Context, base, quote, date string, rate float64, source string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO fx_cache (base, quote, date, rate, source) VALUES (?, ?, ?, ?, ?) ON CONFLICT(base, quote, date) DO UPDATE SET rate = excluded.rate, source = excluded.source`, base, quote, date, rate, source)
	return err
}
