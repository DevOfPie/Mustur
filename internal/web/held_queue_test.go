package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/export"
	"github.com/DevOfPie/Mustur/internal/question"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// queueRig is heldServer with the decision queue and the records surface on
// the same guarded mux, so a test follows a jot from a reader's send to an
// owner's press and into the store.
func queueRig(t *testing.T) heldRig {
	t.Helper()
	h := heldServer(t)
	q := &Questions{Store: h.st, Project: "MUS", Actor: "pie", Roles: h.accounts}
	q.Routes(h.mux)
	rr := &Records{Store: h.st, Project: "MUS"}
	rr.Routes(h.mux)
	return h
}

const sentLine = "The share link on a phone opens the desktop layout"

func holdOne(t *testing.T, h heldRig, reader *http.Client, to string) store.Held {
	t.Helper()
	v := url.Values{"jot": {sentLine}}
	if to != "" {
		v.Set("to", to)
	}
	if res, _ := sendForm(t, reader, h.srv.URL+"/intake", v); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("the reader's send answered %d", res.StatusCode)
	}
	held, err := h.st.HeldJots(context.Background(), "")
	if err != nil || len(held) == 0 {
		t.Fatalf("nothing held: %v", err)
	}
	return held[len(held)-1]
}

func approveAs(t *testing.T, h heldRig, c *http.Client, id, to string) *http.Response {
	t.Helper()
	res, _ := sendForm(t, c, h.srv.URL+"/intake/held/"+id+"/approve", url.Values{"to": {to}})
	return res
}

func TestAnOwnerMeetsHeldJotsAtTheTopOfDecisions(t *testing.T) {
	h := queueRig(t)
	ctx := context.Background()
	if err := h.st.Append(ctx, openQuestion("MUS-Q-0001", "Is this right?"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	holdOne(t, h, reader, "")

	page := bodyOf(t, owner, h.srv.URL+"/questions")
	top := strings.Index(page, "Jots waiting for approval")
	q := strings.Index(page, "Is this right?")
	if top < 0 || q < 0 || top > q {
		t.Fatalf("held jots are not above the questions (held at %d, question at %d)", top, q)
	}
	for _, want := range []string{"friend@example.com", sentLine, "File to", "Approve and file", "Discard",
		`<option value="" selected>Route it for me (Mustur)</option>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the owner's card is missing %q", want)
		}
	}
	// The rule that dims Answer until an option is chosen matched a held card's
	// form too — it has no radio and no textarea — and left Approve and file
	// unpressable; found in a browser, not by a test.
	if !strings.Contains(page, "form:not(.held):not(:has(input[type=radio]:checked))") {
		t.Error("the Answer-dimming rule no longer excludes held cards, so Approve is unpressable")
	}
	// A reader is not shown anybody's held jots on Decisions.
	if rp := bodyOf(t, reader, h.srv.URL+"/questions"); strings.Contains(rp, "Jots waiting for approval") {
		t.Error("a reader is shown the approval section")
	}
}

// Nothing reads a held jot before it is approved: not the record listing
// behind `mustur list` and the export, not the export itself, and not the
// records surface.
func TestAHeldJotIsInNoRecordPathUntilApproved(t *testing.T) {
	h := queueRig(t)
	ctx := context.Background()
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	held := holdOne(t, h, reader, "")

	inRecords := func() bool {
		all, err := h.st.List(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		files, err := export.Render(all)
		if err != nil {
			t.Fatal(err)
		}
		listed, exported := false, false
		for _, r := range all {
			if strings.Contains(r.Body, "share link") {
				listed = true
			}
		}
		for _, b := range files {
			if strings.Contains(string(b), "share link") {
				exported = true
			}
		}
		page := strings.Contains(bodyOf(t, owner, h.srv.URL+"/records"), "share link")
		if listed != exported || listed != page {
			t.Errorf("the paths disagree: listed %v, exported %v, on /records %v", listed, exported, page)
		}
		return listed
	}
	if inRecords() {
		t.Fatal("a held jot is in the records before approval")
	}
	if res := approveAs(t, h, owner, held.ID, ""); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("approve answered %d", res.StatusCode)
	}
	if !inRecords() {
		t.Error("an approved jot is not in the records")
	}
}

func TestApprovalFilesOnceWithFilerAndApprover(t *testing.T) {
	h := queueRig(t)
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	held := holdOne(t, h, reader, "")

	res := approveAs(t, h, owner, held.ID, "")
	if res.StatusCode != http.StatusSeeOther || !strings.Contains(res.Header.Get("Location"), "filed=MUS-F-") {
		t.Fatalf("approve: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	// The second press, from another tab or a resent POST.
	second := approveAs(t, h, owner, held.ID, "")
	if !strings.Contains(second.Header.Get("Location"), "error=") {
		t.Errorf("a second approve was not told it is gone: %s", second.Header.Get("Location"))
	}
	got := findings(t, h.st)
	if len(got) != 1 {
		t.Fatalf("two approvals filed %d record(s)", len(got))
	}
	r := got[0]
	if by, _ := r.Get("Filed by"); by != "friend@example.com" {
		t.Errorf("Filed by %q, want the reader", by)
	}
	if by, _ := r.Get("Approved by"); by != "owner@example.com" {
		t.Errorf("Approved by %q, want the owner", by)
	}
	if r.Body != sentLine {
		t.Errorf("the record's body is %q", r.Body)
	}
	if left, _ := h.st.HeldJots(context.Background(), ""); len(left) != 0 {
		t.Errorf("still held after approval: %+v", left)
	}
}

// The owner may point it elsewhere before approving; the record goes where the
// press said.
func TestTheOwnerCanChangeWhereItIsFiled(t *testing.T) {
	h := queueRig(t)
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner, "IDW": account.Owner})
	held := holdOne(t, h, reader, "MUS-P-0001")
	if res := approveAs(t, h, owner, held.ID, "MUS-P-0002"); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("approve: %d", res.StatusCode)
	}
	got := findings(t, h.st)
	if len(got) != 1 || !strings.HasPrefix(got[0].ID, "IDW-F-") {
		t.Fatalf("filed as %+v, want under IDW", got)
	}
	// The reader did not pick the idea inbox, so the record must not say they did.
	if why, _ := got[0].Get("Routing"); why != "chosen by the approver" {
		t.Errorf("Routing %q, want the approver credited with the choice", why)
	}
}

// Left where the reader pointed it, the choice is still the reader's.
func TestAnUnchangedDestinationStaysTheFilersChoice(t *testing.T) {
	h := queueRig(t)
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	held := holdOne(t, h, reader, "MUS-P-0001")
	if res := approveAs(t, h, owner, held.ID, "MUS-P-0001"); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("approve: %d", res.StatusCode)
	}
	got := findings(t, h.st)
	if len(got) != 1 {
		t.Fatalf("filed %d", len(got))
	}
	if why, _ := got[0].Get("Routing"); why != "chosen by the filer" {
		t.Errorf("Routing %q, want the filer's choice", why)
	}
}

// An owner who moves a reader's chosen destination back to "Route it for me"
// chose nothing: the record carries the guess's own reason.
func TestRouteItForMeKeepsTheGuesssReason(t *testing.T) {
	h := queueRig(t)
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	held := holdOne(t, h, reader, "MUS-P-0001")
	if res := approveAs(t, h, owner, held.ID, ""); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("approve: %d", res.StatusCode)
	}
	got := findings(t, h.st)
	if len(got) != 1 {
		t.Fatalf("filed %d", len(got))
	}
	if why, _ := got[0].Get("Routing"); strings.HasPrefix(why, "chosen by") {
		t.Errorf("Routing %q credits a choice for a routed jot", why)
	}
}

// Only an owner of the project the jot is filed under may approve it.
func TestAnOwnerWhoDoesNotOwnTheDestinationIsRefused(t *testing.T) {
	h := queueRig(t)
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	held := holdOne(t, h, reader, "MUS-P-0002") // the idea inbox, IDW

	if res := approveAs(t, h, owner, held.ID, "MUS-P-0002"); res.StatusCode != http.StatusForbidden {
		t.Errorf("an owner of MUS approving into IDW got %d", res.StatusCode)
	}
	if res, _ := sendForm(t, owner, h.srv.URL+"/intake/held/"+held.ID+"/discard", nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("an owner of MUS discarding a jot sent to IDW got %d", res.StatusCode)
	}
	if got := findings(t, h.st); len(got) != 0 {
		t.Errorf("a refused approval filed %s", got[0].ID)
	}
	if left, _ := h.st.HeldJots(context.Background(), ""); len(left) != 1 {
		t.Errorf("a refused approval did not leave the jot waiting: %+v", left)
	}
	// And it is neither shown to nor counted for them.
	if page := bodyOf(t, owner, h.srv.URL+"/questions"); strings.Contains(page, "Jots waiting for approval") {
		t.Error("an owner is shown a jot they may not approve")
	}
	if n := waitingFor(t, owner, h); n != 0 {
		t.Errorf("an owner's badge counts %d jot(s) they may not approve", n)
	}
	// An owner of MUS who is not an owner of IDW cannot route around it either:
	// pointing an IDW jot at MUS is theirs to approve.
	if res := approveAs(t, h, owner, held.ID, "MUS-P-0001"); res.StatusCode != http.StatusSeeOther {
		t.Errorf("re-pointing to a project they own got %d", res.StatusCode)
	}
}

func TestDiscardLeavesNoRowAndNoRecord(t *testing.T) {
	h := queueRig(t)
	ctx := context.Background()
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	held := holdOne(t, h, reader, "")
	before, _ := h.st.Count(ctx)

	res, _ := sendForm(t, owner, h.srv.URL+"/intake/held/"+held.ID+"/discard", nil)
	if res.StatusCode != http.StatusSeeOther || !strings.Contains(res.Header.Get("Location"), "discarded=1") {
		t.Fatalf("discard: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	var rows int
	if err := h.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM held_jot`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	after, _ := h.st.Count(ctx)
	if rows != 0 || after != before {
		t.Errorf("discard left %d held row(s) and moved the record count %d → %d", rows, before, after)
	}
	if page := bodyOf(t, reader, h.srv.URL+"/intake"); strings.Contains(page, "Waiting for an owner") {
		t.Error("a discarded jot is still listed as waiting for the reader")
	}
}

func waitingFor(t *testing.T, c *http.Client, h heldRig) int {
	t.Helper()
	res, err := c.Get(h.srv.URL + "/questions/count")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var got struct {
		Waiting int `json:"waiting"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Waiting
}

func TestTheBadgeCountsHeldJots(t *testing.T) {
	h := queueRig(t)
	ctx := context.Background()
	q := record.Record{ID: "MUS-Q-0001", Kind: question.Kind, Title: "Open", At: "2026-09-15",
		Data: []record.Field{{Key: question.FieldStatus, Value: question.StatusOpen}}}
	if err := h.st.Append(ctx, q, "create", "test"); err != nil {
		t.Fatal(err)
	}
	reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	holdOne(t, h, reader, "")

	if n := waitingFor(t, owner, h); n != 2 {
		t.Errorf("the owner's count is %d, want one question and one held jot", n)
	}
	if n := waitingFor(t, reader, h); n != 1 {
		t.Errorf("the reader's count is %d, want the question alone", n)
	}
	for _, path := range []string{"/questions", "/intake"} {
		if page := bodyOf(t, owner, h.srv.URL+path); !strings.Contains(page, `<em class="cnt">2</em>`) {
			t.Errorf("the rendered badge on %s does not count the held jot", path)
		}
	}
}
