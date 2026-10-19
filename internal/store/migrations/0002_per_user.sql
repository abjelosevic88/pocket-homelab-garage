-- Per-person data.
--
-- A vehicle belongs to its owner (vehicles.owner_id) and is shared only through
-- vehicle_members. Everything under a vehicle now carries user_id = the
-- vehicle's owner so every content row names the person it belongs to; access
-- is still decided through the vehicle (owner or member), never by user_id
-- alone. Rows inserted without a user_id get the vehicle owner from a trigger,
-- so background jobs and imports can't leave a row unowned. documents.user_id
-- stays "who uploaded it" and is only filled in where it was missing;
-- activity.user_id stays "who did it" and the log is reached through its vehicle.
--
-- Settings split in two: the box-wide ones (title, exchange-rate lookups,
-- setup flag) stay in `settings`, everything a person chooses (currency,
-- units, formats, Firefly / Dawarich / notifications / Travel / Inventory
-- credentials) moves to user_settings. What existed so far belongs to the
-- first admin, the account that configured it.
--
-- The runner applies a file once inside a transaction; the statements below
-- are additionally written to be safe to re-run.

ALTER TABLE records       ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE fuel          ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE odometer      ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE reminders     ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE plans         ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE notes         ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE dawarich_days ADD COLUMN user_id INTEGER REFERENCES users(id);

UPDATE records       SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = records.vehicle_id)       WHERE user_id IS NULL;
UPDATE fuel          SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = fuel.vehicle_id)          WHERE user_id IS NULL;
UPDATE odometer      SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = odometer.vehicle_id)      WHERE user_id IS NULL;
UPDATE reminders     SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = reminders.vehicle_id)     WHERE user_id IS NULL;
UPDATE plans         SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = plans.vehicle_id)         WHERE user_id IS NULL;
UPDATE notes         SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = notes.vehicle_id)         WHERE user_id IS NULL;
UPDATE dawarich_days SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = dawarich_days.vehicle_id) WHERE user_id IS NULL;
UPDATE documents     SET user_id = (SELECT owner_id FROM vehicles v WHERE v.id = documents.vehicle_id)     WHERE user_id IS NULL;

CREATE INDEX IF NOT EXISTS vehicles_owner      ON vehicles(owner_id);
CREATE INDEX IF NOT EXISTS members_user        ON vehicle_members(user_id);
CREATE INDEX IF NOT EXISTS records_user        ON records(user_id);
CREATE INDEX IF NOT EXISTS fuel_user           ON fuel(user_id);
CREATE INDEX IF NOT EXISTS odometer_user       ON odometer(user_id);
CREATE INDEX IF NOT EXISTS reminders_user      ON reminders(user_id);
CREATE INDEX IF NOT EXISTS plans_user          ON plans(user_id);
CREATE INDEX IF NOT EXISTS notes_user          ON notes(user_id);
CREATE INDEX IF NOT EXISTS notes_vehicle       ON notes(vehicle_id);
CREATE INDEX IF NOT EXISTS dawarich_days_user  ON dawarich_days(user_id);
CREATE INDEX IF NOT EXISTS documents_user      ON documents(user_id);

CREATE TRIGGER IF NOT EXISTS records_owner AFTER INSERT ON records WHEN NEW.user_id IS NULL BEGIN
  UPDATE records SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS fuel_owner AFTER INSERT ON fuel WHEN NEW.user_id IS NULL BEGIN
  UPDATE fuel SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS odometer_owner AFTER INSERT ON odometer WHEN NEW.user_id IS NULL BEGIN
  UPDATE odometer SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS reminders_owner AFTER INSERT ON reminders WHEN NEW.user_id IS NULL BEGIN
  UPDATE reminders SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS plans_owner AFTER INSERT ON plans WHEN NEW.user_id IS NULL BEGIN
  UPDATE plans SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS notes_owner AFTER INSERT ON notes WHEN NEW.user_id IS NULL BEGIN
  UPDATE notes SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS dawarich_days_owner AFTER INSERT ON dawarich_days WHEN NEW.user_id IS NULL BEGIN
  UPDATE dawarich_days SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE vehicle_id = NEW.vehicle_id AND date = NEW.date;
END;
CREATE TRIGGER IF NOT EXISTS documents_owner AFTER INSERT ON documents WHEN NEW.user_id IS NULL BEGIN
  UPDATE documents SET user_id = (SELECT owner_id FROM vehicles WHERE id = NEW.vehicle_id) WHERE id = NEW.id;
END;

-- Should a vehicle ever change hands, its rows follow it.
CREATE TRIGGER IF NOT EXISTS vehicles_owner_follow AFTER UPDATE OF owner_id ON vehicles WHEN NEW.owner_id IS NOT OLD.owner_id BEGIN
  UPDATE records       SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
  UPDATE fuel          SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
  UPDATE odometer      SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
  UPDATE reminders     SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
  UPDATE plans         SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
  UPDATE notes         SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
  UPDATE dawarich_days SET user_id = NEW.owner_id WHERE vehicle_id = NEW.id;
END;

CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

INSERT OR IGNORE INTO user_settings (user_id, key, value)
  SELECT (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1), key, value FROM settings
  WHERE key IN ('home_currency', 'date_format', 'time_format', 'distance_unit', 'volume_unit', 'economy_unit',
                'firefly_url', 'firefly_token', 'firefly_category', 'firefly_account', 'firefly_auto_sync',
                'dawarich_url', 'dawarich_api_key',
                'notify_url', 'notify_format', 'notify_hour', 'notify_digest',
                'travel_url', 'travel_token', 'travel_public_url', 'inventory_url', 'inventory_token')
    AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');

DELETE FROM settings
  WHERE key IN ('home_currency', 'date_format', 'time_format', 'distance_unit', 'volume_unit', 'economy_unit',
                'firefly_url', 'firefly_token', 'firefly_category', 'firefly_account', 'firefly_auto_sync',
                'dawarich_url', 'dawarich_api_key',
                'notify_url', 'notify_format', 'notify_hour', 'notify_digest',
                'travel_url', 'travel_token', 'travel_public_url', 'inventory_url', 'inventory_token')
    AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');

-- Per-person caches: the Pocket Travel trip list and the last notification day.
UPDATE cache SET key = 'travel:trips:' || (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1)
  WHERE key = 'travel:trips' AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');
UPDATE cache SET key = 'notify:last_day:' || (SELECT id FROM users WHERE role = 'admin' ORDER BY id LIMIT 1)
  WHERE key = 'notify:last_day' AND EXISTS (SELECT 1 FROM users WHERE role = 'admin');
