package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-garage/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Garage (the capabilities
// value in links.go): one route each, in the shape every Pocket app answers
// in. They read what the vehicle pages read, as the request's person and
// with the same fence: the vehicles that person owns or was added to. None
// of them changes anything; there is no quick capture, because a fill-up
// needs its odometer, volume and cost (the Quick log page is a form).

// recordKindLabel names a record kind as the service list does.
var recordKindLabel = map[string]string{"service": "Service", "repair": "Repair", "upgrade": "Upgrade", "expense": "Expense"}

// dots joins the parts of a line that are there: "Golf · 3 Nov 2026 · 84,210 km".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// hasWord reports whether a query has a letter or a digit to look for.
func hasWord(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// upperFirst starts a line with a capital letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// odoWords is an odometer reading or a distance in the vehicle's own unit,
// whole and grouped: "84,210 km". Nothing for no reading.
func odoWords(n float64, unit string) string {
	n = math.Abs(math.Round(n))
	if n == 0 {
		return ""
	}
	digits := strconv.FormatInt(int64(n), 10)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + " " + unit
}

// volumeWords is a fill-up's volume in the person's unit (stored in litres,
// kWh for an electric vehicle): "42 l", "11.1 gal", "38.5 kWh".
func volumeWords(litres float64, unit string, electric bool) string {
	v, label := litres, "l"
	switch {
	case electric:
		label = "kWh"
	case unit == "gal_us":
		v, label = litres/3.785411784, "gal"
	case unit == "gal_uk":
		v, label = litres/4.54609, "gal"
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) + " " + label
}

// dateWords is a day for a line: "3 Nov 2026"; short says "today",
// "yesterday" and leaves this year's year out.
func dateWords(date string, short bool) string {
	t, err := store.ParseDate(date)
	if err != nil {
		return date
	}
	today := store.Today()
	if short {
		switch {
		case date == today:
			return "today"
		case date == addDays(today, -1):
			return "yesterday"
		case date[:4] == today[:4]:
			return t.Format("2 Jan")
		}
	}
	return t.Format("2 Jan 2006")
}

// costWords is an amount as the API's texts write it ("62.40 EUR"); nothing for none.
func costWords(minor int64, code string) string {
	if minor == 0 || code == "" {
		return ""
	}
	return majorString(minor, code)
}

// sums adds amounts per currency, in the order they came.
type sums struct {
	order []string
	by    map[string]int64
}

func (m *sums) add(code string, minor int64) {
	if minor == 0 || code == "" {
		return
	}
	if m.by == nil {
		m.by = map[string]int64{}
	}
	if _, ok := m.by[code]; !ok {
		m.order = append(m.order, code)
	}
	m.by[code] += minor
}

// text: "62.40 EUR", "62.40 EUR and 20.00 BAM"; nothing when nothing was spent.
func (m *sums) text() string {
	parts := []string{}
	for _, c := range m.order {
		parts = append(parts, majorString(m.by[c], c))
	}
	return nameList(parts)
}

// nameList: "a", "a and b", "a, b and c" — each once.
func nameList(names []string) string {
	uniq := []string{}
	seen := map[string]bool{}
	for _, n := range names {
		if n != "" && !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}
	switch len(uniq) {
	case 0:
		return ""
	case 1:
		return uniq[0]
	}
	return strings.Join(uniq[:len(uniq)-1], ", ") + " and " + uniq[len(uniq)-1]
}

// fuelTitle names a fill-up: "Fuel: 42 l", "Charge: 38.5 kWh".
func fuelTitle(f *store.Fuel, v *store.Vehicle, volumeUnit string) string {
	electric := v.FuelType == "electric"
	word := "Fuel"
	if electric {
		word = "Charge"
	}
	if f.Volume <= 0 {
		return word
	}
	return word + ": " + volumeWords(f.Volume, volumeUnit, electric)
}

// recordWhat is what a record was about: its description, else its category
// or kind ("Oil change", "Tyres", "Service").
func recordWhat(r *store.Record) string {
	return firstNonEmpty(strings.TrimSpace(r.Description), firstNonEmpty(upperFirst(strings.TrimSpace(r.Category)), firstNonEmpty(recordKindLabel[r.Kind], "Record")))
}

// recordTitle names a record with its kind in front: "Service: Oil change",
// "Expense: Insurance"; just "Service" when there is nothing more to say.
func recordTitle(r *store.Record) string {
	label, what := firstNonEmpty(recordKindLabel[r.Kind], "Record"), recordWhat(r)
	if strings.EqualFold(label, what) {
		return label
	}
	return label + ": " + what
}

// recordOdo is a record's odometer, when it has one.
func recordOdo(r *store.Record, unit string) string {
	if r.Odometer == nil {
		return ""
	}
	return odoWords(*r.Odometer, unit)
}

func dayCount(n int) string { return fmt.Sprintf("%d day%s", n, plural(n)) }

// whenWords says where a reminder stands, by date and by distance, as the
// reminders page does: "in 12 days", "3 days overdue", "800 km left",
// "200 km over". Done and dismissed reminders say so.
func whenWords(rm *store.Reminder, unit string) string {
	switch rm.Status {
	case "done":
		return "Done"
	case "dismissed":
		return "Dismissed"
	}
	var parts []string
	if d := rm.DaysLeft; d != nil {
		switch {
		case *d < 0:
			parts = append(parts, dayCount(-*d)+" overdue")
		case *d == 0:
			parts = append(parts, "today")
		case *d == 1:
			parts = append(parts, "tomorrow")
		case *d <= 31:
			parts = append(parts, "in "+dayCount(*d))
		default:
			parts = append(parts, "on "+dateWords(rm.DueDate, false))
		}
	}
	if k := rm.KmLeft; k != nil {
		switch {
		case math.Round(*k) < 0:
			parts = append(parts, odoWords(*k, unit)+" over")
		case math.Round(*k) == 0:
			parts = append(parts, "due now")
		default:
			parts = append(parts, odoWords(*k, unit)+" left")
		}
	}
	return strings.Join(parts, ", ")
}

// level is how pressing one side of a reminder is (a date or a distance),
// with the same steps as store.ComputeUrgency.
func level(left, soon float64) string {
	switch {
	case left < 0:
		return "overdue"
	case left == 0:
		return "due"
	case left <= soon:
		return "soon"
	}
	return "ok"
}

// reminderLine is a reminder in a sentence, by whichever of date and
// distance is more pressing: "Oil change due in 12 days", "Oil change is
// 3 days overdue", "Tyres due in 800 km".
func reminderLine(rm *store.Reminder, unit string) string {
	byKm := rm.KmLeft != nil
	if byKm && rm.DaysLeft != nil {
		byKm = store.UrgencyRank[level(math.Round(*rm.KmLeft), rm.SoonKm)] < store.UrgencyRank[level(float64(*rm.DaysLeft), float64(rm.SoonDays))]
	}
	switch {
	case byKm:
		k := math.Round(*rm.KmLeft)
		switch {
		case k < 0:
			return rm.Title + " is " + odoWords(k, unit) + " overdue"
		case k == 0:
			return rm.Title + " is due now"
		}
		return rm.Title + " due in " + odoWords(k, unit)
	case rm.DaysLeft != nil:
		d := *rm.DaysLeft
		switch {
		case d < 0:
			return rm.Title + " is " + dayCount(-d) + " overdue"
		case d == 0:
			return rm.Title + " is due today"
		case d == 1:
			return rm.Title + " is due tomorrow"
		}
		return rm.Title + " due in " + dayCount(d)
	}
	return rm.Title
}

// odometerEstimate is a vehicle's latest reading plus the kilometres driven
// since, as the reminders page works out what is due.
func (s *Server) odometerEstimate(ctx context.Context, vehicleID int64) float64 {
	reading, date := s.store.LatestOdometer(ctx, vehicleID)
	return reading + s.store.DawarichKmSince(ctx, vehicleID, date)
}

// vehiclesSeen maps the vehicles a person owns or was added to by id.
func (s *Server) vehiclesSeen(ctx context.Context, u *store.User) (map[int64]*store.Vehicle, error) {
	vs, err := s.visibleVehicles(ctx, u, "")
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*store.Vehicle, len(vs))
	for i := range vs {
		out[vs[i].ID] = &vs[i]
	}
	return out, nil
}

func vehicleURL(id int64, tab string) string {
	u := "/vehicles/" + strconv.FormatInt(id, 10)
	if tab != "" {
		u += "/" + tab
	}
	return u
}

func ref(kind string, id int64) string { return kind + ":" + strconv.FormatInt(id, 10) }

// ---------- search ----------

// pocketSearch answers Pocket Search: the vehicles the vehicle list finds
// (name, make, model, plate), the service records the service list finds,
// fill-ups by station and notes, and reminders by title and notes, across
// the person's vehicles. A vehicle whose name has the words comes first,
// then titles that have them, then the rest; newest first inside each.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	u := userFrom(ctx)
	byID, err := s.vehiclesSeen(ctx, u)
	if err != nil {
		return nil, err
	}
	if len(byID) == 0 {
		return nil, nil
	}
	st := s.settingsFor(ctx, u.ID)
	lq := strings.ToLower(q)
	has := func(v ...string) bool { return strings.Contains(strings.ToLower(strings.Join(v, " ")), lq) }
	score := func(title bool) float64 {
		if title {
			return 3
		}
		return 2
	}
	out := []core.SearchItem{}

	vs, err := s.store.ListVehicles(ctx, store.VehicleFilter{UserID: u.ID, Query: q})
	if err != nil {
		return nil, err
	}
	for i := range vs {
		if i == limit {
			break
		}
		v := &vs[i]
		model := ""
		if v.Name != "" { // the title is the nickname: say what it is
			model = strings.TrimSpace(v.Make + " " + v.Model)
			if v.Year > 0 {
				model = strings.TrimSpace(strconv.Itoa(v.Year) + " " + model)
			}
		}
		status := ""
		if v.Status != "active" {
			status = upperFirst(v.Status)
		}
		reading, _ := s.store.LatestOdometer(ctx, v.ID)
		it := core.SearchItem{ID: ref("vehicle", v.ID), Title: v.Title(), Kind: "vehicle", URL: vehicleURL(v.ID, ""), Date: v.UpdatedAt,
			Snippet: dots(model, v.Plate, odoWords(reading, v.OdometerUnit), status), Score: 3}
		if has(v.Title()) {
			it.Score = 4
		}
		out = append(out, it)
	}

	recs, err := s.store.SearchRecords(ctx, u.ID, q, limit)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		rec := &recs[i]
		v := byID[rec.VehicleID]
		if v == nil {
			continue
		}
		out = append(out, core.SearchItem{ID: ref("record", rec.ID), Title: recordTitle(rec), Kind: "record", Date: rec.Date, URL: vehicleURL(v.ID, "service"),
			Snippet: dots(v.Title(), dateWords(rec.Date, false), recordOdo(rec, v.OdometerUnit), costWords(rec.Cost, rec.Currency), rec.Vendor),
			Score:   score(has(rec.Description))})
	}

	fills, err := s.store.SearchFuel(ctx, u.ID, q, limit)
	if err != nil {
		return nil, err
	}
	for i := range fills {
		f := &fills[i]
		v := byID[f.VehicleID]
		if v == nil {
			continue
		}
		out = append(out, core.SearchItem{ID: ref("fill-up", f.ID), Title: fuelTitle(f, v, st.VolumeUnit), Kind: "fill-up", Date: f.Date, URL: vehicleURL(v.ID, "fuel"),
			Snippet: dots(v.Title(), dateWords(f.Date, false), odoWords(f.Odometer, v.OdometerUnit), costWords(f.Cost, f.Currency), f.Station),
			Score:   score(false)})
	}

	rems, err := s.store.SearchReminders(ctx, u.ID, q, limit)
	if err != nil {
		return nil, err
	}
	est := map[int64]float64{}
	today := store.Today()
	for i := range rems {
		rm := &rems[i]
		v := byID[rm.VehicleID]
		if v == nil {
			continue
		}
		if _, ok := est[v.ID]; !ok && rm.Status == "active" {
			est[v.ID] = s.odometerEstimate(ctx, v.ID)
		}
		store.ComputeUrgency(rm, today, est[v.ID])
		out = append(out, core.SearchItem{ID: ref("reminder", rm.ID), Title: rm.Title, Kind: "reminder", Date: rm.DueDate, URL: vehicleURL(v.ID, "reminders"),
			Snippet: dots(v.Title(), upperFirst(whenWords(rm, v.OdometerUnit))), Score: score(has(rm.Title))})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---------- day ----------

// pocketDay answers Pocket Day: the fill-ups and records dated that day on
// the person's vehicles (shared ones included, whoever logged them), and
// the reminders due that day. Days here are plain dates, so the time zone
// is not used.
func (s *Server) pocketDay(r *http.Request, date string, _ *time.Location) (*core.Day, error) {
	ctx := r.Context()
	u := userFrom(ctx)
	fills, err := s.store.FuelOn(ctx, u.ID, date)
	if err != nil {
		return nil, err
	}
	recs, err := s.store.RecordsOn(ctx, u.ID, date)
	if err != nil {
		return nil, err
	}
	dues, err := s.store.RemindersDueOn(ctx, u.ID, date)
	if err != nil {
		return nil, err
	}
	if len(fills)+len(recs)+len(dues) == 0 {
		return nil, nil
	}
	byID, err := s.vehiclesSeen(ctx, u)
	if err != nil {
		return nil, err
	}
	st := s.settingsFor(ctx, u.ID)
	day := &core.Day{Date: date}
	var spent, fuelSpent, recSpent sums
	var fuelled, serviced []string
	var parts []string

	for i := range fills {
		f := &fills[i]
		v := byID[f.VehicleID]
		if v == nil {
			continue
		}
		day.Items = append(day.Items, core.DayItem{ID: ref("fill-up", f.ID), Title: fuelTitle(f, v, st.VolumeUnit), Kind: "fuel",
			Sub: dots(v.Title(), odoWords(f.Odometer, v.OdometerUnit), f.Station), Amount: f.Cost, Currency: f.Currency, Note: strings.TrimSpace(f.Notes),
			URL: vehicleURL(v.ID, "fuel")})
		fuelled = append(fuelled, "the "+v.Title())
		fuelSpent.add(f.Currency, f.Cost)
		spent.add(f.Currency, f.Cost)
	}
	var lastRec *store.Record
	for i := range recs {
		rec := &recs[i]
		v := byID[rec.VehicleID]
		if v == nil {
			continue
		}
		kind := "service"
		if rec.Kind == "expense" {
			kind = "other"
		}
		day.Items = append(day.Items, core.DayItem{ID: ref("record", rec.ID), Title: recordTitle(rec), Kind: kind,
			Sub: dots(v.Title(), recordOdo(rec, v.OdometerUnit), rec.Vendor), Amount: rec.Cost, Currency: rec.Currency, Note: strings.TrimSpace(rec.Notes),
			URL: vehicleURL(v.ID, "service")})
		serviced = append(serviced, "the "+v.Title())
		recSpent.add(rec.Currency, rec.Cost)
		spent.add(rec.Currency, rec.Cost)
		lastRec = rec
	}
	var dueNames []string
	var lastDue *store.Reminder
	for i := range dues {
		rm := &dues[i]
		v := byID[rm.VehicleID]
		if v == nil {
			continue
		}
		day.Items = append(day.Items, core.DayItem{ID: ref("reminder", rm.ID), Title: rm.Title, Kind: "due", Sub: v.Title(), Note: strings.TrimSpace(rm.Notes),
			URL: vehicleURL(v.ID, "reminders")})
		dueNames = append(dueNames, "the "+v.Title())
		lastDue = rm
	}
	if len(day.Items) == 0 {
		return nil, nil
	}

	// "Fuelled the Golf, 62.40 EUR; Oil change for the Golf, 89.00 EUR; Tyres due for the Panda"
	with := func(line string, m *sums) string {
		if t := m.text(); t != "" {
			return line + ", " + t
		}
		return line
	}
	if len(fuelled) > 0 {
		parts = append(parts, with("Fuelled "+nameList(fuelled), &fuelSpent))
	}
	switch n := len(serviced); {
	case n == 1:
		parts = append(parts, with(recordWhat(lastRec)+" for "+serviced[0], &recSpent))
	case n > 1:
		parts = append(parts, with(fmt.Sprintf("%d records for %s", n, nameList(serviced)), &recSpent))
	}
	switch n := len(dueNames); {
	case n == 1:
		parts = append(parts, lastDue.Title+" due for "+dueNames[0])
	case n > 1:
		parts = append(parts, fmt.Sprintf("%d reminders due for %s", n, nameList(dueNames)))
	}
	day.Summary = upperFirst(strings.Join(parts, "; "))
	if t := spent.text(); t != "" {
		day.Metrics = []core.Metric{{Label: "Spent", Value: t}}
	}
	return day, nil
}

// ---------- widget ----------

const widgetRows = 8

// pocketWidget answers Pocket Hub: the reminders that need doing across the
// person's vehicles, as on the first page (overdue first, then due, then
// soon, by date or distance, whichever comes first), then each active
// vehicle's odometer and last fill-up. Someone without vehicles gets
// nothing to show.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	u := userFrom(ctx)
	vs, err := s.visibleVehicles(ctx, u, "")
	if err != nil {
		return nil, err
	}
	today := store.Today()
	byID := map[int64]*store.Vehicle{}
	est := map[int64]float64{}
	var pressing []store.Reminder
	for i := range vs {
		v := &vs[i]
		if v.Status == "archived" {
			continue
		}
		byID[v.ID] = v
		est[v.ID] = s.odometerEstimate(ctx, v.ID)
		rs, err := s.store.ListReminders(ctx, v.ID, "active")
		if err != nil {
			return nil, err
		}
		for j := range rs {
			store.ComputeUrgency(&rs[j], today, est[v.ID])
			if rs[j].Urgency != "ok" && rs[j].Urgency != "done" {
				pressing = append(pressing, rs[j])
			}
		}
	}
	if len(byID) == 0 {
		return nil, nil
	}
	sortReminders(pressing)
	w := &core.WidgetData{Items: []core.WidgetItem{}}
	due, soon := 0, 0
	for i := range pressing {
		rm := &pressing[i]
		v := byID[rm.VehicleID]
		if rm.Urgency == "soon" {
			soon++
		} else {
			due++
		}
		if len(w.Items) < widgetRows {
			w.Items = append(w.Items, core.WidgetItem{ID: ref("reminder", rm.ID), Title: rm.Title, Detail: dots(v.Title(), whenWords(rm, v.OdometerUnit)),
				URL: vehicleURL(v.ID, "reminders")})
		}
	}
	if len(pressing) > 0 {
		top := &pressing[0]
		w.Line = reminderLine(top, byID[top.VehicleID].OdometerUnit)
		if n := len(pressing) - 1; n > 0 {
			w.Line += fmt.Sprintf(", and %d more", n)
		}
		w.URL = "/reminders"
	}
	w.Metrics = []core.Metric{{Label: "Due", Value: strconv.Itoa(due)}, {Label: "Soon", Value: strconv.Itoa(soon)}}
	for i := range vs {
		v := &vs[i]
		if v.Status != "active" || len(w.Items) >= widgetRows {
			continue
		}
		fill := ""
		if f := s.store.LastFuel(ctx, v.ID); f != nil {
			word := "filled up "
			if v.FuelType == "electric" {
				word = "charged "
			}
			fill = word + dateWords(f.Date, true)
		}
		w.Items = append(w.Items, core.WidgetItem{ID: ref("vehicle", v.ID), Title: v.Title(), Detail: dots(odoWords(est[v.ID], v.OdometerUnit), fill),
			URL: vehicleURL(v.ID, "")})
	}
	return w, nil
}
