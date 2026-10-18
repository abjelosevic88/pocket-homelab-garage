package store

import (
	"context"
)

// Summary computes the figures for a vehicle card, the overview and the
// Homepage dashboard. One connection: every query runs to completion before
// the next starts.
func (s *Store) Summary(ctx context.Context, v *Vehicle) (*VehicleSummary, error) {
	sum := &VehicleSummary{CostByKind: map[string]int64{}}
	today := Today()
	year := today[:4]
	month := today[:7]

	sum.Odometer, sum.OdometerDate = s.LatestOdometer(ctx, v.ID)
	if sum.Odometer > 0 {
		sum.EstimateKm = s.DawarichKmSince(ctx, v.ID, sum.OdometerDate)
	}
	sum.OdometerEst = sum.Odometer + sum.EstimateKm

	// costs by kind (vehicle currency)
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, COALESCE(SUM(cost_home),0), COUNT(1) FROM records WHERE vehicle_id = ? GROUP BY kind`, v.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var c int64
		var n int
		if err := rows.Scan(&k, &c, &n); err != nil {
			rows.Close()
			return nil, err
		}
		sum.CostByKind[k] = c
		sum.CostTotal += c
		sum.Records += n
	}
	rows.Close()
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost_home),0), COUNT(1) FROM fuel WHERE vehicle_id = ?`, v.ID).Scan(&sum.CostFuelTotal, &sum.Fills)
	sum.CostByKind["fuel"] = sum.CostFuelTotal
	sum.CostTotal += sum.CostFuelTotal
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT SUM(cost_home) FROM records WHERE vehicle_id = ? AND date >= ?),0) + COALESCE((SELECT SUM(cost_home) FROM fuel WHERE vehicle_id = ? AND date >= ?),0)`, v.ID, year+"-01-01", v.ID, year+"-01-01").Scan(&sum.CostYear)
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT SUM(cost_home) FROM records WHERE vehicle_id = ? AND date >= ?),0) + COALESCE((SELECT SUM(cost_home) FROM fuel WHERE vehicle_id = ? AND date >= ?),0)`, v.ID, month+"-01", v.ID, month+"-01").Scan(&sum.CostMonth)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM plans WHERE vehicle_id = ? AND stage <> 'done'`, v.ID).Scan(&sum.Plans)

	// fuel economy from the fill list
	fills, err := s.ListFuel(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	var vol, dist float64
	var recentVol, recentDist float64
	recent := 0
	// overall economy: volume over distance across consecutive non-missed fills
	var prevOdo float64
	havePrev := false
	for i := len(fills) - 1; i >= 0; i-- {
		f := &fills[i]
		if f.Missed || !havePrev {
			prevOdo, havePrev = f.Odometer, true
			continue
		}
		if f.Odometer > prevOdo {
			dist += f.Odometer - prevOdo
			vol += f.Volume
		}
		prevOdo = f.Odometer
	}
	if dist > 0 && vol > 0 {
		sum.Economy = vol / dist * 100
	}
	for i := range fills {
		f := &fills[i]
		if f.Economy > 0 && recent < 5 {
			recentVol += f.Economy
			recentDist++
			recent++
		}
	}
	if recentDist > 0 {
		sum.EconomyRecent = recentVol / recentDist
	}
	if len(fills) > 0 {
		lf := fills[0]
		sum.LastFuel = &lf
	}
	// cost per km over the owned/recorded span
	firstOdo := v.PurchaseOdo
	if firstOdo == 0 {
		var lo float64
		_ = s.DB.QueryRowContext(ctx, `SELECT MIN(reading) FROM (SELECT reading FROM odometer WHERE vehicle_id = ? UNION ALL SELECT odometer FROM fuel WHERE vehicle_id = ? AND odometer > 0 UNION ALL SELECT odometer FROM records WHERE vehicle_id = ? AND odometer > 0)`, v.ID, v.ID, v.ID).Scan(&lo)
		firstOdo = lo
	}
	if span := sum.Odometer - firstOdo; span > 0 {
		sum.CostPerKm = float64(sum.CostTotal) / span
		sum.FuelPerKm = float64(sum.CostFuelTotal) / span
	}
	// km per year from first to last reading
	var firstDate string
	_ = s.DB.QueryRowContext(ctx, `SELECT MIN(date) FROM (SELECT date FROM odometer WHERE vehicle_id = ? UNION ALL SELECT date FROM fuel WHERE vehicle_id = ? UNION ALL SELECT date FROM records WHERE vehicle_id = ? AND odometer > 0)`, v.ID, v.ID, v.ID).Scan(&firstDate)
	if firstDate != "" && sum.OdometerDate != "" {
		if days := DaysBetween(firstDate, sum.OdometerDate); days > 30 {
			sum.KmPerYear = (sum.Odometer - s.OdometerOn(ctx, v.ID, firstDate)) / float64(days) * 365
		}
	}
	if v.PurchaseDate != "" {
		end := today
		if v.SoldDate != "" {
			end = v.SoldDate
		}
		sum.OwnedDays = DaysBetween(v.PurchaseDate, end)
	}
	// last service
	rec, _ := s.ListRecords(ctx, v.ID, RecordFilter{Kind: "service", Limit: 1})
	if len(rec) > 0 {
		sum.LastService = &rec[0]
	}
	// reminders
	rems, err := s.ListReminders(ctx, v.ID, "active")
	if err != nil {
		return nil, err
	}
	var next *Reminder
	for i := range rems {
		ComputeUrgency(&rems[i], today, sum.OdometerEst)
		sum.Reminders++
		switch rems[i].Urgency {
		case "due", "overdue":
			sum.RemindersDue++
		case "soon":
			sum.RemindersSoon++
		}
		if next == nil || UrgencyRank[rems[i].Urgency] < UrgencyRank[next.Urgency] || (rems[i].Urgency == next.Urgency && sooner(&rems[i], next)) {
			next = &rems[i]
		}
	}
	if next != nil {
		n := *next
		sum.NextReminder = &n
	}
	return sum, nil
}

// sooner reports whether a comes before b by days left, then km left.
func sooner(a, b *Reminder) bool {
	da, db := 1<<30, 1<<30
	if a.DaysLeft != nil {
		da = *a.DaysLeft
	}
	if b.DaysLeft != nil {
		db = *b.DaysLeft
	}
	if da != db {
		return da < db
	}
	ka, kb := 1e18, 1e18
	if a.KmLeft != nil {
		ka = *a.KmLeft
	}
	if b.KmLeft != nil {
		kb = *b.KmLeft
	}
	return ka < kb
}

// MonthlyCost is one bar of the cost chart.
type MonthlyCost struct {
	Month  string           `json:"month"` // YYYY-MM
	Total  int64            `json:"total"`
	ByKind map[string]int64 `json:"by_kind"`
}

// CostsByMonth sums record + fuel costs per month between two dates.
func (s *Store) CostsByMonth(ctx context.Context, vehicleID int64, from, to string) ([]MonthlyCost, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT substr(date,1,7) m, kind, SUM(cost_home) FROM (
		SELECT date, kind, cost_home FROM records WHERE vehicle_id = ? AND date >= ? AND date <= ?
		UNION ALL SELECT date, 'fuel', cost_home FROM fuel WHERE vehicle_id = ? AND date >= ? AND date <= ?
	) GROUP BY m, kind ORDER BY m`, vehicleID, from, to, vehicleID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MonthlyCost
	idx := map[string]int{}
	for rows.Next() {
		var m, k string
		var c int64
		if err := rows.Scan(&m, &k, &c); err != nil {
			return nil, err
		}
		i, ok := idx[m]
		if !ok {
			out = append(out, MonthlyCost{Month: m, ByKind: map[string]int64{}})
			i = len(out) - 1
			idx[m] = i
		}
		out[i].ByKind[k] += c
		out[i].Total += c
	}
	if out == nil {
		out = []MonthlyCost{}
	}
	return out, rows.Err()
}

// YearlyCost is one row of the per-year table.
type YearlyCost struct {
	Year   string           `json:"year"`
	Total  int64            `json:"total"`
	ByKind map[string]int64 `json:"by_kind"`
	Km     float64          `json:"km"`
	Fills  int              `json:"fills"`
	Volume float64          `json:"volume"`
}

// CostsByYear sums everything per calendar year with the km driven that year.
func (s *Store) CostsByYear(ctx context.Context, vehicleID int64) ([]YearlyCost, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT substr(date,1,4) y, kind, SUM(cost_home), COUNT(1), SUM(vol) FROM (
		SELECT date, kind, cost_home, 0 vol FROM records WHERE vehicle_id = ?
		UNION ALL SELECT date, 'fuel', cost_home, volume FROM fuel WHERE vehicle_id = ?
	) GROUP BY y, kind ORDER BY y`, vehicleID, vehicleID)
	if err != nil {
		return nil, err
	}
	var out []YearlyCost
	idx := map[string]int{}
	for rows.Next() {
		var y, k string
		var c int64
		var n int
		var vol float64
		if err := rows.Scan(&y, &k, &c, &n, &vol); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := idx[y]
		if !ok {
			out = append(out, YearlyCost{Year: y, ByKind: map[string]int64{}})
			i = len(out) - 1
			idx[y] = i
		}
		out[i].ByKind[k] += c
		out[i].Total += c
		if k == "fuel" {
			out[i].Fills += n
			out[i].Volume += vol
		}
	}
	rows.Close()
	for i := range out {
		y := out[i].Year
		start := s.OdometerOn(ctx, vehicleID, y+"-01-01")
		end := s.OdometerOn(ctx, vehicleID, y+"-12-31")
		if start == 0 {
			// first reading inside the year
			_ = s.DB.QueryRowContext(ctx, `SELECT MIN(reading) FROM (SELECT reading, date FROM odometer WHERE vehicle_id = ? UNION ALL SELECT odometer, date FROM fuel WHERE vehicle_id = ? AND odometer > 0 UNION ALL SELECT odometer, date FROM records WHERE vehicle_id = ? AND odometer > 0) WHERE date >= ? AND date <= ?`, vehicleID, vehicleID, vehicleID, y+"-01-01", y+"-12-31").Scan(&start)
		}
		if end > start {
			out[i].Km = end - start
		}
	}
	if out == nil {
		out = []YearlyCost{}
	}
	return out, nil
}

// HomeStats aggregates across the vehicles a user can see.
type HomeStats struct {
	Vehicles  int   `json:"vehicles"`
	Active    int   `json:"active"`
	CostYear  int64 `json:"cost_year"`
	CostMonth int64 `json:"cost_month"`
	Due       int   `json:"due"`
	Soon      int   `json:"soon"`
}

// RecentEntry is one line of the "latest activity" list across vehicles.
type RecentEntry struct {
	Kind        string  `json:"kind"` // fuel | service | repair | upgrade | expense
	ID          int64   `json:"id"`
	VehicleID   int64   `json:"vehicle_id"`
	VehicleName string  `json:"vehicle_name"`
	Date        string  `json:"date"`
	Title       string  `json:"title"`
	Cost        int64   `json:"cost"`
	Currency    string  `json:"currency"`
	Odometer    float64 `json:"odometer"`
}

// RecentEntries returns the newest fuel and records across the given vehicles.
func (s *Store) RecentEntries(ctx context.Context, vehicleIDs []int64, limit int) ([]RecentEntry, error) {
	out := []RecentEntry{}
	if len(vehicleIDs) == 0 {
		return out, nil
	}
	q := `SELECT * FROM (
		SELECT 'fuel' kind, f.id, f.vehicle_id, f.date, COALESCE(NULLIF(f.station,''), 'Fill-up'), f.cost, f.currency, f.odometer, f.created_at FROM fuel f WHERE f.vehicle_id IN (?` + repeat(",?", len(vehicleIDs)-1) + `)
		UNION ALL SELECT r.kind, r.id, r.vehicle_id, r.date, r.description, r.cost, r.currency, COALESCE(r.odometer,0), r.created_at FROM records r WHERE r.vehicle_id IN (?` + repeat(",?", len(vehicleIDs)-1) + `)
	) ORDER BY date DESC, created_at DESC LIMIT ?`
	args := make([]any, 0, 2*len(vehicleIDs)+1)
	for _, id := range vehicleIDs {
		args = append(args, id)
	}
	for _, id := range vehicleIDs {
		args = append(args, id)
	}
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e RecentEntry
		var created string
		if err := rows.Scan(&e.Kind, &e.ID, &e.VehicleID, &e.Date, &e.Title, &e.Cost, &e.Currency, &e.Odometer, &created); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

// StationStats aggregates fill-ups per station.
type StationStats struct {
	Station  string  `json:"station"`
	Fills    int     `json:"fills"`
	Volume   float64 `json:"volume"`
	Cost     int64   `json:"cost"`
	PricePer float64 `json:"price_per"`
}

// Stations returns per-station fuel totals.
func (s *Store) Stations(ctx context.Context, vehicleID int64) ([]StationStats, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT COALESCE(NULLIF(station,''), '—'), COUNT(1), SUM(volume), SUM(cost_home) FROM fuel WHERE vehicle_id = ? GROUP BY 1 ORDER BY 2 DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StationStats{}
	for rows.Next() {
		var st StationStats
		if err := rows.Scan(&st.Station, &st.Fills, &st.Volume, &st.Cost); err != nil {
			return nil, err
		}
		if st.Volume > 0 {
			st.PricePer = float64(st.Cost) / st.Volume
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
