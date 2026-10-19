package store

import (
	"context"
	"math"
	"time"
)

const reminderCols = `id, vehicle_id, title, notes, kind, category, due_date, due_odometer, interval_days, interval_km, soon_days, soon_km, status, last_done_date, last_done_odo, notified_level, notified_at, external_id, created_at, updated_at`

func scanReminder(row interface{ Scan(...any) error }) (*Reminder, error) {
	var r Reminder
	if err := row.Scan(&r.ID, &r.VehicleID, &r.Title, &r.Notes, &r.Kind, &r.Category, &r.DueDate, &r.DueOdometer, &r.IntervalDays, &r.IntervalKm, &r.SoonDays, &r.SoonKm, &r.Status, &r.LastDoneDate, &r.LastDoneOdo, &r.NotifiedLevel, &r.NotifiedAt, &r.ExternalID, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &r, nil
}

// CreateReminder inserts a reminder.
func (s *Store) CreateReminder(ctx context.Context, r *Reminder) error {
	if r.Status == "" {
		r.Status = "active"
	}
	if r.SoonDays == 0 {
		r.SoonDays = 14
	}
	if r.SoonKm == 0 {
		r.SoonKm = 500
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO reminders (vehicle_id, title, notes, kind, category, due_date, due_odometer, interval_days, interval_km, soon_days, soon_km, status, last_done_date, last_done_odo, external_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.VehicleID, r.Title, r.Notes, r.Kind, r.Category, r.DueDate, nullF(r.DueOdometer), r.IntervalDays, r.IntervalKm, r.SoonDays, r.SoonKm, r.Status, r.LastDoneDate, nullF(r.LastDoneOdo), r.ExternalID)
	if err != nil {
		return conflict(err)
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

// UpdateReminder saves every column.
func (s *Store) UpdateReminder(ctx context.Context, r *Reminder) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE reminders SET title=?, notes=?, kind=?, category=?, due_date=?, due_odometer=?, interval_days=?, interval_km=?, soon_days=?, soon_km=?, status=?, last_done_date=?, last_done_odo=?, notified_level=?, notified_at=?, updated_at=? WHERE id=? AND vehicle_id=?`,
		r.Title, r.Notes, r.Kind, r.Category, r.DueDate, nullF(r.DueOdometer), r.IntervalDays, r.IntervalKm, r.SoonDays, r.SoonKm, r.Status, r.LastDoneDate, nullF(r.LastDoneOdo), r.NotifiedLevel, r.NotifiedAt, Now(), r.ID, r.VehicleID)
	return err
}

// GetReminder loads one reminder (urgency not computed).
func (s *Store) GetReminder(ctx context.Context, vehicleID, id int64) (*Reminder, error) {
	return scanReminder(s.DB.QueryRowContext(ctx, `SELECT `+reminderCols+` FROM reminders WHERE id = ? AND vehicle_id = ?`, id, vehicleID))
}

// ListReminders returns one vehicle's reminders (urgency not computed).
func (s *Store) ListReminders(ctx context.Context, vehicleID int64, status string) ([]Reminder, error) {
	q := `SELECT ` + reminderCols + ` FROM reminders WHERE vehicle_id = ?`
	args := []any{vehicleID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date, id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reminder{}
	for rows.Next() {
		r, err := scanReminder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// DeleteReminder removes a reminder.
func (s *Store) DeleteReminder(ctx context.Context, vehicleID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM reminders WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Urgency order for sorting: overdue first.
var UrgencyRank = map[string]int{"overdue": 0, "due": 1, "soon": 2, "ok": 3, "done": 4}

// ComputeUrgency fills Urgency, DaysLeft and KmLeft against today and the odometer estimate.
func ComputeUrgency(r *Reminder, today string, odometer float64) {
	if r.Status != "active" {
		r.Urgency = "done"
		return
	}
	r.Urgency = "ok"
	if r.DueDate != "" {
		d := daysSigned(today, r.DueDate)
		r.DaysLeft = &d
	}
	if r.DueOdometer != nil && odometer > 0 {
		k := *r.DueOdometer - odometer
		r.KmLeft = &k
	}
	level := func(l string) {
		if UrgencyRank[l] < UrgencyRank[r.Urgency] {
			r.Urgency = l
		}
	}
	if r.DaysLeft != nil {
		switch {
		case *r.DaysLeft < 0:
			level("overdue")
		case *r.DaysLeft == 0:
			level("due")
		case *r.DaysLeft <= r.SoonDays:
			level("soon")
		}
	}
	if r.KmLeft != nil {
		switch {
		case *r.KmLeft < 0:
			level("overdue")
		case *r.KmLeft == 0:
			level("due")
		case *r.KmLeft <= r.SoonKm:
			level("soon")
		}
	}
}

// daysSigned returns b-a in days, negative when b is before a.
func daysSigned(a, b string) int {
	ta, err1 := ParseDate(a)
	tb, err2 := ParseDate(b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

// RollForward moves a recurring reminder to its next due point from the
// date/odometer it was done at; non-recurring reminders become done.
func RollForward(r *Reminder, doneDate string, doneOdo *float64) {
	r.LastDoneDate = doneDate
	r.LastDoneOdo = doneOdo
	recurring := r.IntervalDays > 0 || r.IntervalKm > 0
	if !recurring {
		r.Status = "done"
		return
	}
	if r.IntervalDays > 0 {
		base, err := ParseDate(doneDate)
		if err != nil {
			base = time.Now()
		}
		r.DueDate = base.AddDate(0, 0, r.IntervalDays).Format("2006-01-02")
	} else {
		r.DueDate = ""
	}
	if r.IntervalKm > 0 && doneOdo != nil {
		n := *doneOdo + r.IntervalKm
		r.DueOdometer = &n
	} else if r.IntervalKm == 0 {
		r.DueOdometer = nil
	}
	r.NotifiedLevel = ""
	r.NotifiedAt = ""
}
