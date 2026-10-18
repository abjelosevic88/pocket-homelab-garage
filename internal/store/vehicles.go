package store

import (
	"context"
	"strings"
)

const vehicleCols = `id, owner_id, name, year, make, model, trim, plate, vin, kind, fuel_type, odometer_unit, tank_capacity, currency, color, image_path,
	purchase_date, purchase_price, purchase_odo, sold_date, sold_price, status, odometer_offset, notes, tags, firefly_tag, firefly_account, dawarich, sort_order, external_id, created_at, updated_at`

func scanVehicle(row interface{ Scan(...any) error }) (*Vehicle, error) {
	var v Vehicle
	var daw int
	if err := row.Scan(&v.ID, &v.OwnerID, &v.Name, &v.Year, &v.Make, &v.Model, &v.Trim, &v.Plate, &v.VIN, &v.Kind, &v.FuelType, &v.OdometerUnit, &v.TankCapacity, &v.Currency, &v.Color, &v.ImagePath,
		&v.PurchaseDate, &v.PurchasePrice, &v.PurchaseOdo, &v.SoldDate, &v.SoldPrice, &v.Status, &v.OdometerOffset, &v.Notes, &v.Tags, &v.FireflyTag, &v.FireflyAccount, &daw, &v.SortOrder, &v.ExternalID, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	v.Dawarich = daw == 1
	v.HasImage = v.ImagePath != ""
	v.DisplayName = v.Title()
	return &v, nil
}

// CreateVehicle inserts a vehicle and makes the owner a member.
func (s *Store) CreateVehicle(ctx context.Context, v *Vehicle) error {
	if v.Status == "" {
		v.Status = "active"
	}
	if v.OdometerUnit == "" {
		v.OdometerUnit = "km"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO vehicles (owner_id, name, year, make, model, trim, plate, vin, kind, fuel_type, odometer_unit, tank_capacity, currency, color, image_path,
		purchase_date, purchase_price, purchase_odo, sold_date, sold_price, status, odometer_offset, notes, tags, firefly_tag, firefly_account, dawarich, sort_order, external_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.OwnerID, v.Name, v.Year, v.Make, v.Model, v.Trim, v.Plate, v.VIN, v.Kind, v.FuelType, v.OdometerUnit, v.TankCapacity, v.Currency, v.Color, v.ImagePath,
		v.PurchaseDate, v.PurchasePrice, v.PurchaseOdo, v.SoldDate, v.SoldPrice, v.Status, v.OdometerOffset, v.Notes, v.Tags, v.FireflyTag, v.FireflyAccount, boolInt(v.Dawarich), v.SortOrder, v.ExternalID)
	if err != nil {
		return conflict(err)
	}
	v.ID, _ = res.LastInsertId()
	_, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO vehicle_members (vehicle_id, user_id, role) VALUES (?, ?, 'owner')`, v.ID, v.OwnerID)
	v.DisplayName = v.Title()
	return err
}

// UpdateVehicle saves every column.
func (s *Store) UpdateVehicle(ctx context.Context, v *Vehicle) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE vehicles SET name=?, year=?, make=?, model=?, trim=?, plate=?, vin=?, kind=?, fuel_type=?, odometer_unit=?, tank_capacity=?, currency=?, color=?, image_path=?,
		purchase_date=?, purchase_price=?, purchase_odo=?, sold_date=?, sold_price=?, status=?, odometer_offset=?, notes=?, tags=?, firefly_tag=?, firefly_account=?, dawarich=?, sort_order=?, external_id=?, updated_at=? WHERE id=?`,
		v.Name, v.Year, v.Make, v.Model, v.Trim, v.Plate, v.VIN, v.Kind, v.FuelType, v.OdometerUnit, v.TankCapacity, v.Currency, v.Color, v.ImagePath,
		v.PurchaseDate, v.PurchasePrice, v.PurchaseOdo, v.SoldDate, v.SoldPrice, v.Status, v.OdometerOffset, v.Notes, v.Tags, v.FireflyTag, v.FireflyAccount, boolInt(v.Dawarich), v.SortOrder, v.ExternalID, Now(), v.ID)
	v.DisplayName = v.Title()
	return conflict(err)
}

// GetVehicle loads a vehicle with members and the caller's role.
func (s *Store) GetVehicle(ctx context.Context, id, userID int64) (*Vehicle, error) {
	v, err := scanVehicle(s.DB.QueryRowContext(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	v.Members, _ = s.VehicleMembers(ctx, id)
	for _, m := range v.Members {
		if m.UserID == userID {
			v.Role = m.Role
		}
	}
	return v, nil
}

// VehicleMembers lists who can see a vehicle.
func (s *Store) VehicleMembers(ctx context.Context, id int64) ([]VehicleMember, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT m.user_id, u.name, u.email, m.role FROM vehicle_members m JOIN users u ON u.id = m.user_id WHERE m.vehicle_id = ? ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, u.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VehicleMember{}
	for rows.Next() {
		var m VehicleMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetVehicleMembers replaces the non-owner members.
func (s *Store) SetVehicleMembers(ctx context.Context, id int64, members []VehicleMember) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM vehicle_members WHERE vehicle_id = ? AND role <> 'owner'`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, m := range members {
		role := m.Role
		if role != "editor" && role != "viewer" {
			role = "editor"
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO vehicle_members (vehicle_id, user_id, role) VALUES (?, ?, ?)`, id, m.UserID, role); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// VehicleRole returns the user's role on a vehicle ("" = no access). Admins see everything.
func (s *Store) VehicleRole(ctx context.Context, id int64, u *User) string {
	if u == nil {
		return ""
	}
	var role string
	err := s.DB.QueryRowContext(ctx, `SELECT role FROM vehicle_members WHERE vehicle_id = ? AND user_id = ?`, id, u.ID).Scan(&role)
	if err == nil {
		return role
	}
	if u.Role == "admin" {
		var n int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM vehicles WHERE id = ?`, id).Scan(&n); err == nil && n > 0 {
			return "admin"
		}
	}
	return ""
}

// VehicleFilter narrows ListVehicles.
type VehicleFilter struct {
	UserID   int64 // 0 = everyone's
	Status   string
	Query    string
	Dawarich bool
}

// ListVehicles returns vehicles the user can see, active first.
func (s *Store) ListVehicles(ctx context.Context, f VehicleFilter) ([]Vehicle, error) {
	q := `SELECT DISTINCT v.` + strings.ReplaceAll(vehicleCols, ", ", ", v.") + `, COALESCE(m.role, '') FROM vehicles v LEFT JOIN vehicle_members m ON m.vehicle_id = v.id AND m.user_id = ? WHERE 1=1`
	args := []any{f.UserID}
	if f.UserID > 0 {
		q += ` AND m.user_id IS NOT NULL`
	}
	if f.Status != "" {
		q += ` AND v.status = ?`
		args = append(args, f.Status)
	}
	if f.Dawarich {
		q += ` AND v.dawarich = 1`
	}
	if f.Query != "" {
		q += ` AND (v.name LIKE ? OR v.make LIKE ? OR v.model LIKE ? OR v.plate LIKE ?)`
		like := "%" + f.Query + "%"
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY CASE v.status WHEN 'active' THEN 0 WHEN 'sold' THEN 1 ELSE 2 END, v.sort_order, v.id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vehicle{}
	for rows.Next() {
		var v Vehicle
		var daw int
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.Name, &v.Year, &v.Make, &v.Model, &v.Trim, &v.Plate, &v.VIN, &v.Kind, &v.FuelType, &v.OdometerUnit, &v.TankCapacity, &v.Currency, &v.Color, &v.ImagePath,
			&v.PurchaseDate, &v.PurchasePrice, &v.PurchaseOdo, &v.SoldDate, &v.SoldPrice, &v.Status, &v.OdometerOffset, &v.Notes, &v.Tags, &v.FireflyTag, &v.FireflyAccount, &daw, &v.SortOrder, &v.ExternalID, &v.CreatedAt, &v.UpdatedAt, &v.Role); err != nil {
			return nil, err
		}
		v.Dawarich = daw == 1
		v.HasImage = v.ImagePath != ""
		v.DisplayName = v.Title()
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeleteVehicle removes a vehicle and everything under it.
func (s *Store) DeleteVehicle(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM vehicles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// VehicleByExternalID finds an imported vehicle.
func (s *Store) VehicleByExternalID(ctx context.Context, ext string) (*Vehicle, error) {
	return scanVehicle(s.DB.QueryRowContext(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE external_id = ?`, ext))
}
