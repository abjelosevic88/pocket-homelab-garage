// Package export renders vehicle data as files other tools understand: an
// iCalendar feed of reminders and CSV tables.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

func icsEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\n", "\\n", "\r", "")
	return r.Replace(s)
}

// ICS renders reminders with a due date (and documents that expire) as all-day events.
func ICS(v *store.Vehicle, reminders []store.Reminder, docs []store.Document, baseURL string) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteString("\r\n") }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Pocket Garage//EN")
	w("CALSCALE:GREGORIAN")
	w("X-WR-CALNAME:" + icsEscape(v.Title()+" — garage"))
	stamp := time.Now().UTC().Format("20060102T150405Z")
	link := ""
	if baseURL != "" {
		link = fmt.Sprintf("%s/vehicles/%d/reminders", baseURL, v.ID)
	}
	for _, r := range reminders {
		if r.DueDate == "" || r.Status != "active" {
			continue
		}
		d, err := time.Parse("2006-01-02", r.DueDate)
		if err != nil {
			continue
		}
		w("BEGIN:VEVENT")
		w(fmt.Sprintf("UID:pocket-garage-reminder-%d-%d@pocket-garage", v.ID, r.ID))
		w("DTSTAMP:" + stamp)
		w("DTSTART;VALUE=DATE:" + d.Format("20060102"))
		w("DTEND;VALUE=DATE:" + d.AddDate(0, 0, 1).Format("20060102"))
		w("SUMMARY:" + icsEscape("🔧 "+v.Title()+": "+r.Title))
		desc := r.Notes
		if r.DueOdometer != nil {
			desc = strings.TrimSpace(desc + fmt.Sprintf("\nor at %.0f %s", *r.DueOdometer, v.OdometerUnit))
		}
		if desc != "" {
			w("DESCRIPTION:" + icsEscape(desc))
		}
		if link != "" {
			w("URL:" + link)
		}
		w("BEGIN:VALARM")
		w("TRIGGER:-P1D")
		w("ACTION:DISPLAY")
		w("DESCRIPTION:" + icsEscape(r.Title))
		w("END:VALARM")
		w("END:VEVENT")
	}
	for _, d := range docs {
		if d.ExpiresOn == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", d.ExpiresOn)
		if err != nil {
			continue
		}
		w("BEGIN:VEVENT")
		w(fmt.Sprintf("UID:pocket-garage-document-%d-%d@pocket-garage", v.ID, d.ID))
		w("DTSTAMP:" + stamp)
		w("DTSTART;VALUE=DATE:" + t.Format("20060102"))
		w("DTEND;VALUE=DATE:" + t.AddDate(0, 0, 1).Format("20060102"))
		w("SUMMARY:" + icsEscape("📄 "+v.Title()+": "+d.Name+" expires"))
		w("END:VEVENT")
	}
	w("END:VCALENDAR")
	return b.String()
}

func money(minor int64, decimals int) string {
	f := float64(minor)
	for i := 0; i < decimals; i++ {
		f /= 10
	}
	return strconv.FormatFloat(f, 'f', decimals, 64)
}

// RecordsCSV writes records as CSV.
func RecordsCSV(w io.Writer, rs []store.Record, decimals int) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "kind", "category", "odometer", "description", "vendor", "cost", "currency", "cost_home", "parts", "tags", "notes"})
	for _, r := range rs {
		odo := ""
		if r.Odometer != nil {
			odo = strconv.FormatFloat(*r.Odometer, 'f', -1, 64)
		}
		var parts []string
		for _, p := range r.Parts {
			parts = append(parts, fmt.Sprintf("%s x%g", p.Name, p.Qty))
		}
		_ = cw.Write([]string{r.Date, r.Kind, r.Category, odo, r.Description, r.Vendor, money(r.Cost, decimals), r.Currency, money(r.CostHome, decimals), strings.Join(parts, "; "), r.Tags, r.Notes})
	}
	cw.Flush()
	return cw.Error()
}

// FuelCSV writes fill-ups as CSV.
func FuelCSV(w io.Writer, fs []store.Fuel, decimals int) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "odometer", "volume", "cost", "currency", "cost_home", "price_per_unit", "full_tank", "missed", "distance", "economy", "station", "grade", "tags", "notes"})
	for _, f := range fs {
		_ = cw.Write([]string{f.Date, strconv.FormatFloat(f.Odometer, 'f', -1, 64), strconv.FormatFloat(f.Volume, 'f', 3, 64), money(f.Cost, decimals), f.Currency, money(f.CostHome, decimals),
			strconv.FormatFloat(f.PricePer/pow10(decimals), 'f', 3, 64), strconv.FormatBool(f.FullTank), strconv.FormatBool(f.Missed), strconv.FormatFloat(f.Distance, 'f', 1, 64), strconv.FormatFloat(f.Economy, 'f', 2, 64), f.Station, f.Grade, f.Tags, f.Notes})
	}
	cw.Flush()
	return cw.Error()
}

// OdometerCSV writes the odometer history.
func OdometerCSV(w io.Writer, os []store.OdometerReading) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "reading", "delta", "days", "source", "notes"})
	for _, o := range os {
		_ = cw.Write([]string{o.Date, strconv.FormatFloat(o.Reading, 'f', -1, 64), strconv.FormatFloat(o.Delta, 'f', 1, 64), strconv.Itoa(o.Days), o.Source, o.Notes})
	}
	cw.Flush()
	return cw.Error()
}

func pow10(n int) float64 {
	f := 1.0
	for i := 0; i < n; i++ {
		f *= 10
	}
	return f
}
