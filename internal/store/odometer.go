package store

import (
	"context"
	"sort"
)

// CreateOdometer inserts a standalone reading.
func (s *Store) CreateOdometer(ctx context.Context, o *OdometerReading) error {
	if o.Source == "" {
		o.Source = "manual"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO odometer (vehicle_id, date, reading, notes, source, external_id) VALUES (?, ?, ?, ?, ?, ?)`, o.VehicleID, o.Date, o.Reading, o.Notes, o.Source, o.ExternalID)
	if err != nil {
		return conflict(err)
	}
	o.ID, _ = res.LastInsertId()
	return nil
}

// UpdateOdometer saves a standalone reading.
func (s *Store) UpdateOdometer(ctx context.Context, o *OdometerReading) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE odometer SET date=?, reading=?, notes=? WHERE id=? AND vehicle_id=?`, o.Date, o.Reading, o.Notes, o.ID, o.VehicleID)
	return err
}

// GetOdometer loads a standalone reading.
func (s *Store) GetOdometer(ctx context.Context, vehicleID, id int64) (*OdometerReading, error) {
	var o OdometerReading
	if err := s.DB.QueryRowContext(ctx, `SELECT id, vehicle_id, date, reading, notes, source, created_at FROM odometer WHERE id = ? AND vehicle_id = ?`, id, vehicleID).Scan(&o.ID, &o.VehicleID, &o.Date, &o.Reading, &o.Notes, &o.Source, &o.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &o, nil
}

// DeleteOdometer removes a standalone reading.
func (s *Store) DeleteOdometer(ctx context.Context, vehicleID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM odometer WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// OdometerHistory unions standalone readings with the odometer carried by
// fuel and records, newest first, with the delta to the previous point.
func (s *Store) OdometerHistory(ctx context.Context, vehicleID int64) ([]OdometerReading, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, date, reading, notes, source, '', 0, created_at FROM odometer WHERE vehicle_id = ?
		UNION ALL SELECT id, date, odometer, COALESCE(NULLIF(station,''), 'Fill-up'), 'fuel', 'fuel', id, created_at FROM fuel WHERE vehicle_id = ? AND odometer > 0
		UNION ALL SELECT id, date, odometer, description, 'record', 'record', id, created_at FROM records WHERE vehicle_id = ? AND odometer IS NOT NULL AND odometer > 0
		ORDER BY date, reading`, vehicleID, vehicleID, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var asc []OdometerReading
	for rows.Next() {
		var o OdometerReading
		if err := rows.Scan(&o.ID, &o.Date, &o.Reading, &o.Notes, &o.Source, &o.LinkKind, &o.LinkID, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.VehicleID = vehicleID
		asc = append(asc, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := 1; i < len(asc); i++ {
		asc[i].Delta = asc[i].Reading - asc[i-1].Reading
		asc[i].Days = DaysBetween(asc[i-1].Date, asc[i].Date)
	}
	sort.SliceStable(asc, func(i, j int) bool {
		if asc[i].Date != asc[j].Date {
			return asc[i].Date > asc[j].Date
		}
		return asc[i].Reading > asc[j].Reading
	})
	return asc, nil
}

// LatestOdometer returns the highest confirmed reading and its date (0 when none).
func (s *Store) LatestOdometer(ctx context.Context, vehicleID int64) (float64, string) {
	var reading float64
	var date string
	_ = s.DB.QueryRowContext(ctx, `SELECT reading, date FROM (
		SELECT reading, date FROM odometer WHERE vehicle_id = ?
		UNION ALL SELECT odometer, date FROM fuel WHERE vehicle_id = ? AND odometer > 0
		UNION ALL SELECT odometer, date FROM records WHERE vehicle_id = ? AND odometer IS NOT NULL AND odometer > 0
	) ORDER BY reading DESC, date DESC LIMIT 1`, vehicleID, vehicleID, vehicleID).Scan(&reading, &date)
	return reading, date
}

// OdometerOn returns the last reading at or before a date (for the "km since" figures).
func (s *Store) OdometerOn(ctx context.Context, vehicleID int64, date string) float64 {
	var reading float64
	_ = s.DB.QueryRowContext(ctx, `SELECT reading FROM (
		SELECT reading, date FROM odometer WHERE vehicle_id = ?
		UNION ALL SELECT odometer, date FROM fuel WHERE vehicle_id = ? AND odometer > 0
		UNION ALL SELECT odometer, date FROM records WHERE vehicle_id = ? AND odometer IS NOT NULL AND odometer > 0
	) WHERE date <= ? ORDER BY date DESC, reading DESC LIMIT 1`, vehicleID, vehicleID, vehicleID, date).Scan(&reading)
	return reading
}

// ---- Dawarich days ----

// PutDawarichDay stores one day's recorded driving distance.
func (s *Store) PutDawarichDay(ctx context.Context, vehicleID int64, date string, km, total float64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO dawarich_days (vehicle_id, date, km, total_km, fetched_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(vehicle_id, date) DO UPDATE SET km = excluded.km, total_km = excluded.total_km, fetched_at = excluded.fetched_at`, vehicleID, date, km, total, Now())
	return err
}

// DawarichKmSince sums recorded driving km strictly after a date (the day of
// the reading itself is counted as known).
func (s *Store) DawarichKmSince(ctx context.Context, vehicleID int64, date string) float64 {
	var km float64
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(km), 0) FROM dawarich_days WHERE vehicle_id = ? AND date > ?`, vehicleID, date).Scan(&km)
	return km
}

// DawarichDays lists recorded days in a range, oldest first.
func (s *Store) DawarichDays(ctx context.Context, vehicleID int64, from, to string) ([]DawarichDay, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, km, total_km FROM dawarich_days WHERE vehicle_id = ? AND date >= ? AND date <= ? ORDER BY date`, vehicleID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DawarichDay{}
	for rows.Next() {
		var d DawarichDay
		if err := rows.Scan(&d.Date, &d.Km, &d.TotalKm); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// LastDawarichDay returns the newest stored day ("" when none).
func (s *Store) LastDawarichDay(ctx context.Context, vehicleID int64) string {
	var d string
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(date), '') FROM dawarich_days WHERE vehicle_id = ?`, vehicleID).Scan(&d)
	return d
}
