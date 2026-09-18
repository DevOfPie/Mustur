package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

// MUS-F-0174: a title is inline markdown, never a block, and never raw HTML.
func TestATitleRendersInlineMarkdownOnly(t *testing.T) {
	for _, c := range []struct{ in, html, text string }{
		// LNK-D-0010's shape.
		{"Install with `go install ./cmd/linkctrl`", `Install with <code class="t-code">go install ./cmd/linkctrl</code>`, "Install with go install ./cmd/linkctrl"},
		{"A *light* and a **heavy** one", "A <em>light</em> and a <strong>heavy</strong> one", "A light and a heavy one"},
		{"Plain", "Plain", "Plain"},
		{"", "", ""},
		// Raw HTML is text: escaped in the markup, the characters in the text.
		{"Never <script>alert(1)</script> here", "Never &lt;script&gt;alert(1)&lt;/script&gt; here", "Never <script>alert(1)</script> here"},
		{"`<b>` is a tag", `<code class="t-code">&lt;b&gt;</code> is a tag`, "<b> is a tag"},
		// No block: a heading, a list or a quote is the characters.
		{"# Not a heading", "# Not a heading", "# Not a heading"},
		{"- not a list", "- not a list", "- not a list"},
		{"> not a quote", "&gt; not a quote", "> not a quote"},
		// A link is its text, so a title inside an <a> nests no anchor.
		{"See [the docs](https://example.com) first", "See the docs first", "See the docs first"},
		{"[`code` link](x) and ![alt *em*](y.png)", `<code class="t-code">code</code> link and alt <em>em</em>`, "code link and alt em"},
		{"<https://example.com>", "&lt;https://example.com&gt;", "<https://example.com>"},
		// An identifier's underscores are not emphasis, as in a body.
		{"Moves _IB-F-0001_ aside", "Moves _IB-F-0001_ aside", "Moves _IB-F-0001_ aside"},
		{"Fish & chips", "Fish &amp; chips", "Fish & chips"},
	} {
		got := string(title(c.in))
		if got != c.html {
			t.Errorf("title(%q) = %q, want %q", c.in, got, c.html)
		}
		if strings.Contains(got, "<p>") || strings.Contains(got, "<a") || strings.Contains(got, "<h") ||
			strings.Contains(got, "<li") || strings.Contains(got, "<blockquote") {
			t.Errorf("title(%q) = %q carries a block or a link", c.in, got)
		}
		if tx := titleText(c.in); tx != c.text {
			t.Errorf("titleText(%q) = %q, want %q", c.in, tx, c.text)
		}
	}
}

// Every records surface that shows a title renders its code span: the record's
// own heading, the index row, and a citation expanded on another record.
func TestATitleWithCodeReadsAsCodeOnTheRecordsPages(t *testing.T) {
	const code = `<code class="t-code">mustur get</code>`
	srv := serveRecords(t, "",
		decision("MUS-D-0001", "Read one with `mustur get`", "The body."),
		decision("MUS-D-0002", "The citing one", "This follows MUS-D-0001.",
			record.Field{Key: "corrects", Value: "MUS-D-0001"}),
		decision("MUS-D-0003", "The one citing in prose", "This builds on MUS-D-0001."),
	)
	one, _ := fetch(t, srv, "/records/MUS-D-0001")
	if !strings.Contains(one, "<h3>Read one with "+code+"</h3>") {
		t.Error("the record's heading does not render its title's code span")
	}
	if !strings.Contains(one, "code.t-code") {
		t.Error("the record page does not style a title's code")
	}
	index, _ := fetch(t, srv, "/records")
	if !strings.Contains(index, `<span class="t">Read one with `+code+`</span>`) {
		t.Error("the index row does not render its title's code span")
	}
	// A named ref and a citation in prose are drawn by different templates.
	citing, _ := fetch(t, srv, "/records/MUS-D-0002")
	prose, _ := fetch(t, srv, "/records/MUS-D-0003")
	for where, body := range map[string]string{"ref": citing, "prose": prose} {
		if !strings.Contains(body, "<strong>Read one with "+code+"</strong>") {
			t.Errorf("the %s citation does not render the cited title's code span", where)
		}
	}
	for where, body := range map[string]string{"record": one, "index": index, "ref": citing, "prose": prose} {
		if strings.Contains(body, "`mustur get`") {
			t.Errorf("the %s page still shows the backticks", where)
		}
	}
}

// The decision queue's card heading is a title too.
func TestAQuestionTitleWithCodeReadsAsCode(t *testing.T) {
	srv, _ := serveQuestions(t, openQuestion("MUS-Q-0001", "Is `make check` enough?"))
	body := getFrom(t, srv, "/questions")
	if !strings.Contains(body, `<h2>Is <code class="t-code">make check</code> enough?</h2>`) {
		t.Error("the queue card's heading does not render its title's code span")
	}
	if !strings.Contains(body, "code.t-code") {
		t.Error("the queue does not style a title's code")
	}
}

// retitle gives a record already in the store a title with a code span,
// keeping everything else it carries.
func retitle(t *testing.T, st *store.Store, id, to string) {
	t.Helper()
	ctx := context.Background()
	r, err := st.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	r.Title = to
	if err := st.Append(ctx, r, "amend", "test"); err != nil {
		t.Fatal(err)
	}
}

// follow reads the page a redirect points at.
func follow(t *testing.T, base string, res *http.Response) string {
	t.Helper()
	loc := res.Header.Get("Location")
	if loc == "" {
		t.Fatalf("no redirect: %d", res.StatusCode)
	}
	return get(t, base+loc)
}

// Where markup cannot go, a title is its plain text: the intake and queue
// pickers, the start picker and the composer's intake-box target each show
// the words a title's code span holds and none of its backticks. One render
// per surface, each checked for what it must carry.
func TestATitleWithCodeIsPlainWhereMarkupCannotGo(t *testing.T) {
	type surface struct {
		name, body string
		want       []string
	}
	var surfaces []surface

	{ // The intake box's destination picker.
		srv, st := serve(t)
		retitle(t, st, "MUS-P-0002", "Idea `inbox`")
		surfaces = append(surfaces, surface{"intake picker", get(t, srv.URL+"/intake"),
			[]string{`<option value="MUS-P-0002">Idea inbox</option>`}})
	}
	{ // The queue's held-jot picker, and its Route it for me guess.
		h := queueRig(t)
		retitle(t, h.st, "MUS-P-0001", "`Mustur` core")
		reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
		owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner})
		holdOne(t, h, reader, "")
		surfaces = append(surfaces, surface{"queue picker", bodyOf(t, owner, h.srv.URL+"/questions"),
			[]string{`<option value="" selected>Route it for me (Mustur core)</option>`,
				`<option value="MUS-P-0001">Mustur core</option>`}})
	}
	{ // The start picker.
		srv, st, ctx := restoreServer(t, fakeRunner{})
		repo := record.Record{ID: "MUS-R-0001", Kind: "repository", Title: "The `mustur` tree", At: "2026-08-20",
			Data: []record.Field{{Key: "Checkout on MUS-H-0001", Value: "/checkout"}}}
		if err := st.Append(ctx, repo, "create", "test"); err != nil {
			t.Fatal(err)
		}
		surfaces = append(surfaces, surface{"start picker", getFrom(t, srv, "/sessions?new=1"),
			[]string{`<option value="MUS-R-0001">The mustur tree &mdash; /checkout</option>`}})
	}
	{ // The composer's intake-box target.
		now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
		srv, st := serveCompose(t, active("mustur/Mustur", now), true)
		retitle(t, st, "MUS-P-0002", "Idea `inbox`")
		surfaces = append(surfaces, surface{"compose target", getFrom(t, srv, "/compose"),
			[]string{"Idea inbox"}})
	}

	for _, s := range surfaces {
		for _, w := range s.want {
			if !strings.Contains(s.body, w) {
				t.Errorf("the %s does not carry %q", s.name, w)
			}
		}
		for _, raw := range []string{"`inbox`", "`Mustur`", "`mustur`"} {
			if strings.Contains(s.body, raw) {
				t.Errorf("the %s shows %s with its backticks", s.name, raw)
			}
		}
	}
}

// What was just filed says where it went, and the destination's title reads
// as it does everywhere else: intake's done line, its reason and its recent
// row, the queue after approving, and the composer's "filed ... to ...".
func TestWhereAJotWentReadsItsTitlesCode(t *testing.T) {
	const inbox = `Idea <code class="t-code">inbox</code>`
	noClient := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	t.Run("intake", func(t *testing.T) {
		srv, st := serve(t)
		retitle(t, st, "MUS-P-0002", "Idea `inbox`")
		// Named through an alias, so the reason quotes the title.
		r, err := st.Get(context.Background(), "MUS-R-0001")
		if err != nil {
			t.Fatal(err)
		}
		r.Title = "The `mustur` tree"
		r.Data = append(r.Data, record.Field{Key: "Aliases", Value: "mustur tree"})
		if err := st.Append(context.Background(), r, "amend", "test"); err != nil {
			t.Fatal(err)
		}
		res, err := noClient.PostForm(srv.URL+"/intake", url.Values{"jot": {"a thought with no home"}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		done := follow(t, srv.URL, res)
		if !strings.Contains(done, "→ "+inbox) {
			t.Error("the done line does not render the destination's code span")
		}
		if !strings.Contains(done, `<span class="to">`+inbox+` (MUS-P-0002)</span>`) {
			t.Error("the recent row does not render the destination's code span")
		}
		res, err = noClient.PostForm(srv.URL+"/intake", url.Values{"jot": {"the mustur tree should log slow queries"}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		named := follow(t, srv.URL, res)
		if !strings.Contains(named, `<span class="why">the jot names The <code class="t-code">mustur</code> tree</span>`) {
			t.Error("the reason does not render the named title's code span")
		}
		for where, body := range map[string]string{"done": done, "named": named} {
			if strings.Contains(body, "`inbox`") || strings.Contains(body, "`mustur`") {
				t.Errorf("the %s page shows a title's backticks", where)
			}
		}
	})

	t.Run("queue", func(t *testing.T) {
		h := queueRig(t)
		retitle(t, h.st, "MUS-P-0002", "Idea `inbox`")
		reader, _ := h.as(t, "friend@example.com", map[string]account.Role{"MUS": account.Reader})
		owner, _ := h.as(t, "owner@example.com", map[string]account.Role{"MUS": account.Owner, "IDW": account.Owner})
		held := holdOne(t, h, reader, "MUS-P-0001")
		res := approveAs(t, h, owner, held.ID, "MUS-P-0002")
		if res.StatusCode != http.StatusSeeOther {
			t.Fatalf("approve: %d", res.StatusCode)
		}
		page := bodyOf(t, owner, h.srv.URL+res.Header.Get("Location"))
		if !strings.Contains(page, "→ "+inbox+".") {
			t.Error("the queue's filed line does not render the destination's code span")
		}
		if strings.Contains(page, "`inbox`") {
			t.Error("the queue's filed line shows the title's backticks")
		}
	})

	t.Run("compose", func(t *testing.T) {
		now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
		srv, st := serveCompose(t, active("mustur/Mustur", now), true)
		retitle(t, st, "MUS-P-0002", "Idea `inbox`")
		res := post(t, srv, url.Values{"text": {"a thought"}, "to": {"MUS-P-0002"}})
		res.Body.Close()
		page := follow(t, srv.URL, res)
		if !strings.Contains(page, "to Idea inbox</p>") {
			t.Error("the composer does not say where the jot was filed, in plain words")
		}
		if strings.Contains(page, "`inbox`") {
			t.Error("the composer's filed line shows the title's backticks")
		}
	})
}
