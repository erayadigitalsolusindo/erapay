package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apprelease"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func TestListenerApkUploadThenDownloadHandlers(t *testing.T) {
	db := testdb.New(t)
	svc, err := apprelease.NewService(db, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := Deps{AppReleases: svc, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	var apk bytes.Buffer
	zw := zip.NewWriter(&apk)
	for _, n := range []string{"AndroidManifest.xml", "classes.dex"} {
		f, _ := zw.Create(n)
		_, _ = f.Write([]byte("x"))
	}
	_ = zw.Close()

	// The routes sit behind platform auth; the handler reads the actor from the request context.
	asDewa := func(r *http.Request) *http.Request {
		return r.WithContext(context.WithValue(r.Context(), ctxKey{}, &auth.Principal{}))
	}

	// Nothing published yet: tenant sees "not available", download is 404.
	rec := httptest.NewRecorder()
	d.qrisgateListenerInfo(rec, httptest.NewRequest(http.MethodGet, "/v1/qrisgate/listener", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"available":false`) {
		t.Fatalf("info before upload: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	d.qrisgateListenerDownload(rec, httptest.NewRequest(http.MethodGet, "/v1/qrisgate/listener/download", nil))
	if rec.Code != 404 {
		t.Fatalf("download before upload: %d", rec.Code)
	}

	for name, c := range map[string]struct {
		url  string
		body []byte
		code int
	}{
		"not an apk":  {"/v1/platform/listener-apk?version=1.0.0", []byte("nope"), 422},
		"bad version": {"/v1/platform/listener-apk?version=a/b", apk.Bytes(), 400},
	} {
		rec = httptest.NewRecorder()
		d.platformUploadListener(rec, asDewa(httptest.NewRequest(http.MethodPost, c.url, bytes.NewReader(c.body))))
		if rec.Code != c.code {
			t.Errorf("%s: %d %s; want %d", name, rec.Code, rec.Body, c.code)
		}
	}

	rec = httptest.NewRecorder()
	d.platformUploadListener(rec, asDewa(httptest.NewRequest(http.MethodPost, "/v1/platform/listener-apk?version=1.0.0&note=awal", bytes.NewReader(apk.Bytes()))))
	if rec.Code != 201 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	d.platformUploadListener(rec, asDewa(httptest.NewRequest(http.MethodPost, "/v1/platform/listener-apk?version=1.0.0", bytes.NewReader(apk.Bytes()))))
	if rec.Code != 409 {
		t.Fatalf("same version again: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	d.qrisgateListenerDownload(rec, httptest.NewRequest(http.MethodGet, "/v1/qrisgate/listener/download", nil))
	h := rec.Header()
	if rec.Code != 200 || h.Get("Content-Type") != "application/vnd.android.package-archive" ||
		h.Get("Content-Disposition") != `attachment; filename="erapay-listener-1.0.0.apk"` || h.Get("X-Content-Type-Options") != "nosniff" ||
		!bytes.Equal(rec.Body.Bytes(), apk.Bytes()) {
		t.Fatalf("download: %d %v", rec.Code, h)
	}
}
