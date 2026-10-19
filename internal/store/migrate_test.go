package store

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

// TestPerUserMigration builds a database as v0.1.x left it (only 0001, the
// owner's vehicle and global integration settings) and checks that opening it
// backfills every row to the owner and moves the personal settings to them.
func TestPerUserMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	init, _ := migrationFS.ReadFile("migrations/0001_init.sql")
	stmts := []string{
		string(init),
		`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`,
		`INSERT INTO schema_migrations VALUES ('0001_init.sql', 'then')`,
		`INSERT INTO users (id, email, role) VALUES (1, 'owner@example.com', 'admin'), (2, 'member@example.com', 'member')`,
		`INSERT INTO vehicles (id, owner_id, make, model) VALUES (1, 1, 'Skoda', 'Fabia')`,
		`INSERT INTO vehicle_members (vehicle_id, user_id, role) VALUES (1, 1, 'owner')`,
		`INSERT INTO records (vehicle_id, date, description) VALUES (1, '2026-10-01', 'a'), (1, '2026-10-02', 'b')`,
		`INSERT INTO fuel (vehicle_id, date, odometer, volume) VALUES (1, '2026-10-01', 100, 40)`,
		`INSERT INTO odometer (vehicle_id, date, reading) VALUES (1, '2026-10-01', 100)`,
		`INSERT INTO reminders (vehicle_id, title) VALUES (1, 'oil')`,
		`INSERT INTO plans (vehicle_id, title) VALUES (1, 'plan')`,
		`INSERT INTO notes (vehicle_id, title) VALUES (1, 'note')`,
		`INSERT INTO documents (vehicle_id, name, filename) VALUES (1, 'doc', '1/x')`,
		`INSERT INTO dawarich_days (vehicle_id, date, km, fetched_at) VALUES (1, '2026-10-01', 12, 'now')`,
		`INSERT INTO activity (vehicle_id, user_id, action) VALUES (1, 1, 'added vehicle'), (1, NULL, 'imported from Firefly')`,
		`INSERT INTO settings (key, value) VALUES ('title', 'My garage'), ('fx_auto', '1'), ('setup_complete', '1'), ('home_currency', 'BAM'), ('firefly_token', 'tok'), ('dawarich_url', 'http://d'), ('notify_url', 'http://n')`,
		`INSERT INTO cache (key, value, fetched_at) VALUES ('travel:trips', '[]', 'now'), ('notify:last_day', '2026-10-18', 'now'), ('firefly:synced:1', 'x', 'now')`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q[:min(len(q), 60)], err)
		}
	}
	db.Close()

	s, err := Open(path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, table := range []string{"records", "fuel", "odometer", "reminders", "plans", "notes", "documents", "dawarich_days"} {
		var total, owned int
		_ = s.DB.QueryRow(`SELECT COUNT(1), COALESCE(SUM(user_id = 1), 0) FROM `+table).Scan(&total, &owned)
		if total == 0 || total != owned {
			t.Errorf("%s: %d rows, %d owned by the owner", table, total, owned)
		}
	}
	// the actor of a log line is not rewritten
	var nullActors int
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM activity WHERE user_id IS NULL`).Scan(&nullActors)
	if nullActors != 1 {
		t.Errorf("activity actors rewritten: %d null", nullActors)
	}
	// new rows pick up the vehicle's owner
	if err := s.CreateNote(ctx, &Note{VehicleID: 1, Title: "later"}); err != nil {
		t.Fatal(err)
	}
	var uid int64
	_ = s.DB.QueryRow(`SELECT user_id FROM notes WHERE title = 'later'`).Scan(&uid)
	if uid != 1 {
		t.Errorf("new note owned by %d", uid)
	}

	global, _ := s.GetSettings(ctx)
	if global.Title != "My garage" || global.FireflyToken != "" || global.HomeCurrency != "EUR" {
		t.Errorf("global settings after migration: %+v", global)
	}
	owner, _ := s.GetUserSettings(ctx, 1, global)
	if owner.HomeCurrency != "BAM" || owner.FireflyToken != "tok" || owner.DawarichURL != "http://d" || owner.NotifyURL != "http://n" || owner.Title != "My garage" {
		t.Errorf("owner settings after migration: %+v", owner)
	}
	member, _ := s.GetUserSettings(ctx, 2, global)
	if member.FireflyToken != "" || member.NotifyURL != "" || member.HomeCurrency != "EUR" {
		t.Errorf("member inherited the owner's settings: %+v", member)
	}
	if _, _, ok := s.CacheGet(ctx, "travel:trips:1"); !ok {
		t.Error("owner's trip cache not moved")
	}
	if v, _, ok := s.CacheGet(ctx, "notify:last_day:1"); !ok || v != "2026-10-18" {
		t.Error("owner's notify day not moved")
	}
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM settings WHERE key IN ('home_currency', 'firefly_token', 'dawarich_url', 'notify_url')`).Scan(&n)
	if n != 0 {
		t.Errorf("%d personal keys left in the global settings", n)
	}
}
