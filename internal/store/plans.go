package store

import "context"

const planCols = `id, vehicle_id, title, notes, kind, category, priority, stage, estimate, target_date, record_id, sort_order, external_id, created_at, updated_at`

func scanPlan(row interface{ Scan(...any) error }) (*Plan, error) {
	var p Plan
	if err := row.Scan(&p.ID, &p.VehicleID, &p.Title, &p.Notes, &p.Kind, &p.Category, &p.Priority, &p.Stage, &p.Estimate, &p.TargetDate, &p.RecordID, &p.SortOrder, &p.ExternalID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

// CreatePlan inserts a planner card at the end of its stage.
func (s *Store) CreatePlan(ctx context.Context, p *Plan) error {
	if p.Stage == "" {
		p.Stage = "backlog"
	}
	if p.Priority == "" {
		p.Priority = "normal"
	}
	var next int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM plans WHERE vehicle_id = ? AND stage = ?`, p.VehicleID, p.Stage).Scan(&next)
	p.SortOrder = next
	res, err := s.DB.ExecContext(ctx, `INSERT INTO plans (vehicle_id, title, notes, kind, category, priority, stage, estimate, target_date, record_id, sort_order, external_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.VehicleID, p.Title, p.Notes, p.Kind, p.Category, p.Priority, p.Stage, p.Estimate, p.TargetDate, nullI(p.RecordID), p.SortOrder, p.ExternalID)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// UpdatePlan saves every column.
func (s *Store) UpdatePlan(ctx context.Context, p *Plan) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE plans SET title=?, notes=?, kind=?, category=?, priority=?, stage=?, estimate=?, target_date=?, record_id=?, sort_order=?, updated_at=? WHERE id=? AND vehicle_id=?`,
		p.Title, p.Notes, p.Kind, p.Category, p.Priority, p.Stage, p.Estimate, p.TargetDate, nullI(p.RecordID), p.SortOrder, Now(), p.ID, p.VehicleID)
	return err
}

// GetPlan loads one card.
func (s *Store) GetPlan(ctx context.Context, vehicleID, id int64) (*Plan, error) {
	return scanPlan(s.DB.QueryRowContext(ctx, `SELECT `+planCols+` FROM plans WHERE id = ? AND vehicle_id = ?`, id, vehicleID))
}

// ListPlans returns the board in stage/sort order.
func (s *Store) ListPlans(ctx context.Context, vehicleID int64) ([]Plan, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+planCols+` FROM plans WHERE vehicle_id = ? ORDER BY stage, sort_order, id`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ReorderPlans applies an ordered list of ids inside a stage.
func (s *Store) ReorderPlans(ctx context.Context, vehicleID int64, stage string, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE plans SET stage = ?, sort_order = ?, updated_at = ? WHERE id = ? AND vehicle_id = ?`, stage, i+1, Now(), id, vehicleID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeletePlan removes a card.
func (s *Store) DeletePlan(ctx context.Context, vehicleID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM plans WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- notes ----

// CreateNote inserts a note.
func (s *Store) CreateNote(ctx context.Context, n *Note) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO notes (vehicle_id, title, body, pinned, external_id) VALUES (?, ?, ?, ?, ?)`, n.VehicleID, n.Title, n.Body, boolInt(n.Pinned), n.ExternalID)
	if err != nil {
		return conflict(err)
	}
	n.ID, _ = res.LastInsertId()
	return nil
}

// UpdateNote saves a note.
func (s *Store) UpdateNote(ctx context.Context, n *Note) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE notes SET title=?, body=?, pinned=?, updated_at=? WHERE id=? AND vehicle_id=?`, n.Title, n.Body, boolInt(n.Pinned), Now(), n.ID, n.VehicleID)
	return err
}

// GetNote loads a note.
func (s *Store) GetNote(ctx context.Context, vehicleID, id int64) (*Note, error) {
	var n Note
	var pinned int
	if err := s.DB.QueryRowContext(ctx, `SELECT id, vehicle_id, title, body, pinned, external_id, created_at, updated_at FROM notes WHERE id = ? AND vehicle_id = ?`, id, vehicleID).Scan(&n.ID, &n.VehicleID, &n.Title, &n.Body, &pinned, &n.ExternalID, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	n.Pinned = pinned == 1
	return &n, nil
}

// ListNotes returns notes, pinned first then newest.
func (s *Store) ListNotes(ctx context.Context, vehicleID int64) ([]Note, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, vehicle_id, title, body, pinned, external_id, created_at, updated_at FROM notes WHERE vehicle_id = ? ORDER BY pinned DESC, updated_at DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		var pinned int
		if err := rows.Scan(&n.ID, &n.VehicleID, &n.Title, &n.Body, &pinned, &n.ExternalID, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		n.Pinned = pinned == 1
		out = append(out, n)
	}
	return out, rows.Err()
}

// DeleteNote removes a note.
func (s *Store) DeleteNote(ctx context.Context, vehicleID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM notes WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- documents ----

const docCols = `id, vehicle_id, user_id, name, filename, mime, bytes, kind, expires_on, link_kind, link_id, created_at`

func scanDoc(row interface{ Scan(...any) error }) (*Document, error) {
	var d Document
	if err := row.Scan(&d.ID, &d.VehicleID, &d.UserID, &d.Name, &d.Filename, &d.Mime, &d.Bytes, &d.Kind, &d.ExpiresOn, &d.LinkKind, &d.LinkID, &d.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &d, nil
}

// CreateDocument inserts a document row (the file is already on disk).
func (s *Store) CreateDocument(ctx context.Context, d *Document) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO documents (vehicle_id, user_id, name, filename, mime, bytes, kind, expires_on, link_kind, link_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.VehicleID, nullI(d.UserID), d.Name, d.Filename, d.Mime, d.Bytes, d.Kind, d.ExpiresOn, d.LinkKind, d.LinkID)
	if err != nil {
		return err
	}
	d.ID, _ = res.LastInsertId()
	return nil
}

// UpdateDocument saves the editable fields.
func (s *Store) UpdateDocument(ctx context.Context, d *Document) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE documents SET name=?, kind=?, expires_on=?, link_kind=?, link_id=? WHERE id=? AND vehicle_id=?`, d.Name, d.Kind, d.ExpiresOn, d.LinkKind, d.LinkID, d.ID, d.VehicleID)
	return err
}

// GetDocument loads a document.
func (s *Store) GetDocument(ctx context.Context, vehicleID, id int64) (*Document, error) {
	return scanDoc(s.DB.QueryRowContext(ctx, `SELECT `+docCols+` FROM documents WHERE id = ? AND vehicle_id = ?`, id, vehicleID))
}

// ListDocuments returns documents newest first.
func (s *Store) ListDocuments(ctx context.Context, vehicleID int64) ([]Document, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+docCols+` FROM documents WHERE vehicle_id = ? ORDER BY id DESC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// DeleteDocument removes the row and returns the filename to unlink.
func (s *Store) DeleteDocument(ctx context.Context, vehicleID, id int64) (string, error) {
	d, err := s.GetDocument(ctx, vehicleID, id)
	if err != nil {
		return "", err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM documents WHERE id = ? AND vehicle_id = ?`, id, vehicleID)
	return d.Filename, err
}

// ExpiringDocuments lists documents whose expires_on falls within the window.
func (s *Store) ExpiringDocuments(ctx context.Context, vehicleID int64, until string) ([]Document, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+docCols+` FROM documents WHERE vehicle_id = ? AND expires_on <> '' AND expires_on <= ? ORDER BY expires_on`, vehicleID, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
