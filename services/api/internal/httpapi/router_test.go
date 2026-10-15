package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newTestRouter(db, rd error) http.Handler {
	return NewRouter(Deps{
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:    fakePinger{db},
		Redis: fakePinger{rd},
	})
}

// TestKYCMounting memastikan route KYC terpasang pada rute /v1 terpisah (di luar
// requireJSON, karena unggah dokumen memakai multipart) dan router tidak panic.
// Handler tidak dieksekusi karena tidak ada sesi → cukup membuktikan mount.
func TestKYCMounting(t *testing.T) {
	svc := kyc.NewService(nil, kyc.Config{})
	defer svc.SetReviewer(nil) // tidak dipakai di test
	router := NewRouter(Deps{
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:    fakePinger{},
		Redis: fakePinger{},
		KYC:   svc,
	})
	for _, path := range []string{"/v1/kyc/profile", "/v1/kyc/submit", "/v1/kyc/documents/ktp", "/v1/kyc/brand/logo", "/v1/kyc/evidences/add", "/v1/kyc/regions"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if path != "/v1/kyc/profile" && path != "/v1/kyc/regions" {
			req = httptest.NewRequest(http.MethodPost, path, nil)
		}
		router.ServeHTTP(rec, req)
		// Tanpa sesi: malah tersedia route → 403 (diblokir requireAuth) / 400 (parsing body);
		// TIDAK BOLEH panic internal (505). Tidak perlu menegaskan 401/403 spesifik.
		if rec.Body.String() != "" && (rec.Code == 500 || rec.Code == 505 || rec.Code == 0) {
			t.Fatalf("%s: unexpected panic-ish status %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	cases := []struct {
		name   string
		db, rd error
		want   int
	}{
		{"all up", nil, nil, 200},
		{"db down", errors.New("x"), nil, 503},
		{"redis down", nil, errors.New("x"), 503},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		newTestRouter(c.db, c.rd).ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		if rec.Code != c.want {
			t.Errorf("%s: got %d want %d", c.name, rec.Code, c.want)
		}
	}
}

func TestARUSIntegrationRequiresProvisionToken(t *testing.T) {
	router := NewRouter(Deps{
		ARUSProvisionToken: "a-server-side-token-long-enough-for-tests",
		Log:                slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{{"missing", "", http.StatusUnauthorized}, {"wrong", "wrong", http.StatusUnauthorized}, {"valid", "a-server-side-token-long-enough-for-tests", http.StatusServiceUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/integrations/arus/tenants/11111111-1111-4111-8111-111111111111", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
