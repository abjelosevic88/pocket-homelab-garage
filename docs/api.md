# API

Base path `/api/v1`. JSON in and out. Authenticate with the session cookie (browser), `Authorization: Bearer <token>` or `X-API-Key: <token>` (tokens under Settings → API & apps). Money is in **minor units** (`1250` = 12.50) with a `currency` code; odometers are in the vehicle's own unit; fuel volume is litres (kWh for electric vehicles).

## Auth & account
| Method | Path | Notes |
|---|---|---|
| GET | `/auth/status` | `needs_setup`, `auth_mode`, `authenticated`, `user`, `version` |
| POST | `/auth/setup` · `/auth/login` · `/auth/logout` | `{email, password, name}` |
| GET/PUT | `/auth/me` | profile, password change |
| GET/POST/DELETE | `/tokens`, `/tokens/{id}` | personal access tokens |
| GET/POST/PUT/DELETE | `/users`, `/users/{id}` | admin |
| GET/PUT | `/settings` · GET `/settings/system` | the caller's own preferences and integrations over the box-wide title / exchange-rate switch (only an admin changes those); secrets masked, a masked value means "keep". `/settings/system` shows paths and row counts to admins only; `/backup` is admin-only |
| GET | `/meta` · `/currencies` · `/fx?from=&to=&date=` | lists the UI uses |
| GET | `/backup` | consistent SQLite copy |

## Vehicles
| Method | Path | Notes |
|---|---|---|
| GET | `/vehicles?status=active&q=&bare=1` | with `summary` unless `bare=1` |
| POST | `/vehicles` | `make`, `model`, `year`, `plate`, `fuel_type`, `odometer_unit`, `currency`, `initial_odometer`… |
| GET/PUT/DELETE | `/vehicles/{id}` | only sent fields change |
| GET | `/vehicles/{id}/overview` | vehicle + summary, reminders with urgency, recent fuel/records, months, pinned notes, trips, activity |
| PUT | `/vehicles/{id}/members` | `{members:[{user_id, role}]}` (owner) |
| POST/GET/DELETE | `/vehicles/{id}/image` | multipart `file` |
| GET | `/vehicles/{id}/suggestions` | categories, vendors, stations used so far |
| GET | `/vehicles/{id}/reports?months=24` | monthly stacks, yearly table, stations, fuel/odometer series, categories, Dawarich days |
| GET | `/vehicles/{id}/trips` | Pocket Travel trips with this vehicle's cost during each |

### Records (service / repair / upgrade / expense)
`GET /vehicles/{id}/records?kind=service,repair&q=&from=&to=&limit=` · `POST` · `GET/PUT/DELETE /records/{rid}` · `POST /records/{rid}/money` (book it in Pocket Money, `{money_account?, money_category?}`).

Fields: `kind`, `category`, `date`, `odometer` (null = none), `description`, `notes`, `vendor`, `cost`, `currency`, `fx_rate`, `parts: [{name, qty, cost, part_no, inventory_id, link}]`, `tags`. Response adds `cost_home` (vehicle currency), `source`, `money_tx_id`.

**Pocket Inventory.** A part with `inventory_id` (picked from Pocket Inventory) comes off the person's shelf when the record is created, an edit of `parts` takes or returns the difference, deleting the record puts it back (setting `inventory_take`, on by default). The movement's note is `Pocket Garage: <vehicle> · <description>`. Failures don't fail the save: the response carries `inventory_error`.

**Pocket Money.** Create and update (records, fill-ups, reminder *done* and plan *done*) take `money` (book it; absent = the person's `money_enabled` setting), `money_account` and `money_category`. The transaction goes into the Money of whoever logs it. A failed booking does not fail the save: the response carries `money_error`. Later edits to date, cost, currency, description, vendor (fuel: station, volume) update the transaction, a cost of 0 removes it, and deleting the record or fill-up deletes it. A transaction deleted in Money unlinks quietly.

### Fuel
`GET /vehicles/{id}/fuel?q=` · `POST` · `GET/PUT/DELETE /fuel/{fid}` · `POST /fuel/{fid}/money`.

Fields: `date`, `odometer`, `volume`, `cost` (or `price_per`), `currency`, `full_tank`, `missed`, `station`, `grade`, `soc_start`, `soc_end`, `lat`, `lng`, `notes`, `tags`. Response adds `distance`, `economy` (volume per 100 distance units over the stretch ending here), `price_per`, `cost_per_km`.

### Odometer
`GET /vehicles/{id}/odometer` → `{history, latest, latest_date, estimate_km, estimate}` (history unions standalone readings with fuel and record odometers) · `POST` `{date, reading, notes}` · `POST /odometer/confirm` (turn the Dawarich estimate into a reading) · `PUT/DELETE /odometer/{oid}` (standalone readings only).

### Reminders
`GET /reminders` (all vehicles) · `GET /vehicles/{id}/reminders?status=active` · `POST` · `PUT/DELETE /reminders/{rid}` · `POST /reminders/{rid}/done` `{date, odometer, log_record, cost, currency, vendor, notes}` · `POST /reminders/{rid}/snooze` `{days, km}`.

Fields: `title`, `notes`, `kind` (what "done" logs; `none` for a plain reminder), `category`, `due_date`, `due_odometer`, `interval_days`, `interval_km`, `soon_days`, `soon_km`. Response adds `urgency` (`ok|soon|due|overdue|done`), `days_left`, `km_left`.

### Planner, notes, documents
`/vehicles/{id}/plans` (+ `PUT /plans/reorder {stage, ids}`, `POST /plans/{pid}/done`), `/vehicles/{id}/notes`, `/vehicles/{id}/documents` (multipart `file` + `name`, `kind`, `expires_on`, `link_kind`, `link_id`; `GET /documents/{did}/file?download=1`).

### Integrations
| Method | Path | Notes |
|---|---|---|
| GET | `/money?vehicle=` | `{connected, via: core\|token, error, enabled, offer, core, url, accounts, default_account}`: whether forms offer the option and where it books |
| GET | `/money/categories?account=` | expense categories usable on that Money account |
| POST | `/vehicles/{id}/dawarich/sync` · GET `/dawarich/days?from=&to=` | |
| GET | `/inventory/search?q=` | the person's Pocket Inventory items `{items, url}` (through a Core link, or the token in Settings); `{items: [], error}` when not connected |
| POST | `/integrations/test` | `{which: money|dawarich|notify|travel|inventory|lubelogger, url, key, username, password, format}` |
| POST | `/notify/test` · `/notify/run?force=1` | |
| POST | `/import/lubelogger/vehicles` · `/import/lubelogger` | `{url, username, password, currency, vehicles:[ids]}` |
| POST | `/import/vehicle` | a JSON backup |

### Exports
`GET /vehicles/{id}/export/calendar.ics` · `/export/records.csv?kind=` · `/export/fuel.csv` · `/export/odometer.csv` · `/export/vehicle.json`.
Calendar subscription without a session: `GET /cal/{id}.ics?token=<api token>`.

## Dashboard (Homepage widget)

`GET /api/v1/dashboard` with a bearer token returns a flat object:

```json
{"vehicles":1,"vehicle_name":"2014 Skoda Fabia","odometer":261780,"odometer_unit":"km","economy":6.4,
 "cost_month":91.76,"cost_month_text":"91.76 BAM","cost_year":3067.91,"cost_year_text":"3067.91 BAM",
 "reminders_due":1,"reminders_soon":1,"next_reminder":"Registration (in 44 d)","next_reminder_urgency":"soon",
 "last_fuel_date":"2026-10-03","last_fuel_price":"115.64 BAM","currency":"BAM"}
```

Homepage `services.yaml`:

```yaml
- Pocket Garage:
    href: https://garage.example.com/
    icon: mdi-car-wrench
    widget:
      type: customapi
      url: http://pocket-garage:8080/api/v1/dashboard
      headers: { Authorization: "Bearer {{HOMEPAGE_VAR_POCKET_GARAGE_TOKEN}}" }
      mappings:
        - { field: odometer, label: Odometer, format: number }
        - { field: cost_month_text, label: This month }
        - { field: reminders_due, label: Due, format: number }
        - { field: next_reminder, label: Next }
```

## Pocket Core app links

Another Pocket app the person allowed in Pocket Core (Pocket Day, for the journal) calls this API for them with a five-minute `pcx_` token (see `docs/links.md` in pocket-homelab-core). The token acts as that person: it sees the vehicles they own or are a member of, exactly as with their own login, and opens only these routes; every other `/api/v1` route answers `403`, and the LubeLogger-compatible `/api/*` and the calendar feed never accept it.

| Scope | Routes |
|---|---|
| any linked app | GET `/auth/me` |
| `vehicles:read` | GET `/vehicles` (`?bare=1` skips the summaries), `/vehicles/{id}`, `/vehicles/{id}/fuel`, `/vehicles/{id}/records` (`?from=&to=`) |

## LubeLogger-compatible API

Mounted at `/api/…` (no `/v1`). Auth: **Basic** with a Pocket Garage email + password, or a bearer token. Bodies may be form-encoded or JSON; numbers are read with either decimal separator; dates come back as ISO. Send `culture-invariant: true` to receive JSON numbers instead of strings (the Pocket Homelab app does).

| Path | Maps to |
|---|---|
| `GET /api/vehicles` · `POST /api/vehicles/add` | vehicles |
| `GET /api/vehicle/info?vehicleId=` | summary (counts, costs, reminders, last odometer, `estimatedOdometer`) |
| `GET /api/vehicle/gasrecords?vehicleId=` + `/add`, `/update`, `/delete?id=` | fuel |
| `…/servicerecords`, `…/repairrecords`, `…/upgraderecords` | records of that kind |
| `…/taxrecords` | records with kind `expense` |
| `…/odometerrecords` | standalone readings (fuel/record odometers are not mirrored) |
| `GET /api/vehicle/reminders` · `/planrecords` · `/notes` | read-only |

A `[Pocket Homelab currency: XXX]` line in `notes` sets the row's currency and is echoed back on rows created through this API.

## Health & metrics

`GET /healthz`, `GET /readyz`, `GET /metrics` (Prometheus: build info, uptime, requests, rows per table).
