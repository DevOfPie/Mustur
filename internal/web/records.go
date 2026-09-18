package web

// Records — the reading surface, and the fourth tab.
//
// **A document, not a graph** (MUS-D-0040). Identifiers here are dense and
// cross-referential and the graph reading was real; what the owner chose is a
// thing to read, where a citation expands in place with no round trip and no
// new tab.
//
// **A record is a document; the index is a list** (MUS-D-0186, amending
// MUS-D-0040). The index used to be the document too — every record in the
// store rendered in full with every citation resolved, and the kind counts
// were the only navigation (MUS-F-0164). Measured for PR 101 on a copy of the
// live store on 2026-09-15, that was 11,413,213 bytes for 2,144 records; the
// 11,379,228 bytes in MUS-F-0164's evidence is the earlier measurement, taken
// against the live store by the MUS-F-0163 check. It is now
// one line a record, narrowed by project, kind and a search box and paged fifty
// at a time, and it never renders a body or resolves a citation. What reads as
// a document is `/records/{id}`, which is unchanged.
//
// Expansion is a `<details>` element, which is why this page carries no script:
// the browser already knows how to open and close a thing, and the decision
// queue reached the same answer for the same reason.
//
// **Routing lives inside it**, because repositories, machines and projects are
// record kinds like any other and a separate page would be a second surface to
// keep true. What makes routing different is that its rows are claims about
// this machine — so the surface **verifies rather than repeats**: a checkout
// that moved, or a contract file that is gone, reads as stale on the
// repository's own page, `/records/{ID}`. Index rows do not carry the badge:
// the approved row is five fields with no place for it, and the owner kept it
// to the record's page on MUS-Q-0149. That verification is the whole reason it
// is a surface and not a printed table, and it is the same posture the
// dispatcher contract takes, which verifies before entering rather than
// trusting a row.

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/ident"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// Records serves the records document.
type Records struct {
	counts countCache

	Store   *store.Store
	Project string
	// Home expands a leading ~ in a checkout path. Empty means the running
	// user's home, and it is injectable so the verification can be tested
	// without depending on whose machine the test runs on.
	Home string
	Now  func() time.Time
	// ShowSessions renders the Sessions tab. This surface shipped without it,
	// which quietly reduced MUS-D-0041's bar from four tabs to three.
	ShowSessions bool
	// ShowAccount renders the header link to the account surface, which is
	// served only when an origin is configured. Off means the link is absent
	// rather than dead (MUS-Q-0052).
	ShowAccount bool

	// Auth names who pressed Move or Keep, when accounts are served. Nil
	// falls back to what Cloudflare Access says, then to Actor.
	Auth  *Auth
	Actor string
	// ExportTo is the tree the store is rendered into after a move or a keep,
	// for the reason the intake box has one: whoever pressed it from a phone
	// cannot run `make export`. Empty means no export.
	ExportTo string
}

func (rr *Records) now() time.Time {
	if rr.Now != nil {
		return rr.Now()
	}
	return time.Now()
}

// Routes registers the surface.
func (rr *Records) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /records", rr.index)
	mux.HandleFunc("GET /records/{id}", rr.one)
	// The bytes of an attached image. On this surface deliberately: it is
	// behind the same guard as the record the image belongs to, and it is the
	// one place the picture is shown at all.
	mux.HandleFunc("GET /records/image/{id}", rr.image)
	// The Records badge's poll (MUS-D-0193), beside the Decisions one.
	mux.HandleFunc("GET /records/attention/count", rr.count)
	// The two ways out of needing attention (MUS-D-0193). Plain form posts,
	// so they work with script blocked, and an owner's alone.
	mux.HandleFunc("POST /records/{id}/move", rr.move)
	mux.HandleFunc("POST /records/{id}/keep", rr.keep)
}

// count answers the Records badge's poll: how many records need attention,
// cached exactly as the decisions count is. A number and never which records,
// for the same reason that one is.
func (rr *Records) count(w http.ResponseWriter, r *http.Request) {
	n := 0
	if rr.Store != nil {
		n = rr.counts.get(r.Context(), rr.Store, rr.now, intake.AttentionCount)
	}
	writeCount(w, n)
}

// kinds is the order the document presents, which is the order the export
// already uses. Routing last, because it is the part about this machine rather
// than about the work.
var kinds = []struct {
	Kind string
	One  string
	Many string
}{
	{"phase", "phase", "phases"},
	{"milestone", "milestone", "milestones"},
	{"work-unit", "work unit", "work units"},
	{"question", "question", "questions"},
	{"decision", "decision", "decisions"},
	{"finding", "finding", "findings"},
	{"investigation", "investigation", "investigations"},
	// Both spellings are written out rather than derived. Trimming an "s"
	// turned "repositories" into "1 repositorie" on the first render against
	// the real store, which is what a rule about English usually does.
	{"repository", "repository", "repositories"},
	{"machine", "machine", "machines"},
	{"project", "project", "projects"},
}

type citation struct {
	Key   string
	ID    string
	Kind  string
	Title string
	At    string
	// Known is false for an identifier the store does not hold, which is a
	// dangling citation and says so rather than rendering an empty box.
	Known bool
	// Plain marks a ref whose value is not an identifier at all — a path, a
	// name — which is shown as written rather than looked up and reported
	// missing.
	Plain bool
}

type recordView struct {
	ID    string
	Kind  string
	Title string
	At    string
	Body  template.HTML
	Data  []record.Field
	Refs  []citation
	// Cites are identifiers found in the prose, which is where most of this
	// tree's cross-references actually live.
	Cites []citation
	// Images are attachments held privately in the store. They are shown on
	// this surface, which is behind the guard, and are never exported: the
	// exported tree is committed to a public repository, so what travels is an
	// agent's reading of the picture rather than the picture.
	Images []imageView
	// State is the verification, for a routing record. Empty for everything
	// else: a decision cannot be stale in this sense.
	State string
	Stale bool

	// Attention is what the record names and was kept from, set only while it
	// needs attention (MUS-D-0193). CanMove says whether this viewer is shown
	// the buttons; a reader sees the banner without them.
	Attention []namedView
	CanMove   bool
	// MovedFrom and MovedTo are the success line after a move, shown on the
	// record the move filed.
	MovedFrom string
	MovedTo   string
	// Kept is the line after Keep.
	Kept bool
}

// A namedView is a destination a record names, by identifier and title.
type namedView struct {
	ID    string
	Title string
}

// An attentionRow is one line of the pinned section on the index.
type attentionRow struct {
	ID    string
	Title string
	Names []namedView
}

// named resolves a record's Names to titles. An identifier the store does not
// hold is shown as itself rather than dropped.
func named(r record.Record, by map[string]record.Record) []namedView {
	var out []namedView
	for _, id := range intake.Named(r) {
		title := id
		if d, ok := by[id]; ok && d.Title != "" {
			title = d.Title
		}
		out = append(out, namedView{ID: id, Title: title})
	}
	return out
}

// recordsPerPage is the owner's answer on MUS-Q-0140: fifty. At that size the
// 2,144 records of the 2026-09-15 measurement above are 43 pages.
const recordsPerPage = 50

// A rowView is one line of the index. Nothing in it needs the body or another
// record, which is the whole reason the index is cheap.
type rowView struct {
	ID      string
	Kind    string
	Project string
	Title   string
	At      string
	// Attention marks a row that also sits in the pinned section.
	Attention bool
}

// A pick is one option in a picker, carrying how many records choosing it
// holds.
type pick struct {
	Value    string
	Label    string
	Selected bool
}

type recordsIndex struct {
	// Attention is the pinned section above the filters: every record needing
	// attention, whatever the filters say, so narrowing the list can never
	// hide one (MUS-D-0193).
	Attention []attentionRow

	Projects []pick
	Kinds    []pick
	Q        string
	// Chosen is whether anything narrows the list, which is when Clear is
	// offered.
	Chosen  bool
	Summary string
	Rows    []rowView
	Page    int
	Pages   int
	// Beyond is a page past the last, which is shown empty rather than
	// refused: a bookmark outlives the records it was paging through.
	Beyond bool
	Pager  bool
	Newer  string
	Older  string
}

type recordsPage struct {
	// OpenQuestions is the bar's count. Every surface carries it, and bar.js
	// keeps it true after this render (MUS-F-0086).
	OpenQuestions int
	// Attention is the Records badge: how many records need attention
	// (MUS-D-0193). bar.js keeps it true after this render, as it does the
	// count above.
	Attention int

	Project      string
	ShowSessions bool
	ShowAccount  bool
	// Index is set on /records.
	Index *recordsIndex
	// One is set when a single record was asked for by identifier.
	One     *recordView
	Missing string
	Checked string
}

// An imageView is one attached picture, named by identifier rather than by
// filename — the filename was the sender's text and is not stored.
type imageView struct {
	ID   string
	Kind string
	Size string
}

// idInProse finds identifiers written in a record's text. It is ident.Cited,
// so the page offers exactly the citations the export check counts: a
// regular expression's \b read `_MUS-D-0001_` in italics as no citation at
// all, because the underscore is a word character.
func idInProse(text string) []string { return ident.Cited(text) }

func (rr *Records) load(ctx context.Context) (map[string]record.Record, []record.Record, error) {
	all, err := rr.Store.List(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	by := make(map[string]record.Record, len(all))
	for _, r := range all {
		by[r.ID] = r
	}
	return by, all, nil
}

// view builds what the page shows for one record, including its citations
// resolved so they can expand without another request.
func (rr *Records) view(r record.Record, by map[string]record.Record) recordView {
	// The body is rendered; the citations below are still read from the text
	// as written, so markdown cannot hide an identifier from them.
	// A retired identifier this record describes is text, never a link or a
	// citation to expand (MUS-D-0197): the store may later issue one of the
	// same spelling, and it would be somebody else's record.
	plain := retiredIn(r.ID, by)
	v := recordView{ID: r.ID, Kind: r.Kind, Title: r.Title, At: r.At, Body: markdownPlain(r.Body, plain), Data: r.Data}

	// A ref field may name several records — "Decided by: MUS-D-0002,
	// MUS-D-0008, MUS-D-0027" is one field and three citations. Looking the
	// whole value up as one identifier rendered eleven perfectly good
	// citations as dangling on the first run against the real store, which is
	// the sort of thing that reads as a finding about the tree until somebody
	// looks.
	for _, ref := range r.Refs {
		found := idInProse(ref.Value)
		if len(found) == 0 {
			// Not an identifier at all: some refs name a file or a person.
			v.Refs = append(v.Refs, citation{Key: ref.Key, ID: ref.Value, Plain: true})
			continue
		}
		for _, id := range found {
			if plain[id] {
				v.Refs = append(v.Refs, citation{Key: ref.Key, ID: id, Plain: true})
				continue
			}
			v.Refs = append(v.Refs, resolve(ref.Key, id, by))
		}
	}

	// Identifiers in the prose and in field values, which is where most
	// citations in this tree live. Deduplicated, and never the record itself.
	seen := map[string]bool{r.ID: true}
	for _, ref := range r.Refs {
		seen[ref.Value] = true
	}
	text := r.Body
	for _, f := range r.Data {
		text += " " + f.Value
	}
	for _, found := range idInProse(text) {
		if seen[found] || plain[found] {
			continue
		}
		seen[found] = true
		v.Cites = append(v.Cites, resolve("", found, by))
	}
	return v
}

// retiredIn is the set of retired identifiers the record with this
// identifier shows as plain text.
func retiredIn(id string, by map[string]record.Record) map[string]bool {
	all := make([]record.Record, 0, len(by))
	for _, r := range by {
		all = append(all, r)
	}
	return record.Retire(all).PlainIn(id)
}

func resolve(key, id string, by map[string]record.Record) citation {
	c := citation{Key: key, ID: id}
	if cited, ok := by[id]; ok {
		c.Known, c.Kind, c.Title, c.At = true, cited.Kind, cited.Title, cited.At
	}
	return c
}

// verify checks a routing record against the machine it describes.
//
// Only what can be checked cheaply and locally: that a checkout is where the
// row says, and that the contract file it names is in it. A row about another
// machine is not verifiable from here and says so rather than guessing.
func (rr *Records) verify(r record.Record) (string, bool) {
	if r.Kind != "repository" {
		return "", false
	}
	var path, contract string
	for _, f := range r.Data {
		switch {
		case strings.HasPrefix(f.Key, "Checkout on"):
			path = strings.TrimSpace(f.Value)
		case f.Key == "Contract":
			contract = strings.TrimSpace(f.Value)
		}
	}
	if path == "" {
		return "no checkout named", true
	}
	full := rr.expand(path)
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return "stale — nothing at " + path, true
	}
	if contract != "" {
		if _, err := os.Stat(filepath.Join(full, contract)); err != nil {
			return "stale — no " + contract, true
		}
	}
	return "there", false
}

func (rr *Records) expand(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home := rr.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// index is the list of records, narrowed and paged (MUS-D-0186, the plan
// approved on MUS-Q-0140).
//
// It filters what Store.List already returns rather than asking the store a
// narrower question, because the cost MUS-F-0164 measured was never the query:
// it was view(), which renders every body and resolves every citation of every
// record. The index calls neither.
//
// Unknown values are ignored rather than refused, so a stale bookmark still
// shows something.
func (rr *Records) index(w http.ResponseWriter, r *http.Request) {
	all, err := rr.Store.List(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))

	// A whole identifier is a request for that record, not a search for it.
	if id := strings.ToUpper(q); id != "" {
		for _, rec := range all {
			if rec.ID == id {
				http.Redirect(w, r, "/records/"+id, http.StatusSeeOther)
				return
			}
		}
	}

	names := projectNamesIn(all)
	perProject := map[string]int{}
	for _, rec := range all {
		perProject[prefixOf(rec.ID)]++
	}
	project := strings.ToUpper(strings.TrimSpace(query.Get("project")))
	if perProject[project] == 0 {
		project = ""
	}
	kind := strings.TrimSpace(query.Get("kind"))
	known := false
	for _, k := range kinds {
		known = known || k.Kind == kind
	}
	if !known {
		kind = ""
	}

	idx := &recordsIndex{Q: q, Chosen: project != "" || kind != "" || q != ""}

	// Before any filter is applied, so the section is the same on every page
	// of every narrowing. Newest first, like the list below it.
	by := make(map[string]record.Record, len(all))
	for _, rec := range all {
		by[rec.ID] = rec
	}
	for _, rec := range all {
		if intake.NeedsAttention(rec) {
			idx.Attention = append(idx.Attention, attentionRow{ID: rec.ID, Title: rec.Title, Names: named(rec, by)})
		}
	}
	sort.SliceStable(idx.Attention, func(i, j int) bool {
		a, b := by[idx.Attention[i].ID], by[idx.Attention[j].ID]
		if a.At != b.At {
			return a.At > b.At
		}
		return less(b.ID, a.ID)
	})

	prefixes := make([]string, 0, len(perProject))
	for p := range perProject {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, p := range prefixes {
		idx.Projects = append(idx.Projects, pick{
			Value: p, Selected: p == project,
			Label: names.name(p) + " · " + thousands(perProject[p]),
		})
	}

	// The kinds within the chosen project, so a picker never offers a choice
	// that holds nothing. The one chosen is kept even at nought, or the form
	// would quietly say something other than what the list is showing.
	perKind := map[string]int{}
	for _, rec := range all {
		if project == "" || prefixOf(rec.ID) == project {
			perKind[rec.Kind]++
		}
	}
	for _, k := range kinds {
		if perKind[k.Kind] == 0 && k.Kind != kind {
			continue
		}
		idx.Kinds = append(idx.Kinds, pick{
			Value: k.Kind, Selected: k.Kind == kind,
			Label: k.One + " · " + thousands(perKind[k.Kind]),
		})
	}

	var matched []record.Record
	for _, rec := range all {
		if project != "" && prefixOf(rec.ID) != project {
			continue
		}
		if kind != "" && rec.Kind != kind {
			continue
		}
		if q != "" && !searchMatches(rec, q) {
			continue
		}
		matched = append(matched, rec)
	}
	// Newest first, so what was filed today is at the top whatever project it
	// belongs to.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].At != matched[j].At {
			return matched[i].At > matched[j].At
		}
		return less(matched[j].ID, matched[i].ID)
	})

	idx.Pages = max(1, (len(matched)+recordsPerPage-1)/recordsPerPage)
	idx.Page = 1
	if n, err := strconv.Atoi(query.Get("page")); err == nil && n > 1 {
		idx.Page = n
	}
	from := (idx.Page - 1) * recordsPerPage
	if from < len(matched) {
		for _, rec := range matched[from:min(from+recordsPerPage, len(matched))] {
			idx.Rows = append(idx.Rows, rowView{
				ID: rec.ID, Kind: kindLabel(rec.Kind), Title: rec.Title, At: rec.At,
				Project:   names.title(prefixOf(rec.ID)),
				Attention: intake.NeedsAttention(rec),
			})
		}
	} else {
		idx.Beyond = idx.Page > 1
	}
	link := func(page int) string {
		v := url.Values{}
		for key, val := range map[string]string{"project": project, "kind": kind, "q": q} {
			if val != "" {
				v.Set(key, val)
			}
		}
		if page > 1 {
			v.Set("page", strconv.Itoa(page))
		}
		if len(v) == 0 {
			return "/records"
		}
		return "/records?" + v.Encode()
	}
	if idx.Page > 1 {
		// Past the end, Newer is the last page that has anything on it rather
		// than the one before a page that never existed.
		idx.Newer = link(min(idx.Page-1, idx.Pages))
	}
	if idx.Page < idx.Pages {
		idx.Older = link(idx.Page + 1)
	}
	idx.Pager = idx.Pages > 1 || idx.Beyond

	summary := thousands(len(matched)) + " records"
	if len(matched) == 1 {
		summary = "1 record"
	}
	if q != "" {
		if len(matched) == 1 {
			summary += " matches " + q
		} else {
			summary += " match " + q
		}
	}
	if project != "" {
		summary += " · " + names.title(project)
	}
	if kind != "" {
		summary += " · " + kindLabel(kind)
	}
	if !idx.Chosen {
		summary += " · newest first"
	}
	if len(matched) > recordsPerPage && len(idx.Rows) > 0 {
		summary += " · " + thousands(from+1) + "–" + thousands(from+len(idx.Rows))
	}
	idx.Summary = summary

	rr.render(w, r, recordsPage{Project: rr.Project, Index: idx, Checked: rr.now().Format("15:04")})
}

// prefixOf is the project an identifier belongs to, which is its prefix
// (MUS-D-0093).
func prefixOf(id string) string {
	p, _, _ := strings.Cut(id, "-")
	return p
}

func kindLabel(kind string) string {
	for _, k := range kinds {
		if k.Kind == kind {
			return k.One
		}
	}
	return kind
}

// searchMatches is the search box's rule, on MUS-Q-0140: a bare number matches
// the end of an identifier, in every project and kind, and anything else is
// words in the title. Never the body — a common word matches hundreds of
// LinkCtrl's decisions, and the owner chose titles.
func searchMatches(rec record.Record, q string) bool {
	if strings.Trim(q, "0123456789") == "" {
		return strings.HasSuffix(rec.ID, q)
	}
	return strings.Contains(strings.ToLower(rec.Title), strings.ToLower(q))
}

// thousands writes a count the way the page reads it: 2,133.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// one is the canonical URL for a single record, which is what "every record
// addressable by identifier" means: a bare identifier can be pasted into a bar.
func (rr *Records) one(w http.ResponseWriter, r *http.Request) {
	id := strings.ToUpper(strings.TrimSpace(r.PathValue("id")))
	by, _, err := rr.load(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rec, ok := by[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		rr.render(w, r, recordsPage{Project: rr.Project, Missing: id})
		return
	}
	v := rr.view(rec, by)
	v.State, v.Stale = rr.verify(rec)
	if intake.NeedsAttention(rec) {
		v.Attention = named(rec, by)
		v.CanMove = CanWrite(r)
	}
	// The line after a move, said only by the record the move filed: one that
	// does not correct the identifier in the address says nothing, so a link
	// cannot make any record claim a move it was not part of.
	if from := strings.ToUpper(r.URL.Query().Get("moved")); from != "" {
		for _, ref := range rec.Refs {
			if ref.Key == "Corrects" && ref.Value == from {
				v.MovedFrom = from
				v.MovedTo, _ = rec.Get("Routed to")
				for _, dest := range rec.Refs {
					if dest.Key == "Routed to" {
						if d, ok := by[dest.Value]; ok {
							v.MovedTo = d.Title
						}
					}
				}
			}
		}
	}
	if _, kept := rec.Get(intake.KeptField); kept && r.URL.Query().Get("kept") == "1" {
		v.Kept = true
	}
	// Only on a record's own page. The index lists hundreds and would fetch
	// every picture at once for a reader who asked for none of them.
	if shots, err := rr.Store.Attachments(r.Context(), id); err == nil {
		for _, a := range shots {
			v.Images = append(v.Images, imageView{
				ID: a.ID, Kind: a.MediaType,
				Size: fmt.Sprintf("%d KB", (a.Size+1023)/1024),
			})
		}
	}
	rr.render(w, r, recordsPage{Project: rr.Project, One: &v, Checked: rr.now().Format("15:04")})
}

// actor is who pressed Move or Keep: the signed-in account when accounts are
// served, else what Cloudflare Access says at the edge, else the configured
// actor — the order the rest of this package already trusts.
func (rr *Records) actor(r *http.Request) string {
	if rr.Auth != nil {
		if acct, ok := rr.Auth.Whoever(r.Context(), r); ok && acct.Email != "" {
			return acct.Email
		}
	}
	if who := r.Header.Get("Cf-Access-Authenticated-User-Email"); who != "" {
		return who
	}
	if rr.Actor != "" {
		return rr.Actor
	}
	return "owner"
}

// attending is the shared front half of move and keep: an owner, from this
// site, about a record that needs attention. It writes the refusal itself and
// reports whether to go on.
//
// The guard already refuses a reader's POST; this refuses it again so the
// rule holds on a server run without the guard's role in front of it, and
// says which rule it was.
func (rr *Records) attending(w http.ResponseWriter, r *http.Request) (record.Record, bool) {
	if !CanWrite(r) {
		http.Error(w, "only an owner can move or keep a record", http.StatusForbidden)
		return record.Record{}, false
	}
	// The same check the composer and the account screens make: a browser
	// sends Origin on a form post, and one from another site is refused.
	if !sameOrigin(r) {
		http.Error(w, "cross-origin post refused", http.StatusForbidden)
		return record.Record{}, false
	}
	id := strings.ToUpper(strings.TrimSpace(r.PathValue("id")))
	rec, err := rr.Store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "no record called "+id, http.StatusNotFound)
		return record.Record{}, false
	}
	return rec, true
}

// refuseUnlessAttending is the 409 for a record that proposes nothing. Called
// after the second-press checks, so a press that already happened is sent to
// its result rather than told there is nothing to do.
func refuseUnlessAttending(w http.ResponseWriter, rec record.Record) bool {
	if !intake.NeedsAttention(rec) {
		http.Error(w, rec.ID+" proposes no move, so there is nothing to move or keep", http.StatusConflict)
		return true
	}
	return false
}

// moved sends the browser to the record a move filed, with the success line.
// The same answer for the press that moved it and for a second press that
// arrived after: both asked for the move, and the move happened.
func moved(w http.ResponseWriter, r *http.Request, from, to string) {
	http.Redirect(w, r, "/records/"+to+"?moved="+url.QueryEscape(from), http.StatusSeeOther)
}

// move performs the move a record proposes: exactly `mustur reroute <ID> --to
// <what it names>`, through the same function, with the owner as actor.
func (rr *Records) move(w http.ResponseWriter, r *http.Request) {
	rec, ok := rr.attending(w, r)
	if !ok {
		return
	}
	if by := intake.CorrectedBy(rec); by != "" {
		moved(w, r, rec.ID, by)
		return
	}
	if refuseUnlessAttending(w, rec) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "that form did not arrive intact", http.StatusBadRequest)
		return
	}
	names := intake.Named(rec)
	to := strings.TrimSpace(r.FormValue("to"))
	if to == "" && len(names) == 1 {
		to = names[0]
	}
	// Only somewhere the record names. Anywhere else is a correction, which
	// the command makes and this button does not.
	proposed := false
	for _, id := range names {
		proposed = proposed || id == to
	}
	if !proposed {
		http.Error(w, rec.ID+" does not name "+to+"; it names "+strings.Join(names, ", "), http.StatusBadRequest)
		return
	}
	who := rr.actor(r)
	title := to
	if d, err := rr.Store.Get(r.Context(), to); err == nil && d.Title != "" {
		title = d.Title + " (" + to + ")"
	}
	done, err := intake.Reroute(r.Context(), rr.Store, intake.RerouteRequest{
		Project: rr.Project, ID: rec.ID, To: to, Actor: who, Now: rr.now(),
		Why: "it named " + title + ", which takes jots only when a move is confirmed, and " + who + " confirmed it",
	})
	var already *intake.AlreadyCorrected
	if errors.As(err, &already) {
		// The other press of a double press won the race.
		moved(w, r, rec.ID, already.By)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	rr.counts.forget()
	rr.export(r.Context())
	moved(w, r, rec.ID, done.Fresh.ID)
}

// keep declines the move and leaves the record where it is, saying who and
// when.
func (rr *Records) keep(w http.ResponseWriter, r *http.Request) {
	rec, ok := rr.attending(w, r)
	if !ok {
		return
	}
	back := "/records/" + rec.ID + "?kept=1"
	// A second press on a kept record goes back to it without writing.
	if _, kept := rec.Get(intake.KeptField); kept {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if refuseUnlessAttending(w, rec) {
		return
	}
	_, err := intake.Keep(r.Context(), rr.Store, rec.ID, rr.actor(r), rr.now())
	if errors.Is(err, intake.ErrAlreadyKept) {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	rr.counts.forget()
	rr.export(r.Context())
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// export renders the store into the configured tree. The change is already in
// the store, so a failed export has lost nothing and the press still
// succeeded; it is logged rather than swallowed, because an exported tree
// quietly behind the store is the drift nothing else here would notice.
func (rr *Records) export(ctx context.Context) {
	if rr.ExportTo == "" {
		return
	}
	all, err := rr.Store.List(ctx, "")
	if err == nil {
		err = export.Write(rr.ExportTo, all)
	}
	if err != nil {
		log.Printf("records: exporting to %s after a move or keep failed: %v", rr.ExportTo, err)
	}
}

// less orders identifiers by role then serial, which is how every listing here
// sorts.
func less(a, b string) bool {
	ia, erra := ident.Parse(a)
	ib, errb := ident.Parse(b)
	if erra != nil || errb != nil {
		return a < b
	}
	return ident.Less(ia, ib)
}

// image serves one attached picture.
//
// The stored media type is used rather than a sniff of the response, and nosniff
// plus a closed content policy say so to the browser: an image this refuses to
// call a document must not be talked into behaving like one. Private caching
// only — this is somebody's screenshot, not a static asset.
func (rr *Records) image(w http.ResponseWriter, r *http.Request) {
	a, data, err := rr.Store.Image(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "no such image", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", a.MediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	_, _ = w.Write(data)
}

func (rr *Records) render(w http.ResponseWriter, r *http.Request, p recordsPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// Set here rather than at three call sites, because a page built without
	// them renders a bar missing a tab and says nothing about it.
	p.ShowSessions = rr.ShowSessions && CanWrite(r)
	p.ShowAccount = rr.ShowAccount
	if rr.Store != nil {
		p.OpenQuestions = OpenCount(r.Context(), rr.Store)
		p.Attention = intake.AttentionCount(r.Context(), rr.Store)
	}
	if err := recordsTmpl.Execute(w, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var recordsTmpl = template.Must(template.New("records").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mustur — records</title>
<style>
  :root { color-scheme: light dark; --edge: #8884; --accent: #6a8fd8;
          --accent-soft: #6a8fd820; }
  body { font: 17px/1.5 system-ui, sans-serif; margin: 0; padding: 0 0 1rem;
         max-width: 46rem; margin-inline: auto; }
  header { display: flex; align-items: baseline; gap: .6rem; padding: .75rem 1rem;
           border-bottom: 1.4px solid var(--edge); }
  header strong { font-size: 1rem; }
  /* MUS-Q-0052: the account surface is reached from here rather than from
     a fifth tab, so MUS-D-0041's four stand. Rendered only when the server
     actually serves it — a link that goes nowhere is the failure the bar
     itself was written to avoid. */
  .acct { font-size: .82em; opacity: .6; text-decoration: none;
          color: inherit; margin-left: .6rem; }
  header .who { margin-left: auto; opacity: .6; font-size: .82em; }
  /* The index narrows (MUS-D-0186, amending MUS-D-0040). A plain GET form, so
     the address is the filter and it works with script blocked. Every child
     may be narrower than its content, or a long project name in a picker
     widens the page and takes the fixed bar with it (MUS-F-0033). */
  .narrow { display: flex; flex-wrap: wrap; gap: .5rem; padding: .6rem 1rem;
            border-bottom: 1.4px solid var(--edge); }
  .narrow .pick { display: flex; gap: .5rem; flex: 1 1 22rem; min-width: 0; }
  .narrow .find { display: flex; gap: .5rem; flex: 1 1 16rem; min-width: 0; }
  .narrow select, .narrow input { flex: 1; min-width: 0; }
  .narrow select, .narrow input, .narrow button {
    font: inherit; font-size: .9em; padding: .35rem .5rem; color: inherit;
    background: Canvas; border: 1px solid var(--edge); border-radius: .4rem; }
  .narrow button { flex: none; border-color: var(--accent);
                   background: var(--accent-soft); }
  .tally { margin: 0; padding: .5rem 1rem; font-size: .82em; }
  .tally span { opacity: .7; }
  .tally a { color: inherit; margin-left: .5rem; }
  /* One line a record on a wide screen, the title taking what is left and
     cut short with an ellipsis rather than wrapping. */
  .rows { list-style: none; margin: 0; padding: 0;
          border-top: 1px solid var(--edge); }
  .row { display: flex; align-items: baseline; gap: .7rem; padding: .45rem 1rem;
         border-bottom: 1px solid var(--edge); text-decoration: none;
         color: inherit; white-space: nowrap; }
  .row:hover { background: var(--accent-soft); }
  .row .id { flex: none; width: 6.5rem; font-size: .8em;
             font-variant-numeric: tabular-nums; }
  .row .kind, .row .proj, .row .at { flex: none; font-size: .78em; opacity: .65; }
  .row .kind { width: 6.5rem; }
  .row .proj { width: 6rem; overflow: hidden; text-overflow: ellipsis; }
  .row .t { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis;
            font-size: .93em; }
  /* On a phone a row takes two lines so the title is never cut short, as the
     plan draws it. */
  @media (max-width: 40rem) {
    .row { flex-wrap: wrap; row-gap: .1rem; white-space: normal; }
    .row .id, .row .kind, .row .proj { width: auto; }
    .row .t { flex-basis: 100%; order: 1; overflow-wrap: anywhere; }
    .row .at { display: none; }
  }
  .pager { display: flex; align-items: baseline; justify-content: space-between;
           gap: .5rem; padding: .7rem 1rem; font-size: .9em; }
  .pager span { opacity: .4; }
  .pager small { opacity: .65; }
  article { padding: .7rem 1rem; border-bottom: 1px solid var(--edge); }
  article .line { display: flex; align-items: baseline; gap: .5rem;
                  flex-wrap: wrap; font-size: .8em; opacity: .65; }
  article .line a { color: inherit; }
  /* The same rule as .fields .v below, on the text that is not a field value.
     MUS-F-0033 gave field values somewhere to break and left titles, bodies and
     summaries alone, which held until a record body carried a pasted terminal
     box: an unbroken run of box-drawing characters has no break opportunity, so
     it set the width of the document and the fixed tab bar went off the bottom
     of the screen with it (MUS-F-0131, measured at 390px: 625px of document
     before, 390 after). h3 and summary are the same risk class and are covered
     here rather than waiting for the record that proves it a third time.
     The cost, and it is real: a body that is deliberate ASCII art wraps mid
     frame and reads as broken. A scroll container of its own is what would keep
     it, and that is a design decision rather than this fix. */
  article h3 { font-size: 1.1rem; font-weight: 600; margin: .15rem 0 .3rem;
               overflow-wrap: anywhere; }
  article p { margin: .3rem 0; font-size: .93em; overflow-wrap: anywhere; }
  .fields { font-size: .86em; margin: .4rem 0 0; }
  /* A field row wraps rather than widening the page. A long value — a work
     unit's "Done means" runs to paragraphs — used to push the row past the
     screen, and on a phone a page wider than the viewport takes the fixed tab
     bar with it, so only three of the four tabs could be reached (MUS-F-0033).
     min-width:0 is the declaration that lets a flex child be narrower than its
     content; without it the two below do nothing. */
  .fields div { display: flex; gap: .5rem; padding: .1rem 0; flex-wrap: wrap; }
  .fields .k { opacity: .55; flex: 0 0 9rem; }
  .fields .v { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  details { margin: .25rem 0; font-size: .88em; }
  summary { cursor: pointer; opacity: .8; overflow-wrap: anywhere; }
  details .inner { margin: .3rem 0 .5rem 1rem; padding-left: .6rem;
                   border-left: 2px solid var(--edge); }
  /* Attached pictures. Shown here and nowhere else: this surface is behind
     whatever gate is in front of the records, and the exported tree — which is
     committed to a public repository — carries an agent's reading of the image
     rather than the image. */
  .shots { display: flex; flex-direction: column; gap: .5rem; margin: .6rem 0; }
  .shots figure { margin: 0; }
  .shots img { max-width: 100%; height: auto; display: block;
               border: 1px solid var(--edge); border-radius: .4rem; }
  .shots figcaption { font-size: .78em; opacity: .6; margin-top: .2rem; }
  .badge.stale { border-color: var(--warn); opacity: 1; }
  /* Needs attention (MUS-D-0193), in the warn tone the Records badge wears.
     The pinned section sits above the filters and ignores them, so narrowing
     the list can never hide a record waiting on somebody. */
  .attn { margin: .6rem 1rem 0; border: 1.4px solid var(--warn);
          border-radius: .5rem; overflow: hidden; }
  .attn h2 { margin: 0; padding: .45rem .8rem; font-size: .9rem;
             background: var(--warn-soft); }
  .attn .rows { border-top: 1px solid var(--warn); }
  .attn .row:last-child { border-bottom: 0; }
  .pill { flex: none; font-size: .75em; border: 1px solid var(--warn);
          border-radius: 999px; padding: 0 .45rem; white-space: nowrap; }
  /* In the row's own left padding, so a marked row's identifier stays in the
     column every other row's is in. */
  .rows .row { position: relative; }
  .row .dot { position: absolute; left: .3rem; top: calc(.45rem + .45em);
              width: .45rem; height: .45rem; border-radius: 50%;
              background: var(--warn); }
  .banner { border: 1.4px solid var(--warn); background: var(--warn-soft);
            border-radius: .5rem; padding: .6rem .8rem; margin: 0 0 .6rem; }
  .banner p { margin: 0; }
  .banner .acts { display: flex; flex-wrap: wrap; gap: .5rem; margin-top: .6rem; }
  .banner form { margin: 0; }
  .banner button { font: inherit; font-size: .92em; padding: .4rem .8rem;
                   color: inherit; background: Canvas; cursor: pointer;
                   border: 1px solid var(--edge); border-radius: .4rem; }
  .banner button.primary { border-color: var(--accent); background: var(--accent-soft);
                           font-weight: 600; }
  /* On a phone the two buttons stack full width, so neither is a small
     target beside the other. */
  @media (max-width: 40rem) {
    .banner .acts { flex-direction: column; }
    .banner form, .banner button { width: 100%; }
  }
  .done { border: 1px solid var(--edge); border-radius: .5rem;
          padding: .5rem .8rem; margin: 0 0 .6rem; font-size: .93em; }
  .none { opacity: .6; padding: 2rem 1rem; text-align: center; }
` + markdownCSS + citesCSS + shellCSS + `
</style>
</head>
<body>
<header><strong>{{if .One}}{{.One.ID}}{{else}}Records{{end}}</strong>
  {{if .One}}<a href="/records" style="font-size:.82em">all records</a>{{end}}
  <span class="who">{{.Project}}</span>{{if .ShowAccount}}<a class="acct" href="/account">Account</a>{{end}}</header>

{{if .Missing}}
<p class="none">No record called {{.Missing}}.<br>
<small>An identifier that is not here is either a typo or a citation to something never written.</small></p>
{{else if .One}}
{{template "record" .One}}
{{else if .Index}}{{with .Index}}
{{if .Attention}}<section class="attn" aria-label="Needs attention">
<h2>Needs attention · {{len .Attention}}</h2>
<ol class="rows">
{{range .Attention}}<li><a class="row" href="/records/{{.ID}}"><span class="id">{{.ID}}</span><span class="t">{{.Title}}</span>{{range .Names}}<span class="pill">names {{.Title}} — move?</span>{{end}}</a></li>
{{end}}</ol>
</section>{{end}}
<form class="narrow" method="get" action="/records" role="search">
  <div class="pick">
    <select name="project" aria-label="Project">
      <option value="">All projects</option>
      {{range .Projects}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
    </select>
    <select name="kind" aria-label="Kind">
      <option value="">All kinds</option>
      {{range .Kinds}}<option value="{{.Value}}"{{if .Selected}} selected{{end}}>{{.Label}}</option>{{end}}
    </select>
  </div>
  <div class="find">
    <input type="search" name="q" value="{{.Q}}" placeholder="Identifier or words" aria-label="Identifier or words" autocapitalize="characters" autocomplete="off" spellcheck="false">
    <button type="submit">Show</button>
  </div>
</form>
<p class="tally"><span>{{.Summary}}</span>{{if .Chosen}}<a href="/records">Clear</a>{{end}}</p>
{{if .Rows}}<ol class="rows">
{{range .Rows}}<li><a class="row" href="/records/{{.ID}}">{{if .Attention}}<span class="dot" title="Needs attention" aria-label="Needs attention"></span>{{end}}<span class="id">{{.ID}}</span><span class="kind">{{.Kind}}</span><span class="proj">{{.Project}}</span><span class="t">{{.Title}}</span><span class="at">{{.At}}</span></a></li>
{{end}}</ol>
{{else if .Beyond}}<p class="none">Nothing on page {{.Page}}. The list ends at page {{.Pages}}.</p>
{{else}}<p class="none">No records match.</p>
{{end}}
{{if .Pager}}<div class="pager">
  {{if .Newer}}<a rel="prev" href="{{.Newer}}">Newer</a>{{else}}<span>Newer</span>{{end}}
  <small>{{if not .Beyond}}Page {{.Page}} of {{.Pages}}{{end}}</small>
  {{if .Older}}<a rel="next" href="{{.Older}}">Older</a>{{else}}<span>Older</span>{{end}}
</div>{{end}}
{{end}}{{end}}

<nav>
  {{if .ShowSessions}}<a href="/sessions" aria-label="Sessions"><i class="ic ic-sess"></i><span>Sessions</span></a>{{end}}
  <a href="/questions" aria-label="Decisions"><i class="ic ic-dec">?</i><span>Decisions</span>{{if .OpenQuestions}}<em class="cnt">{{.OpenQuestions}}</em>{{end}}</a>
  <a href="/intake" aria-label="Intake"><i class="ic ic-in"><b></b></i><span>Intake</span></a>
  <a href="/records" class="here" aria-label="Records"><i class="ic ic-rec"></i><span>Records</span>{{if .Attention}}<em class="cnt att">{{.Attention}}</em>{{end}}</a>
  {{if .ShowAccount}}<a class="me" href="/account" title="Account" aria-label="Account"><i class="ic ic-acc"></i></a>{{end}}
</nav>
<script src="/assets/bar.js"></script>
</body>
</html>

{{define "record"}}
<article id="{{.ID}}">
  {{if .MovedFrom}}<p class="done" role="status">Moved to {{.MovedTo}}. {{.MovedFrom}} is kept and points here.</p>{{end}}
  {{if .Kept}}<p class="done" role="status">Kept in the intake box.</p>{{end}}
  {{if .Attention}}<div class="banner" role="note">
    <p><strong>Needs attention.</strong> This jot names {{range $i, $n := .Attention}}{{if $i}} and {{end}}{{$n.Title}}{{end}}, which {{if eq (len .Attention) 1}}takes{{else}}take{{end}} a jot only when a move is confirmed.</p>
    {{if .CanMove}}<div class="acts">
      {{range .Attention}}<form method="post" action="/records/{{$.ID}}/move"><input type="hidden" name="to" value="{{.ID}}"><button class="primary" type="submit">Move to {{.Title}}</button></form>
      {{end}}<form method="post" action="/records/{{.ID}}/keep"><button type="submit">Keep in intake box</button></form>
    </div>{{end}}
  </div>{{end}}
  <div class="line">
    <a href="/records/{{.ID}}">{{.ID}}</a>
    <span>{{.Kind}}</span>
    <span>{{.At}}</span>
    {{if .State}}<span class="badge{{if .Stale}} stale{{end}}">{{.State}}</span>{{end}}
  </div>
  <h3>{{.Title}}</h3>
  {{if .Body}}<div class="md">{{.Body}}</div>{{end}}
  {{if .Data}}<div class="fields">
    {{range .Data}}<div><span class="k">{{.Key}}</span><span class="v">{{.Value}}</span></div>{{end}}
  </div>{{end}}
  {{if .Images}}<div class="shots">
    {{range .Images}}<figure>
      <a href="/records/image/{{.ID}}"><img src="/records/image/{{.ID}}" alt="attached to {{$.ID}}" loading="lazy"></a>
      <figcaption>{{.Kind}} · {{.Size}} · held privately, never exported</figcaption>
    </figure>{{end}}
  </div>{{end}}
  {{range .Refs}}{{if .Plain}}<div class="fields"><div><span class="k">{{.Key}}</span><span class="v">{{.ID}}</span></div></div>{{else}}<details>
    <summary>{{if .Key}}{{.Key}}: {{end}}{{.ID}}{{if .Known}} · {{.Kind}}{{end}}</summary>
    <div class="inner">{{if .Known}}<strong>{{.Title}}</strong><br><small>{{.At}} · <a href="/records/{{.ID}}">open on its own</a></small>{{else}}Nothing in the store has this identifier.{{end}}</div>
  </details>{{end}}{{end}}
  {{template "cites" .Cites}}
</article>
{{end}}
` + citesTmpl))

// citesTmpl is a row of citations, each a <details> that expands in place to
// the cited record's title, kind and date, with a link to open it on its own
// page. Records draws one under every record (MUS-D-0040), and the decision
// queue draws one under every question (MUS-D-0198, extending MUS-D-0040 on the
// owner's answer to MUS-Q-0160), so the two cannot drift apart.
const citesTmpl = `{{define "cites"}}{{if .}}<div class="cites">
    {{range .}}<details>
      <summary class="badge">{{.ID}}</summary>
      <div class="inner">{{if .Known}}<strong>{{.Title}}</strong><br><small>{{.Kind}} · {{.At}} · <a href="/records/{{.ID}}">open on its own</a></small>{{else}}Nothing in the store has this identifier.{{end}}</div>
    </details>{{end}}
  </div>{{end}}{{end}}`

// citesCSS styles citesTmpl. Scoped to .cites, because the queue has
// <details> of its own -- an option's "more" -- that must not take these rules.
// Records' global details and summary rules say the same for its own refs.
const citesCSS = `
  .cites { display: flex; gap: .35rem; flex-wrap: wrap; margin-top: .4rem; }
  .cites details { margin: .25rem 0; font-size: .88em; min-width: 0; }
  .cites summary { cursor: pointer; opacity: .8; overflow-wrap: anywhere; }
  .cites details .inner { margin: .3rem 0 .5rem 1rem; padding-left: .6rem;
                          border-left: 2px solid var(--edge); overflow-wrap: anywhere; }
  .badge { font-size: .78em; border: 1px solid var(--edge); border-radius: 999px;
           padding: .05rem .5rem; opacity: .75; }
`
