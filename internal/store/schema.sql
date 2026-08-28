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
  -- The backup flags, which have to be stored or nobody with a synced passkey
  -- can ever sign in.
  --
  -- WebAuthn requires the relying party to check that BE has not changed since
  -- registration, and go-webauthn enforces it: a credential reconstructed with
  -- BE unset, matched against an assertion carrying BE=1, is refused. Every
  -- synced credential manager — Bitwarden, iCloud Keychain, Google Password
  -- Manager, 1Password — sets BE=1, so not storing this made registration
  -- succeed and sign-in fail for essentially every passkey a person would
  -- actually use. Found by the owner, on a phone, at the only moment it could
  -- have been (MUS-F-0029).
  --
  -- BE is immutable for the life of a credential; BS moves as the thing is
  -- backed up or restored, and is updated on use rather than compared.
  backup_eligible INTEGER NOT NULL DEFAULT 0,
  backup_state    INTEGER NOT NULL DEFAULT 0,
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

-- A WebAuthn ceremony in progress.
--
-- Registration and sign-in are two requests, and the challenge issued by the
-- first has to be remembered — unmodified — until the second. In the database
-- rather than in memory so a restart mid-ceremony fails cleanly rather than
-- mysteriously, and so nothing depends on one process holding the state.
--
-- Rows are short-lived and swept on use. The cookie that points at one carries
-- only the row's id: the challenge itself never goes to the browser except
-- inside the ceremony's own JSON, which is what the browser is meant to sign.
CREATE TABLE IF NOT EXISTS webauthn_ceremony (
  id      TEXT PRIMARY KEY,
  purpose TEXT NOT NULL CHECK (purpose IN ('register', 'signin')),
  data    TEXT NOT NULL,
  handle  BLOB,
  secret  TEXT,
  created TEXT NOT NULL,
  expires TEXT NOT NULL
);

-- An agent's credential.
--
-- A passkey is a person's: it needs a browser, a authenticator and a gesture.
-- An agent has none of those and still has to reach the mandated tool call, so
-- milestone 5c gives it a token it carries in a header. Hashed, like every
-- other secret here, and for the same reason.
--
-- Deliberately NOT an account. It has no email, cannot hold a passkey, cannot
-- sign in to a browser surface, and the guard consults it on exactly one path.
-- Folding it into `account` would have made every question about people also a
-- question about robots, and made a leaked token a way into the browser
-- surfaces rather than into the tool call it is scoped to.
CREATE TABLE IF NOT EXISTS agent_token (
  id         TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  -- What it is for, in the operator's words: "claude-code on whippy-vm". Shown
  -- in every listing, because a token nobody can identify is a token nobody
  -- dares revoke.
  label      TEXT NOT NULL,
  project    TEXT NOT NULL,
  role       TEXT NOT NULL CHECK (role IN ('owner', 'reader')),
  created    TEXT NOT NULL,
  created_by TEXT NOT NULL,
  -- Revocation is a timestamp rather than a deletion, so `mustur account
  -- tokens` can still say a token existed and when it stopped.
  revoked    TEXT,
  -- Optional, and empty means never (MUS-Q-0055). A token is configuration, so
  -- expiring by default would make an ordinary Tuesday into an outage; but a
  -- one-off agent, or one on somebody else's machine, is exactly the case for a
  -- token that stops on its own.
  expires    TEXT,
  last_used  TEXT
);

CREATE INDEX IF NOT EXISTS agent_token_by_project ON agent_token (project);

-- An image attached to a record, held privately.
--
-- The bytes never reach `mustur export`. records/ is committed and this
-- repository is public, so a screenshot written there would be a permanent
-- publication of whatever was on the owner's screen — agent output, record
-- text, an email address. What the export carries instead is an agent's
-- summary of what the image showed, written into the record's own fields, which
-- is the owner's decision: the description travels, the pixels do not.
--
-- Deliberately no filename column. A filename is the client's text and carries
-- more than it looks like — a path, a date, a device, the content itself — and
-- nothing here needs one. The identifier is the handle.
CREATE TABLE IF NOT EXISTS attachment (
  id         TEXT PRIMARY KEY,
  record_id  TEXT NOT NULL,
  -- Sniffed from the bytes, never taken from the request. A caller's
  -- Content-Type is a claim about a file it also chose.
  media_type TEXT NOT NULL,
  bytes      BLOB NOT NULL,
  size       INTEGER NOT NULL,
  sha256     TEXT NOT NULL,
  created    TEXT NOT NULL,
  created_by TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS attachment_by_record ON attachment (record_id);

-- A filing that was never meant to be kept.
--
-- Not a record: no identifier, never in the log, never exported, never counted.
-- The owner tested the picture upload twice and it cost two permanent
-- identifiers in the idea warehouse, which is the whole reason this exists — a
-- test filing should not advance a counter.
--
-- Everything here is dropped when the process starts, and swept by age while it
-- runs. Nothing should come to depend on a row in this table surviving.
CREATE TABLE IF NOT EXISTS scratch (
  id         TEXT PRIMARY KEY,
  text       TEXT NOT NULL,
  created    TEXT NOT NULL,
  created_by TEXT NOT NULL
);
