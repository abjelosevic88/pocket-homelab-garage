// Package lubelogger reads a LubeLogger instance over its API for a one-time
// import, and holds the locale-tolerant number and date parsing that the
// LubeLogger-compatible endpoints of Pocket Garage reuse.
package lubelogger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client talks to one LubeLogger.
type Client struct {
	BaseURL  string
	Username string
	Password string
	HTTP     *http.Client
}

// New creates a client.
func New(baseURL, user, pass string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Username: user, Password: pass, HTTP: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" {
		return errors.New("lubelogger url is empty")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("culture-invariant", "true")
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("lubelogger rejected the credentials (401)")
	}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return errors.New("lubelogger redirected to a sign-in page — it needs a username and password")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("lubelogger HTTP %d", res.StatusCode)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "json") {
		return errors.New("lubelogger returned a page instead of JSON — check the URL")
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// GetBytes fetches a raw path (vehicle image).
func (c *Client) GetBytes(ctx context.Context, path string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, "", err
	}
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	return b, res.Header.Get("Content-Type"), err
}

// Vehicle is LubeLogger's vehicle export.
type Vehicle struct {
	ID            int64        `json:"id"`
	ImageLocation string       `json:"imageLocation"`
	Year          int          `json:"year"`
	Make          string       `json:"make"`
	Model         string       `json:"model"`
	LicensePlate  string       `json:"licensePlate"`
	PurchaseDate  string       `json:"purchaseDate"`
	SoldDate      string       `json:"soldDate"`
	PurchasePrice Number       `json:"purchasePrice"`
	SoldPrice     Number       `json:"soldPrice"`
	IsElectric    bool         `json:"isElectric"`
	IsDiesel      bool         `json:"isDiesel"`
	UseHours      bool         `json:"useHours"`
	Tags          []string     `json:"tags"`
	ExtraFields   []ExtraField `json:"extraFields"`
}

// ExtraField is a LubeLogger custom field.
type ExtraField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Record is the common shape of service/repair/upgrade/tax/gas/odometer rows.
type Record struct {
	ID              int64        `json:"id"`
	Date            string       `json:"date"`
	Odometer        Number       `json:"odometer"`
	InitialOdometer Number       `json:"initialOdometer"`
	Description     string       `json:"description"`
	Notes           string       `json:"notes"`
	Cost            Number       `json:"cost"`
	FuelConsumed    Number       `json:"fuelConsumed"`
	FuelEconomy     Number       `json:"fuelEconomy"`
	IsFillToFull    Bool         `json:"isFillToFull"`
	MissedFuelUp    Bool         `json:"missedFuelUp"`
	Tags            string       `json:"tags"`
	ExtraFields     []ExtraField `json:"extraFields"`
	Files           []File       `json:"files"`
}

// File is an attachment reference.
type File struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// Reminder is LubeLogger's reminder export.
type Reminder struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	Urgency     string `json:"urgency"`
	Metric      string `json:"metric"` // Date | Odometer | Both
	DueDate     string `json:"dueDate"`
	DueOdometer Number `json:"dueOdometer"`
	DueDays     Number `json:"dueDays"`
	DueDistance Number `json:"dueDistance"`
}

// Plan is a planner record.
type Plan struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	Type        string `json:"type"`     // ServiceRecord | RepairRecord | UpgradeRecord
	Priority    string `json:"priority"` // Low | Normal | Critical
	Progress    string `json:"progress"` // Backlog | InProgress | Testing | Done
	Cost        Number `json:"cost"`
	DateCreated string `json:"dateCreated"`
}

// Note is a vehicle note.
type Note struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	NoteText    string `json:"noteText"`
	Pinned      bool   `json:"pinned"`
}

// Vehicles lists all vehicles.
func (c *Client) Vehicles(ctx context.Context) ([]Vehicle, error) {
	var out []Vehicle
	return out, c.get(ctx, "/api/vehicles", nil, &out)
}

// Records fetches one record kind for a vehicle (servicerecords, repairrecords, upgraderecords, taxrecords, gasrecords, odometerrecords).
func (c *Client) Records(ctx context.Context, kind string, vehicleID int64) ([]Record, error) {
	var out []Record
	return out, c.get(ctx, "/api/vehicle/"+kind, url.Values{"vehicleId": {strconv.FormatInt(vehicleID, 10)}}, &out)
}

// Reminders fetches reminders.
func (c *Client) Reminders(ctx context.Context, vehicleID int64) ([]Reminder, error) {
	var out []Reminder
	return out, c.get(ctx, "/api/vehicle/reminders", url.Values{"vehicleId": {strconv.FormatInt(vehicleID, 10)}}, &out)
}

// Plans fetches planner records.
func (c *Client) Plans(ctx context.Context, vehicleID int64) ([]Plan, error) {
	var out []Plan
	return out, c.get(ctx, "/api/vehicle/planrecords", url.Values{"vehicleId": {strconv.FormatInt(vehicleID, 10)}}, &out)
}

// Notes fetches notes.
func (c *Client) Notes(ctx context.Context, vehicleID int64) ([]Note, error) {
	var out []Note
	return out, c.get(ctx, "/api/vehicle/notes", url.Values{"vehicleId": {strconv.FormatInt(vehicleID, 10)}}, &out)
}

// ---- tolerant scalars ----

// Number accepts JSON numbers and strings in any locale ("1.234,56", "1,234.56", "42,25").
type Number float64

// UnmarshalJSON parses leniently.
func (n *Number) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	*n = Number(ParseNumber(s))
	return nil
}

// Bool accepts true/false, "True"/"False", "1"/"0".
type Bool bool

// UnmarshalJSON parses leniently.
func (bl *Bool) UnmarshalJSON(b []byte) error {
	s := strings.ToLower(strings.Trim(string(b), `"`))
	*bl = Bool(s == "true" || s == "1" || s == "yes" || s == "on")
	return nil
}

var spaces = regexp.MustCompile(`[\s\x{00a0}\x{202f}']`)

// ParseNumber reads a number written with either decimal separator. When both
// appear the last one is the decimal point; a lone separator followed by one
// or two digits (or three with no other separator and a comma) is decimal.
func ParseNumber(raw string) float64 {
	t := spaces.ReplaceAllString(strings.TrimSpace(raw), "")
	if t == "" {
		return 0
	}
	comma, dot := strings.LastIndex(t, ","), strings.LastIndex(t, ".")
	norm := t
	switch {
	case comma >= 0 && dot >= 0:
		if comma > dot {
			norm = strings.ReplaceAll(t, ".", "")
			norm = strings.Replace(norm, ",", ".", 1)
		} else {
			norm = strings.ReplaceAll(t, ",", "")
		}
	case comma >= 0:
		if strings.Count(t, ",") == 1 {
			norm = strings.Replace(t, ",", ".", 1)
		} else {
			norm = strings.ReplaceAll(t, ",", "")
		}
	case dot >= 0:
		if strings.Count(t, ".") > 1 {
			norm = strings.ReplaceAll(t, ".", "")
		}
	}
	f, err := strconv.ParseFloat(norm, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

var (
	reISO    = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})`)
	reDotted = regexp.MustCompile(`^(\d{1,2})\.\s*(\d{1,2})\.\s*(\d{4})`)
	reSlash  = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})`)
	reDash   = regexp.MustCompile(`^(\d{1,2})-(\d{1,2})-(\d{4})`)
)

// ParseDate reads "2026-05-26", "26. 5. 2022.", "5/26/2022" or "26.05.2022" into YYYY-MM-DD ("" when unreadable).
// Slash dates are month-first (LubeLogger's en-US default) unless impossible.
func ParseDate(raw string) string {
	t := strings.TrimSpace(raw)
	if t == "" {
		return ""
	}
	ymd := func(y, m, d string) string {
		yy, _ := strconv.Atoi(y)
		mm, _ := strconv.Atoi(m)
		dd, _ := strconv.Atoi(d)
		if mm < 1 || mm > 12 || dd < 1 || dd > 31 || yy < 1900 {
			return ""
		}
		return fmt.Sprintf("%04d-%02d-%02d", yy, mm, dd)
	}
	if m := reISO.FindStringSubmatch(t); m != nil {
		return ymd(m[1], m[2], m[3])
	}
	if m := reDotted.FindStringSubmatch(t); m != nil {
		return ymd(m[3], m[2], m[1])
	}
	if m := reDash.FindStringSubmatch(t); m != nil {
		return ymd(m[3], m[2], m[1])
	}
	if m := reSlash.FindStringSubmatch(t); m != nil {
		a, _ := strconv.Atoi(m[1])
		if a > 12 {
			return ymd(m[3], m[2], m[1])
		}
		return ymd(m[3], m[1], m[2])
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "Jan 2, 2006", "2 Jan 2006", "January 2, 2006"} {
		if tt, err := time.Parse(layout, t); err == nil {
			return tt.Format("2006-01-02")
		}
	}
	return ""
}

// Currency pulls the "[Pocket Homelab currency: XXX]" marker the Pocket
// Homelab phone app leaves in notes, returning the code and the notes without it.
func Currency(notes string) (string, string) {
	lines := strings.Split(notes, "\n")
	code := ""
	kept := lines[:0]
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[Pocket Homelab currency: ") && strings.HasSuffix(t, "]") {
			code = strings.ToUpper(strings.TrimSuffix(strings.TrimPrefix(t, "[Pocket Homelab currency: "), "]"))
			continue
		}
		if strings.HasPrefix(t, "[Pocket Homelab action: ") && strings.HasSuffix(t, "]") {
			continue
		}
		kept = append(kept, l)
	}
	return code, strings.TrimSpace(strings.Join(kept, "\n"))
}
