package store

import (
	"context"
	"encoding/json"
	"strings"
)

const recordCols = `id, vehicle_id, kind, category, date, odometer, description, notes, vendor, cost, currency, cost_home, fx_rate, parts, tags, source, external_id, firefly_id, created_by, created_at, updated_at`

func scanRecord(row interface{ Scan(...any) error }) (*Record, error) {
	var r Record
	var parts string
	if err := row.Scan(&r.ID, &r.VehicleID, &r.Kind, &r.Category, &r.Date, &r.Odometer, &r.Description, &r.Notes, &r.Vendor, &r.Cost, &r.Currency, &r.CostHome, &r.FXRate, &parts, &r.Tags, &r.Source, &r.ExternalID, &r.FireflyID, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	r.Parts = []Part{}
	_ = json.Unmarshal([]byte(parts), &r.Parts)
	return &r, nil
}

func partsJSON(p []Part) string {
	if p == nil {
		p = []Part{}
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// CreateRecord inserts a record.
func (s *Store) CreateRecord(ctx context.Context, r *Record) error {
	if r.Source == "" {
		r.Source = "manual"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO records (vehicle_id, kind, category, date, odometer, description, notes, vendor, cost, currency, cost_home, fx_rate, parts, tags, source, external_id, firefly_id, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.VehicleID, r.Kind, r.Category, r.Date, nullF(r.Odometer), r.Description, r.Notes, r.Vendor, r.Cost, r.Currency, r.CostHome, r.FXRate, partsJSON(r.Parts), r.Tags, r.Source, r.ExternalID, r.FireflyID, nullI(r.CreatedBy))
	if err != nil {
		return conflict(err)
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

// UpdateRecord saves every column.
func (s *Store) UpdateRecord(ctx context.Context, r *Record) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE records SET kind=?, category=?, date=?, odometer=?, description=?, notes=?, vendor=?, cost=?, currency=?, cost_home=?, fx_rate=?, parts=?, tags=?, source=?, external_id=?, firefly_id=?, updated_at=? WHERE id=? AND vehicle_id=?`,
		r.Kind, r.Category, r.Date, nullF(r.Odometer), r.Description, r.Notes, r.Vendor, r.Cost, r.Currency, r.CostHome, r.FXRate, partsJSON(r.Parts), r.Tags, r.Source, r.ExternalID, r.FireflyID, Now(), r.ID, r.VehicleID)
	return conflict(err)
}

// GetRecord loads one record.
func (s *Store) GetRecord(ctx context.Context, vehicleID, id int64) (*Record, error) {
	return scanRecord(s.DB.QueryRowContext(ctx, `SELECT `+recordCols+` FROM records WHERE id = ? AND vehicle_id = ?`, id, vehicleID))
}

// RecordFilter narrows ListRecords.
type RecordFilter struct {
	Kind     string // "" = all
	Query    string
	From, To string
	Limit    int
}

// ListRecords returns records newest first.
func (s *Store) ListRecords(ctx context.Context, vehicleID int64, f RecordFilter) ([]Record, error) {
	q := `SELECT ` + recordCols + ` FROM records WHERE vehicle_id = ?`
	args := []any{vehicleID}
	if f.Kind != "" {
		kinds := strings.Split(f.Kind, ",")
		q += ` AND kind IN (?` + strings.Repeat(",?", len(kinds)-1) + `)`
		for _, k := range kinds {
			args = append(args, strings.TrimSpace(k))
		}
	}
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q += ` AND (description LIKE ? OR notes LIKE ? OR vendor LIKE ? OR category LIKE ? OR tags LIKE ? OR parts LIKE ?)`
		args = append(args, like, like, like, like, like, like)
	}
	if f.From != "" {
		q += ` AND date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND date <= ?`
		args = append(args, f.To)
	}
	q += ` ORDER BY date DESC, id DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// DeleteRecord removes a record (documents linked to it stay, unlinked).
func (s *Store) DeleteRecord(ctx context.Context, vehicleID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM records WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE documents SET link_kind = '', link_id = 0 WHERE vehicle_id = ? AND link_kind = 'record' AND link_id = ?`, vehicleID, id)
	_, _ = s.DB.ExecContext(ctx, `UPDATE plans SET record_id = NULL WHERE vehicle_id = ? AND record_id = ?`, vehicleID, id)
	return nil
}

// RecordByFirefly finds the record or fuel row already tied to a Firefly journal.
func (s *Store) RecordByFirefly(ctx context.Context, vehicleID int64, journalID string) (kind string, id int64, ok bool) {
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM records WHERE vehicle_id = ? AND firefly_id = ?`, vehicleID, journalID).Scan(&id); err == nil {
		return "record", id, true
	}
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM fuel WHERE vehicle_id = ? AND firefly_id = ?`, vehicleID, journalID).Scan(&id); err == nil {
		return "fuel", id, true
	}
	return "", 0, false
}

// Categories returns the distinct categories and vendors used so far (for suggestions).
func (s *Store) Categories(ctx context.Context, vehicleID int64) (cats, vendors, stations []string) {
	collect := func(q string) []string {
		rows, err := s.DB.QueryContext(ctx, q, vehicleID)
		if err != nil {
			return nil
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil && v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	cats = collect(`SELECT category FROM records WHERE vehicle_id = ? AND category <> '' GROUP BY category ORDER BY COUNT(1) DESC LIMIT 40`)
	vendors = collect(`SELECT vendor FROM records WHERE vehicle_id = ? AND vendor <> '' GROUP BY vendor ORDER BY COUNT(1) DESC LIMIT 40`)
	stations = collect(`SELECT station FROM fuel WHERE vehicle_id = ? AND station <> '' GROUP BY station ORDER BY COUNT(1) DESC LIMIT 40`)
	return
}
