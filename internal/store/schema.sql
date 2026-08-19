-- Mustur's store. Two objects carry everything: an immutable log, and a
-- materialized view of the latest state derived from it.
--
-- The log is the record. `record_latest` is a cache that `mustur rebuild`
-- re-derives from the log, so a bug in the materializing trigger costs a
-- rebuild rather than a record.

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS record_event (
  seq        INTEGER PRIMARY KEY AUTOINCREMENT,
  record_id  TEXT    NOT NULL,
  kind       TEXT    NOT NULL,
  op         TEXT    NOT NULL CHECK (op IN ('create', 'amend')),
  at         TEXT    NOT NULL,
  actor      TEXT    NOT NULL,
  payload    TEXT    NOT NULL,
  written_at TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS record_event_by_record ON record_event (record_id, seq);

-- Insert-only, enforced here rather than by convention. The tool layer exposes
-- no update and no delete either; this is the half that survives someone
-- opening the file with the sqlite3 shell.
CREATE TRIGGER IF NOT EXISTS record_event_no_update
BEFORE UPDATE ON record_event
BEGIN
  SELECT RAISE(ABORT, 'record_event is insert-only: update refused');
END;

CREATE TRIGGER IF NOT EXISTS record_event_no_delete
BEFORE DELETE ON record_event
BEGIN
  SELECT RAISE(ABORT, 'record_event is insert-only: delete refused');
END;

-- The materialized latest. Not guarded, deliberately: the trigger below writes
-- it, and a guard that stopped a person would stop the trigger too. Its
-- authority comes from being re-derivable, not from being protected.
CREATE TABLE IF NOT EXISTS record_latest (
  record_id TEXT PRIMARY KEY,
  kind      TEXT    NOT NULL,
  seq       INTEGER NOT NULL,
  at        TEXT    NOT NULL,
  payload   TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS record_latest_by_kind ON record_latest (kind, record_id);

CREATE TRIGGER IF NOT EXISTS record_event_materialize
AFTER INSERT ON record_event
BEGIN
  INSERT INTO record_latest (record_id, kind, seq, at, payload)
  VALUES (NEW.record_id, NEW.kind, NEW.seq, NEW.at, NEW.payload)
  ON CONFLICT (record_id) DO UPDATE SET
    kind    = excluded.kind,
    seq     = excluded.seq,
    at      = excluded.at,
    payload = excluded.payload;
END;
