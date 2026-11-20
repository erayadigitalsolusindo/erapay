package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPKTicketOnceAndExpiry(t *testing.T) {
	tk, err := issueAPKTicket("u1")
	if err != nil {
		t.Fatal(err)
	}
	if uid, ok := takeAPKTicket(tk); !ok || uid != "u1" {
		t.Fatalf("first use: %q %v", uid, ok)
	}
	if _, ok := takeAPKTicket(tk); ok {
		t.Fatal("ticket must be single-use")
	}
	tk, _ = issueAPKTicket("u1")
	apkTickets.Lock()
	apkTickets.m[tk] = apkTicket{"u1", time.Now().Add(-time.Second)}
	apkTickets.Unlock()
	if _, ok := takeAPKTicket(tk); ok {
		t.Fatal("expired ticket accepted")
	}
}

func TestAPKDirectCORSAndAuth(t *testing.T) {
	d := Deps{WebURL: "https://web.example"}
	req := httptest.NewRequest(http.MethodOptions, "/v1/platform/listener-apk/upload", nil)
	req.Header.Set("Origin", "https://web.example")
	rec := httptest.NewRecorder()
	d.platformUploadListenerDirect(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://web.example" {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/platform/listener-apk/upload?ticket=nope&version=1", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	d.platformUploadListenerDirect(rec, req)
	if rec.Code != 401 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("bad ticket / foreign origin: %d %v", rec.Code, rec.Header())
	}
}
