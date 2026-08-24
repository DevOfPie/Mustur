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

-- Accounts, and everything that answers "who is asking".
--
-- Deliberately not records. A record is an immutable claim about the project
-- that gets exported to markdown and read by strangers; an account is mutable
-- operational state — sessions expire, passkeys are added and removed, a role
-- changes — and none of it belongs in a log whose whole value is that it never
-- changes. They share this file because they share a lifetime and a backup,
-- and because two SQLite files would be two things to keep consistent.
--
-- Nothing here is exported. `mustur export` renders records; if an account ever
-- appears in `records/`, that is a defect and not a feature.

CREATE TABLE IF NOT EXISTS account (
  id       TEXT PRIMARY KEY,
  email    TEXT NOT NULL UNIQUE,
  created  TEXT NOT NULL,
  disabled TEXT
);

-- A passkey. One account may hold several, which is the ordinary recovery when
-- a device is lost: the second device still works and can register a third.
CREATE TABLE IF NOT EXISTS credential (
  cred_id    BLOB PRIMARY KEY,
  account_id TEXT    NOT NULL REFERENCES account (id),
  public_key BLOB    NOT NULL,
  sign_count INTEGER NOT NULL DEFAULT 0,
  label      TEXT    NOT NULL,
  created    TEXT    NOT NULL,
  last_used  TEXT
);

CREATE INDEX IF NOT EXISTS credential_by_account ON credential (account_id);

-- A role is per project, not per install: a person may read one project and
-- have no business in another.
CREATE TABLE IF NOT EXISTS grant_role (
  account_id TEXT NOT NULL REFERENCES account (id),
  project    TEXT NOT NULL,
  role       TEXT NOT NULL CHECK (role IN ('owner', 'reader')),
  granted    TEXT NOT NULL,
  granted_by TEXT NOT NULL,
  PRIMARY KEY (account_id, project)
);

-- An invitation carries the role it will grant, so accepting it is not a second
-- decision. The token is stored hashed: this file is a backup away from being
-- somewhere else, and a live invite is a way in.
CREATE TABLE IF NOT EXISTS invite (
  token_hash TEXT PRIMARY KEY,
  email      TEXT NOT NULL,
  project    TEXT NOT NULL,
  role       TEXT NOT NULL CHECK (role IN ('owner', 'reader')),
  created    TEXT NOT NULL,
  created_by TEXT NOT NULL,
  expires    TEXT NOT NULL,
  used       TEXT
);

-- A signed-in browser. Hashed for the same reason an invite is.
CREATE TABLE IF NOT EXISTS auth_session (
  token_hash TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES account (id),
  created    TEXT NOT NULL,
  expires    TEXT NOT NULL,
  last_seen  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS auth_session_by_account ON auth_session (account_id);
