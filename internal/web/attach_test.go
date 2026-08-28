package web

// A picture reaches the record and never reaches the export.
//
// The second half is the one that matters. records/ is committed and this
// repository is public, so a screenshot written there would publish whatever
// was on the owner's screen — permanently, and past any later deletion. The
// owner's decision was that an agent's reading of the image travels and the
// pixels do not, and the test below is what keeps that true.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DevOfPie/Mustur/internal/export"
)

// aPNG is a real one, so the sniffer sees what a phone would send.
func aPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			m.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fileJot posts a jot with an optional attachment, the way the form does.
func fileJot(t *testing.T, srv *httptest.Server, text, filename string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("jot", text); err != nil {
		t.Fatal(err)
	}
	if data != nil {
		part, err := mw.CreateFormFile("image", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := srv.Client().Post(srv.URL+"/intake", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAJotCanCarryAPicture(t *testing.T) {
	srv, st := serve(t)
	defer srv.Close()
	ctx := context.Background()

	res := fileJot(t, srv, "the tab bar is covering the text", "screenshot.png", aPNG(t, 40, 30))
	res.Body.Close()

	all, err := st.List(ctx, "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("%d findings", len(all))
	}
	shots, err := st.Attachments(ctx, all[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 1 {
		t.Fatalf("%d attachments on %s", len(shots), all[0].ID)
	}
	if shots[0].MediaType != "image/png" {
		t.Errorf("stored as %q; the type is sniffed from the bytes, not taken from the sender", shots[0].MediaType)
	}
}

// The whole point of holding them in the store.
func TestTheExportNeverCarriesImageBytes(t *testing.T) {
	srv, st := serve(t)
	defer srv.Close()
	ctx := context.Background()

	shot := aPNG(t, 64, 64)
	res := fileJot(t, srv, "a picture worth keeping private", "IMG_20260826_bedroom.png", shot)
	res.Body.Close()

	dir := t.TempDir()
	all, err := st.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := export.Write(dir, all); err != nil {
		t.Fatal(err)
	}

	// Not the bytes, not the PNG header, and not the filename either — a
	// filename carries a date, a device and often the content.
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, shot[:16]) {
			t.Errorf("%s carries the image bytes", p)
		}
		if bytes.Contains(b, []byte("\x89PNG")) {
			t.Errorf("%s carries a PNG header", p)
		}
		if bytes.Contains(b, []byte("IMG_20260826_bedroom")) {
			t.Errorf("%s carries the sender's filename", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Only pictures, and only ones a browser decodes rather than executes.
func TestWhatIsRefused(t *testing.T) {
	srv, st := serve(t)
	defer srv.Close()
	ctx := context.Background()

	for _, c := range []struct {
		name, why string
		data      []byte
	}{
		{"script.svg", "SVG is XML that can carry script and would run on this origin",
			[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"notes.txt", "not an image at all", []byte("just some words, at length, pretending")},
		{"payload.png", "named like a picture and is not one", []byte("MZ\x90\x00this is a program")},
	} {
		res := fileJot(t, srv, "a jot with "+c.name, c.name, c.data)
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !strings.Contains(string(body), "not an image") {
			t.Errorf("%s was not refused (%s)", c.name, c.why)
		}
	}
	// And none of them left a record behind claiming an attachment.
	all, err := st.List(ctx, "finding")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("%d findings were stored for refused images; a refused picture must not leave a jot", len(all))
	}
}

// The bytes come back with the type they were stored as, and told not to be
// interpreted as anything else.
func TestAnImageIsServedAsAnImageAndNothingElse(t *testing.T) {
	srv, st := serve(t)
	defer srv.Close()
	ctx := context.Background()

	res := fileJot(t, srv, "a picture", "shot.png", aPNG(t, 20, 20))
	res.Body.Close()
	all, _ := st.List(ctx, "finding")
	shots, _ := st.Attachments(ctx, all[0].ID)
	if len(shots) != 1 {
		t.Fatal("nothing attached")
	}

	mux := http.NewServeMux()
	(&Records{Store: st, Project: "MUS"}).Routes(mux)
	rec := httptest.NewServer(mux)
	defer rec.Close()

	got, err := rec.Client().Get(rec.URL + "/records/image/" + shots[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("serving the image returned %d", got.StatusCode)
	}
	if ct := got.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("served as %q", ct)
	}
	if got.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("no nosniff, so a browser may talk itself into treating it as a document")
	}
	if !strings.Contains(got.Header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Error("no closed content policy on somebody's screenshot")
	}
	if !strings.Contains(got.Header.Get("Cache-Control"), "no-store") {
		t.Error("a screenshot should not be cached like a static asset")
	}
	if code := statusOf(t, rec.Client(), rec.URL+"/records/image/deadbeef"); code != http.StatusNotFound {
		t.Errorf("an unknown image returned %d", code)
	}
}

// A scratch filing costs no identifier, which is the whole reason it exists.
//
// The owner tested the picture upload twice and it left IDW-F-0002 and
// IDW-F-0003 in the idea warehouse forever, both saying "test" in their own
// titles. An identifier here is permanent and the log only ever grows, so a
// test filing must not advance the counter.
func TestAScratchFilingIsNotARecord(t *testing.T) {
	srv, st := serve(t)
	defer srv.Close()
	ctx := context.Background()

	before, err := st.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("jot", "testing the box again")
	_ = mw.WriteField("to", "scratch")
	part, err := mw.CreateFormFile("image", "shot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(aPNG(t, 16, 16)); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	res, err := srv.Client().Post(srv.URL+"/intake", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	// Not a record, and the counter has not moved.
	after, err := st.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("the log grew from %d to %d; a scratch filing must not take an identifier",
			len(before), len(after))
	}

	// It is there, though, and it is readable.
	left, err := st.Scratches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Text != "testing the box again" {
		t.Fatalf("scratch holds %+v", left)
	}
	if strings.HasPrefix(left[0].ID, "MUS-") || strings.HasPrefix(left[0].ID, "IDW-") {
		t.Errorf("a scratch filing was given something that looks like an identifier: %q", left[0].ID)
	}

	// The picture went with it, keyed to the scratch rather than to a record.
	shots, err := st.Attachments(ctx, left[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 1 {
		t.Fatalf("%d pictures on the scratch filing", len(shots))
	}

	// And the sweep takes both, so no picture is left unreachable.
	if _, err := st.SweepScratch(ctx, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if gone, _ := st.Scratches(ctx); len(gone) != 0 {
		t.Errorf("%d scratch filings survived the sweep", len(gone))
	}
	if _, _, err := st.Image(ctx, shots[0].ID); err == nil {
		t.Error("the picture outlived the scratch filing it was attached to")
	}
}

// It never reaches the exported tree either, because it is not a record and the
// export only ever writes records.
func TestAScratchFilingIsNeverExported(t *testing.T) {
	srv, st := serve(t)
	defer srv.Close()
	ctx := context.Background()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("jot", "a phrase that would be easy to find")
	_ = mw.WriteField("to", "scratch")
	mw.Close()
	res, err := srv.Client().Post(srv.URL+"/intake", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	dir := t.TempDir()
	all, err := st.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := export.Write(dir, all); err != nil {
		t.Fatal(err)
	}
	err = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte("a phrase that would be easy to find")) {
			t.Errorf("%s carries a scratch filing", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
