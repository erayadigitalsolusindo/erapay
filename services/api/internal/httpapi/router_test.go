package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
