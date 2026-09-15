package web

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// heldServer is the real surfaces behind the real guard, with two projects a
// jot can be routed to: Mustur (MUS), the default, and an idea inbox (IDW).
type heldRig struct {
	srv      *httptest.Server
	st       *store.Store
	accounts *account.Store
	mux      *http.ServeMux
}

func heldServer(t *testing.T) heldRig {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, r := range []record.Record{
		{ID: "MUS-P-0001", Kind: "project", Title: "Mustur", At: "2026-08-20",
			Data: []record.Field{{Key: intake.DefaultField, Value: intake.DefaultValue},
				{Key: intake.PrefixField, Value: "MUS"}}},
		{ID: "MUS-P-0002", Kind: "project", Title: "Idea inbox", At: "2026-08-20",
			Data: []record.Field{{Key: intake.PrefixField, Value: "IDW"}}},
	} {
		if err := st.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	accounts := account.New(st.DB())
	mux := http.NewServeMux()
	in := &Intake{Store: st, Project: "MUS", Actor: "pie", Roles: accounts}
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	mux.Handle("/", in.Handler())
	guard := &Guard{Auth: auth, Project: "MUS"}
	srv := httptest.NewServer(guard.Wrap(mux))
	t.Cleanup(srv.Close)
	return heldRig{srv: srv, st: st, accounts: accounts, mux: mux}
}

func (h heldRig) as(t *testing.T, email string, roles map[string]account.Role) (*http.Client, account.Account) {
	t.Helper()
	ctx := context.Background()
	var acct account.Account
	first := true
	for _, project := range []string{"MUS", "IDW"} {
		role, ok := roles[project]
		if !ok {
			continue
		}
		if first {
			secret, err := h.accounts.Invite(ctx, email, project, role, "test")
			if err != nil {
				t.Fatal(err)
			}
			if acct, _, err = h.accounts.Redeem(ctx, secret, ""); err != nil {
				t.Fatal(err)
			}
			first = false
			continue
		}
		if err := h.accounts.Grant(ctx, acct.ID, project, role, "test"); err != nil {
			t.Fatal(err)
		}
	}
	cookie, _, err := h.accounts.StartSession(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     cookieAdder{cookie: &http.Cookie{Name: SessionCookie, Value: cookie}},
	}, acct
}

func sendForm(t *testing.T, c *http.Client, u string, v url.Values) (*http.Response, string) {
	t.Helper()
	res, err := c.PostForm(u, v)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func findings(t *testing.T, st *store.Store) []record.Record {
	t.Helper()
	all, err := st.List(context.Background(), "finding")
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// MUS-D-0189: a reader's send is held, not filed, and they are told so and
// shown it waiting.
func TestAReadersJotIsHeldNotFiled(t *testing.T) {
	h := heldServer(t)
	reader, acct := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})

	page := bodyOf(t, reader, h.srv.URL+"/intake")
	for _, want := range []string{"Send for approval"} {
		if !strings.Contains(page, want) {
			t.Errorf("a reader's box is missing %q", want)
		}
	}
	for _, not := range []string{`name="image"`, `value="scratch"`, ">File it<"} {
		if strings.Contains(page, not) {
			t.Errorf("a reader's box offers %q", not)
		}
	}

	res, _ := sendForm(t, reader, h.srv.URL+"/intake", url.Values{"jot": {"The share link on a phone opens the desktop layout"}})
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("a reader's send answered %d, want a redirect", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "sent=1") {
		t.Errorf("redirected to %s", loc)
	}
	if got := findings(t, h.st); len(got) != 0 {
		t.Fatalf("a reader's jot was filed as %s", got[0].ID)
	}
	held, err := h.st.HeldJots(context.Background(), acct.ID)
	if err != nil || len(held) != 1 {
		t.Fatalf("held for the reader: %+v, %v", held, err)
	}

	after := bodyOf(t, reader, h.srv.URL+loc)
	for _, want := range []string{
		"Sent for approval. An owner files it or discards it.",
		"Waiting for an owner",
		"The share link on a phone opens the desktop layout",
		"Route it for me",
	} {
		if !strings.Contains(after, want) {
			t.Errorf("after sending, the page is missing %q", want)
		}
	}
	if !strings.Contains(after, " PDT") && !strings.Contains(after, " PST") {
		t.Error("the time a jot was sent is not tagged Pacific")
	}
}

// Pictures stay owner-only in this cut, and a picture is refused with the words
// kept rather than silently dropped.
func TestAReadersPictureIsRefused(t *testing.T) {
	h := heldServer(t)
	reader, acct := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("jot", "look at this")
	part, _ := mw.CreateFormFile("image", "shot.png")
	_ = png.Encode(part, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/intake", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := reader.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(b), "pictures are not taken") {
		t.Errorf("a reader's picture got %d:\n%s", res.StatusCode, b)
	}
	if !strings.Contains(string(b), "look at this") {
		t.Error("the words did not come back with the refusal")
	}
	if held, _ := h.st.HeldJots(context.Background(), acct.ID); len(held) != 0 {
		t.Errorf("a jot with a picture was held: %+v", held)
	}
	var n int
	_ = h.st.DB().QueryRow(`SELECT COUNT(*) FROM attachment`).Scan(&n)
	if n != 0 {
		t.Errorf("a reader's picture reached the store: %d attachment(s)", n)
	}
}

func TestAReaderCannotSendToScratch(t *testing.T) {
	h := heldServer(t)
	reader, acct := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
	_, b := sendForm(t, reader, h.srv.URL+"/intake", url.Values{"jot": {"a line"}, "to": {"scratch"}})
	if !strings.Contains(b, "Not sent:") {
		t.Errorf("a reader's scratch send was not refused:\n%s", b)
	}
	if held, _ := h.st.HeldJots(context.Background(), acct.ID); len(held) != 0 {
		t.Errorf("held anyway: %+v", held)
	}
}

// An owner's box is unchanged: it files.
func TestAnOwnersJotIsStillFiled(t *testing.T) {
	h := heldServer(t)
	owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
	if page := bodyOf(t, owner, h.srv.URL+"/intake"); !strings.Contains(page, ">File it<") || !strings.Contains(page, `name="image"`) {
		t.Error("an owner's box lost its button or picture field")
	}
	res, _ := sendForm(t, owner, h.srv.URL+"/intake", url.Values{"jot": {"an owner's line"}})
	if res.StatusCode != http.StatusSeeOther || !strings.Contains(res.Header.Get("Location"), "filed=") {
		t.Errorf("an owner's send: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if got := findings(t, h.st); len(got) != 1 {
		t.Errorf("an owner's jot filed %d record(s)", len(got))
	}
	if held, _ := h.st.HeldJots(context.Background(), ""); len(held) != 0 {
		t.Errorf("an owner's jot was held: %+v", held)
	}
}
