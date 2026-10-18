CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'admin',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- A vehicle is the unit everything hangs off. Costs are stored in the
-- currency they were paid in and converted into the vehicle's currency at
-- the day's rate (cost_home), so a fill-up abroad still adds up at home.
CREATE TABLE vehicles (
  id              INTEGER PRIMARY KEY,
  owner_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            TEXT NOT NULL DEFAULT '',           -- nickname; "" = year make model
  year            INTEGER NOT NULL DEFAULT 0,
  make            TEXT NOT NULL DEFAULT '',
  model           TEXT NOT NULL DEFAULT '',
  trim            TEXT NOT NULL DEFAULT '',
  plate           TEXT NOT NULL DEFAULT '',
  vin             TEXT NOT NULL DEFAULT '',
  kind            TEXT NOT NULL DEFAULT 'car',        -- car | motorcycle | van | truck | bike | boat | tractor | other
  fuel_type       TEXT NOT NULL DEFAULT 'petrol',     -- petrol | diesel | electric | hybrid | plugin_hybrid | lpg | cng | other
  odometer_unit   TEXT NOT NULL DEFAULT 'km',         -- km | mi | h
  tank_capacity   REAL NOT NULL DEFAULT 0,            -- litres or kWh
  currency        TEXT NOT NULL DEFAULT 'EUR',
  color           TEXT NOT NULL DEFAULT '',
  image_path      TEXT NOT NULL DEFAULT '',
  purchase_date   TEXT NOT NULL DEFAULT '',
  purchase_price  INTEGER NOT NULL DEFAULT 0,         -- minor units of currency
  purchase_odo    REAL NOT NULL DEFAULT 0,
  sold_date       TEXT NOT NULL DEFAULT '',
  sold_price      INTEGER NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT 'active',     -- active | sold | archived
  odometer_offset REAL NOT NULL DEFAULT 0,            -- added to every reading (replaced dash)
  notes           TEXT NOT NULL DEFAULT '',
  tags            TEXT NOT NULL DEFAULT '',
  firefly_tag     TEXT NOT NULL DEFAULT '',
  firefly_account TEXT NOT NULL DEFAULT '',           -- source account for pushed withdrawals
  dawarich        INTEGER NOT NULL DEFAULT 0,         -- this vehicle drives the Dawarich kilometres
  sort_order      INTEGER NOT NULL DEFAULT 0,
  external_id     TEXT NOT NULL DEFAULT '',           -- lubelogger:<id>
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE vehicle_members (
  vehicle_id INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'editor',          -- owner | editor | viewer
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (vehicle_id, user_id)
);

-- Records: service, repairs, upgrades and plain expenses (tax, insurance,
-- registration, wash, parking…) share one table and differ by kind. A
-- record with an odometer IS an odometer reading — there is no mirrored row
-- to drift out of sync.
CREATE TABLE records (
  id          INTEGER PRIMARY KEY,
  vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL DEFAULT 'service',        -- service | repair | upgrade | expense
  category    TEXT NOT NULL DEFAULT '',               -- free text with suggestions: oil, tyres, brakes, insurance…
  date        TEXT NOT NULL,                          -- YYYY-MM-DD
  odometer    REAL,                                   -- NULL = not recorded
  description TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  vendor      TEXT NOT NULL DEFAULT '',
  cost        INTEGER NOT NULL DEFAULT 0,             -- minor units of currency
  currency    TEXT NOT NULL DEFAULT 'EUR',
  cost_home   INTEGER NOT NULL DEFAULT 0,             -- in the vehicle currency
  fx_rate     REAL NOT NULL DEFAULT 1,
  parts       TEXT NOT NULL DEFAULT '[]',             -- JSON [{name, qty, cost, part_no, inventory_id, link}]
  tags        TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT 'manual',         -- manual | api | firefly | lubelogger | reminder | plan
  external_id TEXT NOT NULL DEFAULT '',
  firefly_id  TEXT NOT NULL DEFAULT '',               -- journal id when matched/pushed
  created_by  INTEGER,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX records_vehicle ON records(vehicle_id, date);
CREATE UNIQUE INDEX records_external ON records(vehicle_id, source, external_id) WHERE external_id <> '';

-- Fuel: one row per fill-up (or charge for an EV). Distance and consumption
-- are derived on read from the previous full tank, never stored.
CREATE TABLE fuel (
  id          INTEGER PRIMARY KEY,
  vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  date        TEXT NOT NULL,
  odometer    REAL NOT NULL DEFAULT 0,
  volume      REAL NOT NULL DEFAULT 0,                -- litres (or kWh for electric)
  cost        INTEGER NOT NULL DEFAULT 0,
  currency    TEXT NOT NULL DEFAULT 'EUR',
  cost_home   INTEGER NOT NULL DEFAULT 0,
  fx_rate     REAL NOT NULL DEFAULT 1,
  full_tank   INTEGER NOT NULL DEFAULT 1,
  missed      INTEGER NOT NULL DEFAULT 0,             -- a fill-up before this one was not logged
  station     TEXT NOT NULL DEFAULT '',
  grade       TEXT NOT NULL DEFAULT '',               -- 95, 98, diesel, AdBlue…
  soc_start   REAL NOT NULL DEFAULT 0,                -- EV state of charge %
  soc_end     REAL NOT NULL DEFAULT 0,
  lat         REAL,
  lng         REAL,
  notes       TEXT NOT NULL DEFAULT '',
  tags        TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT 'manual',
  external_id TEXT NOT NULL DEFAULT '',
  firefly_id  TEXT NOT NULL DEFAULT '',
  created_by  INTEGER,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX fuel_vehicle ON fuel(vehicle_id, date, odometer);
CREATE UNIQUE INDEX fuel_external ON fuel(vehicle_id, source, external_id) WHERE external_id <> '';

-- Standalone odometer readings (a glance at the dash, a confirmed Dawarich
-- estimate, an API ping). Readings carried by records and fuel are unioned
-- in at query time.
CREATE TABLE odometer (
  id          INTEGER PRIMARY KEY,
  vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  date        TEXT NOT NULL,
  reading     REAL NOT NULL,
  notes       TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT 'manual',         -- manual | api | dawarich | lubelogger
  external_id TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX odometer_vehicle ON odometer(vehicle_id, date);
CREATE UNIQUE INDEX odometer_external ON odometer(vehicle_id, source, external_id) WHERE external_id <> '';

-- Kilometres Dawarich recorded per day (driving mode), for the odometer
-- estimate and the "km since last service" figure.
CREATE TABLE dawarich_days (
  vehicle_id INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  km         REAL NOT NULL DEFAULT 0,
  total_km   REAL NOT NULL DEFAULT 0,
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (vehicle_id, date)
);

-- Reminders: due by date, by odometer, or whichever comes first. Recurring
-- ones roll forward when marked done; marking done can log a record.
CREATE TABLE reminders (
  id              INTEGER PRIMARY KEY,
  vehicle_id      INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  title           TEXT NOT NULL,
  notes           TEXT NOT NULL DEFAULT '',
  kind            TEXT NOT NULL DEFAULT 'service',    -- what "done" logs: service | repair | upgrade | expense | none
  category        TEXT NOT NULL DEFAULT '',
  due_date        TEXT NOT NULL DEFAULT '',
  due_odometer    REAL,
  interval_days   INTEGER NOT NULL DEFAULT 0,
  interval_km     REAL NOT NULL DEFAULT 0,
  soon_days       INTEGER NOT NULL DEFAULT 14,        -- "due soon" window
  soon_km         REAL NOT NULL DEFAULT 500,
  status          TEXT NOT NULL DEFAULT 'active',     -- active | done | dismissed
  last_done_date  TEXT NOT NULL DEFAULT '',
  last_done_odo   REAL,
  notified_level  TEXT NOT NULL DEFAULT '',           -- last urgency pushed: '' | soon | due | overdue
  notified_at     TEXT NOT NULL DEFAULT '',
  external_id     TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX reminders_vehicle ON reminders(vehicle_id, status);

-- Planner: work you intend to do, as a small kanban. "Done" turns it into a record.
CREATE TABLE plans (
  id          INTEGER PRIMARY KEY,
  vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  title       TEXT NOT NULL,
  notes       TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL DEFAULT 'service',
  category    TEXT NOT NULL DEFAULT '',
  priority    TEXT NOT NULL DEFAULT 'normal',         -- low | normal | high | critical
  stage       TEXT NOT NULL DEFAULT 'backlog',        -- backlog | planned | in_progress | testing | done
  estimate    INTEGER NOT NULL DEFAULT 0,             -- minor units, vehicle currency
  target_date TEXT NOT NULL DEFAULT '',
  record_id   INTEGER,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  external_id TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX plans_vehicle ON plans(vehicle_id, stage, sort_order);

CREATE TABLE notes (
  id          INTEGER PRIMARY KEY,
  vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL DEFAULT '',
  pinned      INTEGER NOT NULL DEFAULT 0,
  external_id TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Documents: registration, insurance policy, invoices, photos of receipts…
-- stored as files under FILES_DIR/<vehicle>/ and optionally tied to one row.
CREATE TABLE documents (
  id          INTEGER PRIMARY KEY,
  vehicle_id  INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  user_id     INTEGER,
  name        TEXT NOT NULL,
  filename    TEXT NOT NULL,
  mime        TEXT NOT NULL DEFAULT '',
  bytes       INTEGER NOT NULL DEFAULT 0,
  kind        TEXT NOT NULL DEFAULT 'other',          -- registration | insurance | invoice | receipt | manual | inspection | photo | other
  expires_on  TEXT NOT NULL DEFAULT '',               -- insurance / registration validity → reminders can watch it
  link_kind   TEXT NOT NULL DEFAULT '',               -- record | fuel
  link_id     INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX documents_vehicle ON documents(vehicle_id);

CREATE TABLE activity (
  id         INTEGER PRIMARY KEY,
  vehicle_id INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
  user_id    INTEGER,
  action     TEXT NOT NULL,
  subject    TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX activity_vehicle ON activity(vehicle_id, id DESC);

-- Cached answers from Firefly, Dawarich, Pocket Travel… keyed by what was asked.
CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  fetched_at TEXT NOT NULL
);

CREATE TABLE fx_cache (
  base   TEXT NOT NULL,
  quote  TEXT NOT NULL,
  date   TEXT NOT NULL,
  rate   REAL NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (base, quote, date)
);
