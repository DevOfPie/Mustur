package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DevOfPie/Mustur/internal/account"
	"github.com/DevOfPie/Mustur/internal/intake"
	"github.com/DevOfPie/Mustur/internal/record"
	"github.com/DevOfPie/Mustur/internal/store"
)

func projectRecord(id, title, prefix string) record.Record {
	return record.Record{ID: id, Kind: "project", Title: title, At: "2026-09-19",
		Data: []record.Field{{Key: intake.PrefixField, Value: prefix}}}
}

// A project registered after the owner was granted used to be on nobody's
// People screen, because nobody owned it. It is on the install owner's now —
// including one written while the server is running — and a project somebody
// else owns is still not (MUS-D-0188).
func TestANewProjectIsOnTheOwnersPeopleScreen(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	for _, r := range []record.Record{
		projectRecord("MUS-P-0001", "Mustur", "MUS"),
		projectRecord("MUS-P-0002", "Hoard", "HRD"),
		projectRecord("MUS-P-0003", "LinkCtrl", "LNK"),
	} {
		if err := st.Append(ctx, r, "create", "test"); err != nil {
			t.Fatal(err)
		}
	}
	accounts := account.New(st.DB())
	mux := http.NewServeMux()
	auth := &Auth{Accounts: accounts, Origin: "http://127.0.0.1"}
	auth.Routes(mux)
	manage := &Accounts{Store: accounts, Auth: auth, Project: "MUS", Records: st}
	manage.Routes(mux)
	srv := httptest.NewServer((&Guard{Auth: auth, Project: "MUS"}).Wrap(mux))
	t.Cleanup(srv.Close)

	owner, _ := personWith(t, accounts, "owner@example.com", "MUS", account.Owner, "k")
	personWith(t, accounts, "lnk@example.com", "LNK", account.Owner, "k2")
	personWith(t, accounts, "friend@example.com", "MUS", account.Reader, "k3")

	page := body(t, owner, srv.URL+"/account/people")
	if !strings.Contains(page, `value="HRD"`) {
		t.Error("Hoard, which nobody owned, is not on the owner's People screen")
	}
	if strings.Contains(page, `value="LNK"`) {
		t.Error("LinkCtrl, which somebody else owns, is on the owner's People screen")
	}

	if err := st.Append(ctx, projectRecord("MUS-P-0004", "Idea Warehouse", "IDW"), "create", "test"); err != nil {
		t.Fatal(err)
	}
	if page := body(t, owner, srv.URL+"/account/people"); !strings.Contains(page, `value="IDW"`) {
		t.Error("a project added while the server runs is not on the next render")
	}
}
