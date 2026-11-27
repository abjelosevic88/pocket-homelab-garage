package store

import (
	"context"
)

// Reads for the box-wide Pocket apps (Search, Day, Hub): the same rows the
// vehicle pages show, across every vehicle a person owns or was added to.
// Who may see a row is decided through the vehicle, as everywhere else in
// Garage, never by the row's own user_id.

// seenBy limits a table's vehicle_id to the vehicles this person can see.
const seenBy = ` vehicle_id IN (SELECT vehicle_id FROM vehicle_members WHERE user_id = ?)`

// FuelOn returns the fill-ups dated that day on the person's vehicles.
func (s *Store) FuelOn(ctx context.Context, userID int64, date string) ([]Fuel, error) {
	return s.fuelWhere(ctx, `date = ? AND`+seenBy+` ORDER BY created_at, id`, date, userID)
}

// RecordsOn returns the service, repair, upgrade and expense records dated
// that day on the person's vehicles.
func (s *Store) RecordsOn(ctx context.Context, userID int64, date string) ([]Record, error) {
	return s.recordsWhere(ctx, `date = ? AND`+seenBy+` ORDER BY created_at, id`, date, userID)
}

// RemindersDueOn returns the active reminders due that day on the person's vehicles.
func (s *Store) RemindersDueOn(ctx context.Context, userID int64, date string) ([]Reminder, error) {
	return s.remindersWhere(ctx, `status = 'active' AND due_date = ? AND`+seenBy+` ORDER BY id`, date, userID)
}

// SearchFuel finds fill-ups by station, grade, notes or tags, newest first.
func (s *Store) SearchFuel(ctx context.Context, userID int64, q string, limit int) ([]Fuel, error) {
	like := "%" + q + "%"
	return s.fuelWhere(ctx, `(station LIKE ? OR grade LIKE ? OR notes LIKE ? OR tags LIKE ?) AND`+seenBy+` ORDER BY date DESC, id DESC LIMIT ?`,
		like, like, like, like, userID, limit)
}

// SearchRecords finds records with the words of the service list's own search
// (description, notes, workshop, category, tags, parts), newest first.
func (s *Store) SearchRecords(ctx context.Context, userID int64, q string, limit int) ([]Record, error) {
	like := "%" + q + "%"
	return s.recordsWhere(ctx, `(description LIKE ? OR notes LIKE ? OR vendor LIKE ? OR category LIKE ? OR tags LIKE ? OR parts LIKE ?) AND`+seenBy+` ORDER BY date DESC, id DESC LIMIT ?`,
		like, like, like, like, like, like, userID, limit)
}

// SearchReminders finds reminders by title, notes or category, the active
// ones first and the soonest due on top.
func (s *Store) SearchReminders(ctx context.Context, userID int64, q string, limit int) ([]Reminder, error) {
	like := "%" + q + "%"
	return s.remindersWhere(ctx, `(title LIKE ? OR notes LIKE ? OR category LIKE ?) AND`+seenBy+
		` ORDER BY CASE status WHEN 'active' THEN 0 ELSE 1 END, CASE WHEN due_date = '' THEN 1 ELSE 0 END, due_date, id LIMIT ?`,
		like, like, like, userID, limit)
}

// LastFuel returns a vehicle's newest fill-up (nil when it has none).
func (s *Store) LastFuel(ctx context.Context, vehicleID int64) *Fuel {
	f, err := scanFuel(s.DB.QueryRowContext(ctx, `SELECT `+fuelCols+` FROM fuel WHERE vehicle_id = ? ORDER BY date DESC, odometer DESC, id DESC LIMIT 1`, vehicleID))
	if err != nil {
		return nil
	}
	return f
}

func (s *Store) fuelWhere(ctx context.Context, where string, args ...any) ([]Fuel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+fuelCols+` FROM fuel WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Fuel{}
	for rows.Next() {
		f, err := scanFuel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

func (s *Store) recordsWhere(ctx context.Context, where string, args ...any) ([]Record, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+recordCols+` FROM records WHERE `+where, args...)
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

func (s *Store) remindersWhere(ctx context.Context, where string, args ...any) ([]Reminder, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reminderCols+` FROM reminders WHERE `+where, args...)
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
