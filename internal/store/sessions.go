package store

// What Mustur last started, so a session the machine took can be offered back.
//
// A tmux session is processes, and a reboot ends processes. Nothing recovers a
// running one — what can be recovered is the conversation, which the CLI keeps
// on disk, and the three facts needed to open it again: which project, where it
// ran, and what it ran. tmux held all three and tmux does not survive a reboot,
// so before this they went with it and Mustur could not say a session had ever
// existed (MUS-Q-0083).
//
// **This is not a mirror of what is running.** MUS-D-0062 stands: listing
// sessions is still a live tmux query and nothing here is consulted for it. A
// row says what was launched, never that it is alive; the surface subtracts the
// live list from these and offers the difference.
//
// Not a record, for the reason a scratch filing is not one — operational state
// about one machine rather than a claim about the project. No identifier, never
// in the log, never exported.

import (
	"context"
	"errors"
	"strings"
	"time"
)

// A Remembered session is one Mustur started and has not been told ended.
type Remembered struct {
	Project string
	Dir     string
	Cmd     string
	// CLI is the conversation identifier the CLI reported through its
	// SessionStart hook. Empty means nobody told us one: a CLI with no such
	// hook, or a session that has not reached its first turn. Such a session
	// can still be started again in the same place — it just arrives empty.
	CLI string
	// Transcript is where the CLI keeps that conversation. Empty for the same
	// reasons CLI is; present and missing from the disk when the session was
	// started and never spoken to, which is a session that can come back but
	// not come back with anything.
	Transcript string
	Started    time.Time
}

// RememberSession records what Start launched, replacing anything held for that
// project.
//
// The conversation identifier is cleared rather than carried over. A new start
// is a new conversation until its hook says otherwise, and keeping the old one
// would offer a restore that resumed the wrong transcript.
func (s *Store) RememberSession(ctx context.Context, project, dir, cmd string) error {
	project = strings.TrimSpace(project)
	if project == "" {
		return errors.New("a session needs a project to be remembered under")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO session_started (project, dir, cmd, cli_session, transcript, started)
		 VALUES (?, ?, ?, '', '', ?)
		 ON CONFLICT (project) DO UPDATE SET
		   dir = excluded.dir, cmd = excluded.cmd,
		   cli_session = '', transcript = '', started = excluded.started`,
		project, dir, cmd, s.now().UTC().Format(time.RFC3339))
	return err
}

// NoteSessionCLI writes down the conversation identifier the CLI reported.
//
// It updates and never inserts. The hook fires inside a session Mustur started,
// so a row is already there; a payload naming a project with no row is a
// session this machine is not holding and inventing one would put a restore
// button in front of something Mustur never launched.
func (s *Store) NoteSessionCLI(ctx context.Context, project, cli, transcript string) error {
	project, cli = strings.TrimSpace(project), strings.TrimSpace(cli)
	if project == "" || cli == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE session_started SET cli_session = ?, transcript = ? WHERE project = ?`,
		cli, strings.TrimSpace(transcript), project)
	return err
}

// ForgetSession drops what was held for a project.
//
// Called when a session is stopped, and that is the whole of the difference
// this table encodes: a session the owner ended is finished and is not offered
// back; a session that went with the machine is still remembered.
func (s *Store) ForgetSession(ctx context.Context, project string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM session_started WHERE project = ?`, strings.TrimSpace(project))
	return err
}

// RememberedSessions lists them, most recently started first.
func (s *Store) RememberedSessions(ctx context.Context) ([]Remembered, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project, dir, cmd, cli_session, transcript, started
		   FROM session_started ORDER BY started DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Remembered
	for rows.Next() {
		var r Remembered
		var started string
		if err := rows.Scan(&r.Project, &r.Dir, &r.Cmd, &r.CLI, &r.Transcript, &started); err != nil {
			return nil, err
		}
		r.Started, _ = time.Parse(time.RFC3339, started)
		out = append(out, r)
	}
	return out, rows.Err()
}
