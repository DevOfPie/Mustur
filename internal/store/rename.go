package store

// Renaming records in place: the one exception to identifier permanence.
//
// Identifiers are permanent, and the log is insert-only, enforced by a trigger.
// This file breaks both, once, for the reason MUS-D-0192 records: the intake
// box's six jots were filed as IDW-F-0001 to IDW-F-0006 before the reserved
// prefix existed, and IDW is Idea Warehouse's. On MUS-Q-0153 the owner chose to
// rewrite them as _IB-F-0001 to _IB-F-0006 rather than leave six IDW stubs
// belonging to the intake box forever, because moving a system list out of the
// namespace projects use serves the permanence rule better than keeping it
// here would.
//
// **It is not a general rename facility.** MUS-D-0192 says the rename is a
// one-off and no precedent, and a reserved prefix beginning with an underscore
// is what keeps it from being needed again. It exists as code rather than as a
// sqlite3 session because the change has to reach every event in the log,
// every citation in every other record, and every table that holds an
// identifier, in one transaction, with a dry run a reader can check first.
//
// What it touches:
//
//   - record_event.record_id and record_event.payload, in every event, not
//     only the latest — the id itself, and every citation of it in another
//     record's title, body, data fields and refs, in both the identifier's own
//     spelling and the lower-cased one the export's anchors use;
//   - record_latest, re-derived from the rewritten log in the same transaction;
//   - attachment.record_id, so a picture follows its record;
//   - held_jot.destination and held_jot.text, and scratch.text, when those
//     tables exist in the store being renamed.
//
// Records named in keep are left exactly as written: they record the rename
// itself, and a decision that says "IDW-F-0001 became _IB-F-0001" is false the
// moment its own text is rewritten to say "_IB-F-0001 became _IB-F-0001".
//
// An old identifier is rewritten exactly where ident.Cited would read it as a
// citation, so what the rename leaves behind is what verify would count. Where
// it is spelled and not read — inside `XIDW-F-0001` or `IDW-F-00010` — it is
// left alone and reported as unmatched, and --apply refuses while any are
// reported unless told they are not citations (review of #107, finding 2).

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/record"
)

// Renaming is one identifier becoming another.
type Renaming struct {
	Old, New string
}

// RenameChange is one row a rename rewrites, or would.
type RenameChange struct {
	Table  string   // record_event, attachment, held_jot, scratch
	Row    string   // The row's key: an event's seq, an attachment's id.
	Record string   // The record the row belongs to, as it was before.
	Where  []string // What in the row changes: "id", "body", "data: Routed to", ...
}

// RenameOptions are everything a rename takes besides the map.
type RenameOptions struct {
	// Keep lists records whose text is left as written.
	Keep []string
	// AcceptUnmatched lets --apply proceed while events spell an old
	// identifier the rename does not read as one.
	AcceptUnmatched bool
	// Apply writes; without it the rename only reports.
	Apply bool
}

// RenameReport is what a rename changed, or would change on --apply.
type RenameReport struct {
	Changes []RenameChange
	// Kept lists events of kept records that cite an old identifier and were
	// deliberately left as written.
	Kept []RenameChange
	// Unmatched lists rows outside the kept records that still spell an old
	// identifier after the rewrite, because it stands inside something longer.
	Unmatched []RenameChange
	// Absent names the optional tables this store does not carry, so a
	// report of none rewritten is not mistaken for none found.
	Absent  []string
	Latest  int  // Rows in record_latest after the rebuild.
	Applied bool // False for a dry run, which writes nothing.
}

// Records counts the distinct records whose events change.
func (r RenameReport) Records() int {
	seen := map[string]bool{}
	for _, c := range r.Changes {
		if c.Table == "record_event" {
			seen[c.Record] = true
		}
	}
	return len(seen)
}

// Rename rewrites identifiers in place under MUS-D-0192. Without Apply it
// reads and works out every change inside the transaction, then returns
// before the guard is lifted or anything is written, so the dry run reports
// what --apply would write.
func (s *Store) Rename(ctx context.Context, renames []Renaming, opts RenameOptions) (RenameReport, error) {
	var report RenameReport
	keep, apply := opts.Keep, opts.Apply
	olds, err := checkRenames(renames)
	if err != nil {
		return report, err
	}
	kept := map[string]bool{}
	for _, k := range keep {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if !ident.Valid(k) {
			return report, fmt.Errorf("--keep %q is not an identifier", k)
		}
		if _, renamed := olds[k]; renamed {
			return report, fmt.Errorf("%s is both renamed and kept: a kept record's text is left as written, and its own identifier is in that text", k)
		}
		kept[k] = true
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()

	count := func(q string, args ...any) (int, error) {
		var n int
		err := tx.QueryRowContext(ctx, q, args...).Scan(&n)
		return n, err
	}
	for k := range kept {
		n, err := count(`SELECT count(*) FROM record_event WHERE record_id = ?`, k)
		if err != nil {
			return report, err
		}
		if n == 0 {
			return report, fmt.Errorf("--keep %s: no such record", k)
		}
	}
	// A retired identifier is never issued again by a rename (MUS-D-0197): the
	// records describing its retirement show it as plain text precisely so
	// nothing ever answers to it by accident, and a rename would be on purpose.
	latest, err := tx.QueryContext(ctx, `SELECT payload FROM record_latest`)
	if err != nil {
		return report, err
	}
	var all []record.Record
	for latest.Next() {
		var p string
		if err := latest.Scan(&p); err != nil {
			latest.Close()
			return report, err
		}
		r, err := record.UnmarshalPayload([]byte(p))
		if err != nil {
			latest.Close()
			return report, err
		}
		all = append(all, r)
	}
	latest.Close()
	if err := latest.Err(); err != nil {
		return report, err
	}
	retired := record.Retire(all).IDs
	for _, r := range renames {
		if retired[r.New] {
			return report, fmt.Errorf("%s is declared retired (a Renamed or Retired field names it): a rename never issues a retired identifier", r.New)
		}
	}

	for _, r := range renames {
		had, err := count(`SELECT count(*) FROM record_event WHERE record_id = ?`, r.Old)
		if err != nil {
			return report, err
		}
		has, err := count(`SELECT count(*) FROM record_event WHERE record_id = ?`, r.New)
		if err != nil {
			return report, err
		}
		attached, err := count(`SELECT count(*) FROM attachment WHERE record_id = ?`, r.New)
		if err != nil {
			return report, err
		}
		switch {
		case had == 0 && has > 0:
			return report, fmt.Errorf("%s does not exist and %s does: this rename has already been applied", r.Old, r.New)
		case had == 0:
			return report, fmt.Errorf("%s does not exist: nothing to rename", r.Old)
		case has > 0:
			return report, fmt.Errorf("%s already exists: a rename never writes over a record", r.New)
		case attached > 0:
			return report, fmt.Errorf("%s already has attachments, and no record: refusing to merge them", r.New)
		}
	}

	type eventRow struct {
		seq               int64
		id, payload       string
		newID, newPayload string
		where             []string
	}
	rows, err := tx.QueryContext(ctx, `SELECT seq, record_id, payload FROM record_event ORDER BY seq`)
	if err != nil {
		return report, err
	}
	var events []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.seq, &e.id, &e.payload); err != nil {
			rows.Close()
			return report, err
		}
		events = append(events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return report, err
	}

	var changed []eventRow
	for _, e := range events {
		newID := e.id
		if n, ok := olds[e.id]; ok {
			newID = n
		}
		if !spells(e.payload, renames) && newID == e.id {
			continue // Nothing in it spells an old identifier, in either case.
		}
		before, err := record.UnmarshalPayload([]byte(e.payload))
		if err != nil {
			return report, fmt.Errorf("event %d (%s): %w", e.seq, e.id, err)
		}
		where := differences(before, renames)
		if kept[e.id] {
			if len(where) > 0 {
				report.Kept = append(report.Kept, RenameChange{
					Table: "record_event", Row: fmt.Sprint(e.seq), Record: e.id, Where: where})
			}
			continue
		}
		if left := leftovers(rewriteRecord(before, renames), renames); len(left) > 0 {
			report.Unmatched = append(report.Unmatched, RenameChange{
				Table: "record_event", Row: fmt.Sprint(e.seq), Record: e.id, Where: left})
		}
		if len(where) == 0 && newID == e.id {
			continue // Spelled only inside something longer, and reported above.
		}
		// The payload is rewritten field by field and marshalled again, which
		// changes no byte the rename does not name only because every payload
		// is what MarshalPayload wrote. One that is not is refused rather than
		// restated in a form nobody wrote.
		again, err := before.MarshalPayload()
		if err != nil {
			return report, err
		}
		if string(again) != e.payload {
			return report, fmt.Errorf("event %d (%s): its payload does not round-trip, so rewriting it would change more than the rename; nothing written", e.seq, e.id)
		}
		after := rewriteRecord(before, renames)
		if after.ID != newID {
			return report, fmt.Errorf("event %d: row says %s and its payload says %s", e.seq, newID, after.ID)
		}
		payload, err := after.MarshalPayload()
		if err != nil {
			return report, err
		}
		e.newID, e.newPayload, e.where = newID, string(payload), where
		changed = append(changed, e)
		report.Changes = append(report.Changes, RenameChange{
			Table: "record_event", Row: fmt.Sprint(e.seq), Record: e.id, Where: where})
	}

	type textRow struct{ key, record, col, value string }
	var attachments []textRow
	for _, r := range renames {
		ar, err := tx.QueryContext(ctx, `SELECT id FROM attachment WHERE record_id = ? ORDER BY id`, r.Old)
		if err != nil {
			return report, err
		}
		for ar.Next() {
			var id string
			if err := ar.Scan(&id); err != nil {
				ar.Close()
				return report, err
			}
			attachments = append(attachments, textRow{key: id, record: r.Old, value: r.New})
			report.Changes = append(report.Changes, RenameChange{
				Table: "attachment", Row: id, Record: r.Old, Where: []string{"record_id"}})
		}
		ar.Close()
		if err := ar.Err(); err != nil {
			return report, err
		}
	}

	// Tables a store may or may not carry, depending on the binary that
	// created it. Each is read only when present.
	optional := []struct {
		table, key string
		cols       []string
	}{
		{"held_jot", "id", []string{"destination", "text"}},
		{"scratch", "id", []string{"text"}},
	}
	type optionalRow struct {
		table, key string
		values     map[string]string
	}
	var others []optionalRow
	for _, o := range optional {
		present, err := count(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, o.table)
		if err != nil {
			return report, err
		}
		if present == 0 {
			report.Absent = append(report.Absent, o.table)
			continue
		}
		// Table and column names are the literals above, never input.
		or, err := tx.QueryContext(ctx, "SELECT "+o.key+", "+strings.Join(o.cols, ", ")+" FROM "+o.table+" ORDER BY "+o.key)
		if err != nil {
			return report, err
		}
		for or.Next() {
			vals := make([]string, len(o.cols)+1)
			ptrs := make([]any, len(vals))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := or.Scan(ptrs...); err != nil {
				or.Close()
				return report, err
			}
			row := optionalRow{table: o.table, key: vals[0], values: map[string]string{}}
			var where, left []string
			for i, col := range o.cols {
				v := rewriteIDs(vals[i+1], renames)
				if v != vals[i+1] {
					row.values[col] = v
					where = append(where, col)
				}
				if spellsText(v, renames) {
					left = append(left, col)
				}
			}
			if len(left) > 0 {
				report.Unmatched = append(report.Unmatched, RenameChange{
					Table: o.table, Row: vals[0], Where: left})
			}
			if len(where) > 0 {
				others = append(others, row)
				report.Changes = append(report.Changes, RenameChange{
					Table: o.table, Row: vals[0], Where: where})
			}
		}
		or.Close()
		if err := or.Err(); err != nil {
			return report, err
		}
	}

	if !apply {
		n, err := count(`SELECT count(DISTINCT record_id) FROM record_event`)
		report.Latest = n
		return report, err
	}
	if len(report.Unmatched) > 0 && !opts.AcceptUnmatched {
		return report, fmt.Errorf("%d row(s) spell an old identifier inside something longer, which the rename leaves as written; "+
			"read them in the dry run, and pass --accept-unmatched if none is a citation", len(report.Unmatched))
	}

	// The insert-only trigger is lifted for this transaction and put back
	// from its own stored text, so what guards the log afterwards is exactly
	// what guarded it before.
	var guard string
	if err := tx.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'record_event_no_update'`).Scan(&guard); err != nil {
		if err == sql.ErrNoRows {
			return report, fmt.Errorf("record_event_no_update is missing: refusing to rewrite a log that is not guarded")
		}
		return report, err
	}
	if _, err := tx.ExecContext(ctx, `DROP TRIGGER record_event_no_update`); err != nil {
		return report, fmt.Errorf("lift the insert-only guard: %w", err)
	}
	for _, e := range changed {
		if _, err := tx.ExecContext(ctx,
			`UPDATE record_event SET record_id = ?, payload = ? WHERE seq = ?`, e.newID, e.newPayload, e.seq); err != nil {
			return report, fmt.Errorf("rewrite event %d: %w", e.seq, err)
		}
	}
	if _, err := tx.ExecContext(ctx, guard); err != nil {
		return report, fmt.Errorf("restore the insert-only guard: %w", err)
	}
	for _, a := range attachments {
		if _, err := tx.ExecContext(ctx, `UPDATE attachment SET record_id = ? WHERE id = ?`, a.value, a.key); err != nil {
			return report, fmt.Errorf("move attachment %s: %w", a.key, err)
		}
	}
	for _, o := range others {
		cols := make([]string, 0, len(o.values))
		for c := range o.values {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		for _, c := range cols {
			if _, err := tx.ExecContext(ctx, "UPDATE "+o.table+" SET "+c+" = ? WHERE id = ?", o.values[c], o.key); err != nil {
				return report, fmt.Errorf("rewrite %s %s: %w", o.table, o.key, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM record_latest`); err != nil {
		return report, fmt.Errorf("clear materialized latest: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO record_latest (record_id, kind, seq, at, payload)
		SELECT e.record_id, e.kind, e.seq, e.at, e.payload
		FROM record_event e
		WHERE e.seq = (SELECT max(seq) FROM record_event x WHERE x.record_id = e.record_id)`); err != nil {
		return report, fmt.Errorf("replay log: %w", err)
	}
	for _, r := range renames {
		left, err := count(`SELECT count(*) FROM record_event WHERE record_id = ?`, r.Old)
		if err != nil {
			return report, err
		}
		if left != 0 {
			return report, fmt.Errorf("%s still has %d event(s) after the rewrite; nothing written", r.Old, left)
		}
	}
	if report.Latest, err = count(`SELECT count(*) FROM record_latest`); err != nil {
		return report, err
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	report.Applied = true
	return report, nil
}

// checkRenames refuses a map that is malformed or collides with itself, and
// returns it keyed by old identifier.
func checkRenames(renames []Renaming) (map[string]string, error) {
	if len(renames) == 0 {
		return nil, fmt.Errorf("nothing to rename: give OLD=NEW")
	}
	olds := map[string]string{}
	news := map[string]string{}
	for _, r := range renames {
		o, err := ident.Parse(r.Old)
		if err != nil {
			return nil, err
		}
		n, err := ident.Parse(r.New)
		if err != nil {
			return nil, err
		}
		if o.Role != n.Role {
			return nil, fmt.Errorf("%s is a %s and %s would be a %s: a rename never changes what a record is",
				r.Old, o.Role.Name(), r.New, n.Role.Name())
		}
		if r.Old == r.New {
			return nil, fmt.Errorf("%s=%s renames nothing", r.Old, r.New)
		}
		if prev, dup := olds[r.Old]; dup {
			return nil, fmt.Errorf("%s is renamed twice, to %s and to %s", r.Old, prev, r.New)
		}
		if prev, dup := news[r.New]; dup {
			return nil, fmt.Errorf("%s and %s would both become %s", prev, r.Old, r.New)
		}
		olds[r.Old] = r.New
		news[r.New] = r.Old
	}
	for _, r := range renames {
		if from, chained := news[r.Old]; chained {
			return nil, fmt.Errorf("%s is renamed to %s and away again: a chain or a swap is not one rename", from, r.Old)
		}
	}
	return olds, nil
}

// rewriteIDs replaces each old identifier with its new one wherever it stands
// as a whole identifier, in its own spelling and in the lower-cased one the
// export's anchors use (`findings.md#idw-f-0004`).
func rewriteIDs(text string, renames []Renaming) string {
	for _, r := range renames {
		text = replaceWhole(text, r.Old, r.New)
		text = replaceWhole(text, strings.ToLower(r.Old), strings.ToLower(r.New))
	}
	return text
}

// spells is the cheap test before a payload is read: does the old identifier
// appear in it at all. It is a substring test on purpose — in the raw JSON a
// body line starting with the identifier follows an escaped newline, whose n would fail
// the whole-identifier test the decoded text passes.
func spells(payload string, renames []Renaming) bool {
	for _, r := range renames {
		if strings.Contains(payload, r.Old) || strings.Contains(payload, strings.ToLower(r.Old)) {
			return true
		}
	}
	return false
}

// spellsText reports whether decoded text still spells an old identifier, in
// either case, anywhere at all.
func spellsText(text string, renames []Renaming) bool {
	for _, r := range renames {
		if strings.Contains(text, r.Old) || strings.Contains(text, strings.ToLower(r.Old)) {
			return true
		}
	}
	return false
}

// replaceWhole replaces old where ident.Spans reads it as an identifier, so
// the rename rewrites exactly the citations verify counts: `_IDW-F-0001_` in
// italics is rewritten, and IDW-F-0001 inside XIDW-F-00012 is left alone.
//
// The lower-case spelling is an export anchor (`findings.md#idw-f-0004`),
// which Spans does not read, so it takes the same rule with letters of either
// case as the neighbours that make it part of something longer.
func replaceWhole(text, old, new string) string {
	if old == "" || !strings.Contains(text, old) {
		return text
	}
	var spans [][2]int
	if old == strings.ToUpper(old) {
		for _, sp := range ident.Spans(text) {
			if text[sp[0]:sp[1]] == old {
				spans = append(spans, sp)
			}
		}
	} else {
		for i := 0; ; {
			j := strings.Index(text[i:], old)
			if j < 0 {
				break
			}
			start, end := i+j, i+j+len(old)
			if (start == 0 || !anchorChar(text[start-1])) && (end == len(text) || !anchorChar(text[end])) {
				spans = append(spans, [2]int{start, end})
			}
			i = end
		}
	}
	if len(spans) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, sp := range spans {
		b.WriteString(text[last:sp[0]])
		b.WriteString(new)
		last = sp[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

func anchorChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
}

// leftovers names the fields of a rewritten record that still spell an old
// identifier: the ones the rename could not read as a citation.
func leftovers(r record.Record, renames []Renaming) []string {
	var where []string
	add := func(name, text string) {
		if spellsText(text, renames) {
			for _, w := range where {
				if w == name {
					return
				}
			}
			where = append(where, name)
		}
	}
	add("title", r.Title)
	add("body", r.Body)
	for _, f := range r.Refs {
		add("ref: "+f.Key, f.Key+" "+f.Value)
	}
	for _, f := range r.Data {
		add("data: "+f.Key, f.Key+" "+f.Value)
	}
	return where
}

// rewriteRecord renames every identifier a record's text carries.
func rewriteRecord(r record.Record, renames []Renaming) record.Record {
	out := r
	out.ID = rewriteIDs(r.ID, renames)
	out.Title = rewriteIDs(r.Title, renames)
	out.Body = rewriteIDs(r.Body, renames)
	out.Refs = rewriteFields(r.Refs, renames)
	out.Data = rewriteFields(r.Data, renames)
	return out
}

func rewriteFields(fs []record.Field, renames []Renaming) []record.Field {
	if fs == nil {
		return nil
	}
	out := make([]record.Field, len(fs))
	for i, f := range fs {
		out[i] = record.Field{Key: rewriteIDs(f.Key, renames), Value: rewriteIDs(f.Value, renames)}
	}
	return out
}

// differences names what in a record the rename touches.
func differences(r record.Record, renames []Renaming) []string {
	var where []string
	if rewriteIDs(r.ID, renames) != r.ID {
		where = append(where, "id")
	}
	if rewriteIDs(r.Title, renames) != r.Title {
		where = append(where, "title")
	}
	if rewriteIDs(r.Body, renames) != r.Body {
		where = append(where, "body")
	}
	for _, f := range r.Refs {
		if rewriteIDs(f.Key, renames) != f.Key || rewriteIDs(f.Value, renames) != f.Value {
			where = append(where, "ref: "+f.Key)
		}
	}
	for _, f := range r.Data {
		if rewriteIDs(f.Key, renames) != f.Key || rewriteIDs(f.Value, renames) != f.Value {
			where = append(where, "data: "+f.Key)
		}
	}
	// A repeated field — a question's options — is named once.
	out := where[:0]
	seen := map[string]bool{}
	for _, w := range where {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}
