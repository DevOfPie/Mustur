package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The access log is only worth having if serve puts it in front of the
// handler, and a test of LogRequests alone would still pass with the wrapping
// deleted from serve. So this goes through newServer, which is what serve
// builds its server with (MUS-F-0162).
func TestServeLogsEveryRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /records", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	var log bytes.Buffer
	srv := newServer("127.0.0.1:0", mux, &log)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/records", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := log.String(); !strings.Contains(got, "GET /records status=200") {
		t.Fatalf("serve's handler wrote no access line: %q", got)
	}
}
