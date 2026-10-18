# Backup & restore

Everything lives in `DATA_DIR` (`/data` in the container):

- `pocket-garage.db` (+ `-wal` / `-shm` while running) — all vehicles, records, fuel, reminders, plans, notes, settings, users
- `files/<vehicle>/…` — uploaded documents and vehicle photos
- `backups/` — where the CLI writes snapshots

## Consistent copies

Never copy a live WAL database file. Use one of:

- **UI**: Settings → Backup & data → *Download database*
- **API**: `GET /api/v1/backup` with a bearer token
- **CLI**: `docker exec pocket-garage pocket-garage backup` → prints the path under `/data/backups/`
- **sqlite3**: `sqlite3 data/pocket-garage.db "VACUUM INTO 'snap.db'"`

Then back up `files/` with your usual tool (restic, Backrest, rsync). Exclude `pocket-garage.db*` from raw file backups and include the snapshot instead.

## Restore

1. Stop the container.
2. Replace `data/pocket-garage.db` with the snapshot (remove any stale `-wal` / `-shm`).
3. Restore `files/`.
4. Start the container. Migrations run forward automatically.

## Per-vehicle JSON

Each vehicle's Settings tab offers a JSON export with every row of that vehicle; it imports again under Settings → Import as a new vehicle (files are not included).
