package store

import (
	"context"
	"sort"
)

const fuelCols = `id, vehicle_id, date, odometer, volume, cost, currency, cost_home, fx_rate, full_tank, missed, station, grade, soc_start, soc_end, lat, lng, notes, tags, source, external_id, firefly_id, created_by, created_at, updated_at, money_tx_id, money_user_id`

func scanFuel(row interface{ Scan(...any) error }) (*Fuel, error) {
	var f Fuel
	var full, missed int
	if err := row.Scan(&f.ID, &f.VehicleID, &f.Date, &f.Odometer, &f.Volume, &f.Cost, &f.Currency, &f.CostHome, &f.FXRate, &full, &missed, &f.Station, &f.Grade, &f.SocStart, &f.SocEnd, &f.Lat, &f.Lng, &f.Notes, &f.Tags, &f.Source, &f.ExternalID, &f.FireflyID, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt, &f.MoneyTxID, &f.MoneyUserID); err != nil {
		return nil, notFound(err)
	}
	f.FullTank = full == 1
	f.Missed = missed == 1
	if f.Volume > 0 {
		f.PricePer = float64(f.Cost) / f.Volume
	}
	return &f, nil
}

// CreateFuel inserts a fill-up.
func (s *Store) CreateFuel(ctx context.Context, f *Fuel) error {
	if f.Source == "" {
		f.Source = "manual"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO fuel (vehicle_id, date, odometer, volume, cost, currency, cost_home, fx_rate, full_tank, missed, station, grade, soc_start, soc_end, lat, lng, notes, tags, source, external_id, firefly_id, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.VehicleID, f.Date, f.Odometer, f.Volume, f.Cost, f.Currency, f.CostHome, f.FXRate, boolInt(f.FullTank), boolInt(f.Missed), f.Station, f.Grade, f.SocStart, f.SocEnd, nullF(f.Lat), nullF(f.Lng), f.Notes, f.Tags, f.Source, f.ExternalID, f.FireflyID, nullI(f.CreatedBy))
	if err != nil {
		return conflict(err)
	}
	f.ID, _ = res.LastInsertId()
	return nil
}

// UpdateFuel saves every column.
func (s *Store) UpdateFuel(ctx context.Context, f *Fuel) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE fuel SET date=?, odometer=?, volume=?, cost=?, currency=?, cost_home=?, fx_rate=?, full_tank=?, missed=?, station=?, grade=?, soc_start=?, soc_end=?, lat=?, lng=?, notes=?, tags=?, source=?, external_id=?, firefly_id=?, updated_at=? WHERE id=? AND vehicle_id=?`,
		f.Date, f.Odometer, f.Volume, f.Cost, f.Currency, f.CostHome, f.FXRate, boolInt(f.FullTank), boolInt(f.Missed), f.Station, f.Grade, f.SocStart, f.SocEnd, nullF(f.Lat), nullF(f.Lng), f.Notes, f.Tags, f.Source, f.ExternalID, f.FireflyID, Now(), f.ID, f.VehicleID)
	return conflict(err)
}

// SetFuelMoney links (txID > 0) or unlinks a fill-up and its Pocket Money transaction.
func (s *Store) SetFuelMoney(ctx context.Context, vehicleID, id, txID, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE fuel SET money_tx_id = ?, money_user_id = ? WHERE id = ? AND vehicle_id = ?`, nullPos(txID), nullPos(userID), id, vehicleID)
	return err
}

// GetFuel loads one fill-up (without derived stretch figures).
func (s *Store) GetFuel(ctx context.Context, vehicleID, id int64) (*Fuel, error) {
	return scanFuel(s.DB.QueryRowContext(ctx, `SELECT `+fuelCols+` FROM fuel WHERE id = ? AND vehicle_id = ?`, id, vehicleID))
}

// ListFuel returns fill-ups newest first with distance, economy and cost per
// km computed over the stretch since the previous full tank (partial fills
// are summed into the next full one, as LubeLogger and Fuelly do).
func (s *Store) ListFuel(ctx context.Context, vehicleID int64) ([]Fuel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+fuelCols+` FROM fuel WHERE vehicle_id = ? ORDER BY date, odometer, id`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	asc := []Fuel{}
	for rows.Next() {
		f, err := scanFuel(rows)
		if err != nil {
			return nil, err
		}
		asc = append(asc, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ComputeFuelStats(asc)
	sort.SliceStable(asc, func(i, j int) bool {
		if asc[i].Date != asc[j].Date {
			return asc[i].Date > asc[j].Date
		}
		return asc[i].Odometer > asc[j].Odometer
	})
	return asc, nil
}

// ComputeFuelStats fills Distance / Economy / CostPerKm on an ascending list.
func ComputeFuelStats(asc []Fuel) {
	var prevOdo float64
	havePrev := false
	var stretchVol float64
	var stretchCost int64
	var stretchStart float64
	stretchOpen := false
	for i := range asc {
		f := &asc[i]
		if havePrev && f.Odometer > prevOdo {
			f.Distance = f.Odometer - prevOdo
		}
		if f.Missed || !havePrev {
			// a gap: start fresh from this fill
			stretchOpen = f.FullTank
			stretchStart = f.Odometer
			stretchVol, stretchCost = 0, 0
			prevOdo, havePrev = f.Odometer, true
			continue
		}
		stretchVol += f.Volume
		stretchCost += f.CostHome
		if f.FullTank {
			if stretchOpen && f.Odometer > stretchStart && stretchVol > 0 {
				d := f.Odometer - stretchStart
				f.Economy = stretchVol / d * 100
				f.CostPerKm = float64(stretchCost) / d
			}
			stretchOpen = true
			stretchStart = f.Odometer
			stretchVol, stretchCost = 0, 0
		}
		prevOdo = f.Odometer
	}
}

// DeleteFuel removes a fill-up.
func (s *Store) DeleteFuel(ctx context.Context, vehicleID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM fuel WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE documents SET link_kind = '', link_id = 0 WHERE vehicle_id = ? AND link_kind = 'fuel' AND link_id = ?`, vehicleID, id)
	return nil
}
