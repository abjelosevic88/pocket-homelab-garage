# Configuration

Everything is read from environment variables; every value has a default so a bare `docker run -v ./data:/data` works. Integration values seed the settings on first start and can be changed later under **Settings → Integrations** (the UI values win afterwards).

| Variable | Default | What it does |
|---|---|---|
| `PORT` / `HOST` | `8080` / `0.0.0.0` | Listen address |
| `BASE_URL` | – | Public URL, used in notification links and the calendar feed hint |
| `DATA_DIR` | `./data` (`/data` in the image) | SQLite database, uploaded files and backups |
| `DB_PATH` / `FILES_DIR` | `$DATA_DIR/pocket-garage.db` / `$DATA_DIR/files` | Override locations |
| `TZ` | – | Container timezone (mount `/usr/share/zoneinfo` on Alpine-based hosts if dates look off) |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | `debug`, `warn`, `error`; `json` |
| `TRUST_PROXY` | `true` | Honour `X-Forwarded-For` |
| `METRICS_ENABLED` | `true` | Prometheus `/metrics` |
| `MAX_UPLOAD_BYTES` | `52428800` | Document / photo upload limit |
| `AUTH_MODE` | `local` | `local` (built-in accounts), `proxy` (trust `AUTH_PROXY_HEADER`), `none` (single user, LAN only) |
| `AUTH_PROXY_HEADER` / `AUTH_PROXY_EMAIL_HEADER` | `Remote-User` / `Remote-Email` | Headers for proxy mode |
| `SESSION_TTL` | `720h` | Cookie lifetime |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | – | Create the first admin on an empty database (otherwise the UI asks) |
| `DEFAULT_CURRENCY` | `EUR` | Home currency for totals and the default for new vehicles |
| `DISTANCE_UNIT` | `km` | `km` or `mi` for new vehicles |
| `VOLUME_UNIT` | `l` | `l`, `gal_us`, `gal_uk` |
| `FX_PROVIDER` | `auto` | `auto`, `frankfurter`, `erapi`, `none`. Euro pegs (BAM, BGN, XOF…) are built in. |
| `FIREFLY_URL` / `FIREFLY_TOKEN` | – | Firefly III base URL and personal access token |
| `DAWARICH_URL` / `DAWARICH_API_KEY` | – | Dawarich |
| `NOTIFY_URL` / `NOTIFY_FORMAT` / `NOTIFY_HOUR` | – / guessed / `8` | Apprise `…/notify/<key>`, ntfy topic URL or JSON webhook; `apprise`, `ntfy`, `json`; hour of the daily reminder check |
| `TRAVEL_URL` / `TRAVEL_TOKEN` / `TRAVEL_PUBLIC_URL` | – | Pocket Travel (public URL = what the browser opens for trip links) |
| `INVENTORY_URL` / `INVENTORY_TOKEN` | – | Pocket Inventory |
| `LUBELOGGER_URL` / `LUBELOGGER_USERNAME` / `LUBELOGGER_PASSWORD` | – | Import everything once on the first start (while the database has no vehicles). `LUBELOGGER_IMPORT_ON_START=false` disables that. |
| `USER_AGENT` | `PocketGarage/1.0 (…)` | Sent to the exchange-rate providers |
| `CORE_URL` / `CORE_REGISTRATION_KEY` | – | [Pocket Core](https://git.bjelke.org/abjelosevic88/pocket-homelab-core): register on start (the key is needed once; the minted app token lives in `$DATA_DIR/core.token`), publish `garage.fuel_logged` / `garage.service_logged` / `garage.odometer_updated`, accept the tokens Core hands to paired phones, and resolve Pocket Money (Firefly API) / Travel / Inventory through Core when their `*_URL` is empty |
| `CORE_APP_URL` | `BASE_URL` | How Core reaches this app for health checks (e.g. `http://host.docker.internal:8175`) |
| `DEMO_DATA` | `false` | Seed a sample car and motorcycle on an empty database |

## Auth modes

- **local** — accounts with email + password. The first account created becomes admin. Also used as Basic auth by the LubeLogger-compatible API.
- **proxy** — trust a forward-auth proxy (Authelia, Authentik, Tinyauth…). Users are created on first visit. Keep `/api/*` and `/cal/*` outside the forward-auth rule if the phone app or a calendar needs them.
- **none** — no login at all; one implicit admin. Only for a trusted LAN.

## CLI

```
pocket-garage                       start the server
pocket-garage backup [-o FILE]      consistent SQLite copy (VACUUM INTO)
pocket-garage reset-password [-email USER] [-password NEW]
pocket-garage version
```
