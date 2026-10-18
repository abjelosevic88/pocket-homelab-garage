// Package config loads runtime configuration from environment variables.
//
// Every option has a sensible default so that `docker run` with a single
// volume works out of the box. See docs/configuration.md for the full list.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	Host           string
	Port           int
	BaseURL        string
	DataDir        string
	DBPath         string
	FilesDir       string
	LogLevel       string
	LogFormat      string // text | json
	TrustProxy     bool
	MetricsEnabled bool

	MaxUploadBytes int64

	AuthMode             string // local | proxy | none
	AuthProxyHeader      string
	AuthProxyEmailHeader string
	SessionTTL           time.Duration
	SecureCookies        bool
	AdminEmail           string
	AdminPassword        string
	AdminName            string

	DefaultCurrency string
	DistanceUnit    string // km | mi
	VolumeUnit      string // l | gal_us | gal_uk
	FXProvider      string // auto | frankfurter | erapi | none

	// Integration defaults; the values saved under Settings → Integrations win.
	FireflyURL      string
	FireflyToken    string
	DawarichURL     string
	DawarichAPIKey  string
	NotifyURL       string
	NotifyFormat    string // ntfy | apprise | json
	NotifyHour      int
	TravelURL       string
	TravelToken     string
	TravelPublicURL string
	InventoryURL    string
	InventoryToken  string
	LubeLoggerURL   string // one-shot import on first start when the database is empty
	LubeLoggerUser  string
	LubeLoggerPass  string
	ImportOnStartup bool

	// Outbound lookups identify as this.
	UserAgent string

	DemoData bool
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(env(key, "")) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	if v := env(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := env(key, ""); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := env(key, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	dataDir := env("DATA_DIR", "./data")
	c := &Config{
		Host:           env("HOST", "0.0.0.0"),
		Port:           envInt("PORT", 8080),
		BaseURL:        strings.TrimRight(env("BASE_URL", ""), "/"),
		DataDir:        dataDir,
		DBPath:         env("DB_PATH", dataDir+"/pocket-garage.db"),
		FilesDir:       env("FILES_DIR", dataDir+"/files"),
		LogLevel:       strings.ToLower(env("LOG_LEVEL", "info")),
		LogFormat:      strings.ToLower(env("LOG_FORMAT", "text")),
		TrustProxy:     envBool("TRUST_PROXY", true),
		MetricsEnabled: envBool("METRICS_ENABLED", true),

		MaxUploadBytes: envInt64("MAX_UPLOAD_BYTES", 50<<20),

		AuthMode:             strings.ToLower(env("AUTH_MODE", "local")),
		AuthProxyHeader:      env("AUTH_PROXY_HEADER", "Remote-User"),
		AuthProxyEmailHeader: env("AUTH_PROXY_EMAIL_HEADER", "Remote-Email"),
		SessionTTL:           envDuration("SESSION_TTL", 720*time.Hour),
		SecureCookies:        envBool("SECURE_COOKIES", false),
		AdminEmail:           strings.ToLower(env("ADMIN_EMAIL", "")),
		AdminPassword:        env("ADMIN_PASSWORD", ""),
		AdminName:            env("ADMIN_NAME", "Admin"),

		DefaultCurrency: strings.ToUpper(env("DEFAULT_CURRENCY", "EUR")),
		DistanceUnit:    strings.ToLower(env("DISTANCE_UNIT", "km")),
		VolumeUnit:      strings.ToLower(env("VOLUME_UNIT", "l")),
		FXProvider:      strings.ToLower(env("FX_PROVIDER", "auto")),

		FireflyURL:      strings.TrimRight(env("FIREFLY_URL", ""), "/"),
		FireflyToken:    env("FIREFLY_TOKEN", ""),
		DawarichURL:     strings.TrimRight(env("DAWARICH_URL", ""), "/"),
		DawarichAPIKey:  env("DAWARICH_API_KEY", ""),
		NotifyURL:       env("NOTIFY_URL", ""),
		NotifyFormat:    strings.ToLower(env("NOTIFY_FORMAT", "")),
		NotifyHour:      envInt("NOTIFY_HOUR", 8),
		TravelURL:       strings.TrimRight(env("TRAVEL_URL", ""), "/"),
		TravelToken:     env("TRAVEL_TOKEN", ""),
		TravelPublicURL: strings.TrimRight(env("TRAVEL_PUBLIC_URL", ""), "/"),
		InventoryURL:    strings.TrimRight(env("INVENTORY_URL", ""), "/"),
		InventoryToken:  env("INVENTORY_TOKEN", ""),
		LubeLoggerURL:   strings.TrimRight(env("LUBELOGGER_URL", ""), "/"),
		LubeLoggerUser:  env("LUBELOGGER_USERNAME", ""),
		LubeLoggerPass:  env("LUBELOGGER_PASSWORD", ""),
		ImportOnStartup: envBool("LUBELOGGER_IMPORT_ON_START", true),

		UserAgent: env("USER_AGENT", "PocketGarage/1.0 (+https://github.com/abjelosevic88/pocket-homelab-garage)"),

		DemoData: envBool("DEMO_DATA", false),
	}
	switch c.AuthMode {
	case "local", "proxy", "none":
	default:
		return nil, fmt.Errorf("AUTH_MODE must be one of local, proxy, none (got %q)", c.AuthMode)
	}
	switch c.FXProvider {
	case "auto", "frankfurter", "erapi", "none":
	default:
		return nil, fmt.Errorf("FX_PROVIDER must be one of auto, frankfurter, erapi, none (got %q)", c.FXProvider)
	}
	if c.DistanceUnit != "km" && c.DistanceUnit != "mi" {
		return nil, fmt.Errorf("DISTANCE_UNIT must be km or mi (got %q)", c.DistanceUnit)
	}
	switch c.VolumeUnit {
	case "l", "gal_us", "gal_uk":
	default:
		return nil, fmt.Errorf("VOLUME_UNIT must be l, gal_us or gal_uk (got %q)", c.VolumeUnit)
	}
	if c.Port <= 0 || c.Port > 65535 {
		return nil, fmt.Errorf("PORT out of range: %d", c.Port)
	}
	if c.NotifyHour < 0 || c.NotifyHour > 23 {
		c.NotifyHour = 8
	}
	return c, nil
}

// Addr returns host:port.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }
