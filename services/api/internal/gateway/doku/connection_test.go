package doku

import (
	"context"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

func stepByName(steps []gateway.ConnectionStep, name string) (gateway.ConnectionStep, bool) {
	for _, s := range steps {
		if s.Name == name {
			return s, true
		}
	}
	return gateway.ConnectionStep{}, false
}

func TestConnectionShallowOnlyChecksCredentials(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(string, map[string]any) (int, any) {
		t.Error("shallow test must not call a transactional endpoint")
		return 500, nil
	}
	steps := a.TestConnection(context.Background(), "MID-9", false)
	if len(steps) != 1 || steps[0].Name != "credentials" || !steps[0].OK {
		t.Fatalf("steps = %+v", steps)
	}
	if f.calls.Load() != 0 {
		t.Fatalf("calls = %d", f.calls.Load())
	}
}

func TestConnectionBadCredentialsStopsEarly(t *testing.T) {
	f, a := newFake(t)
	a.cfg.ClientID = "wrong-client" // fake answers 401 for an unknown client key
	steps := a.TestConnection(context.Background(), "MID-9", true)
	if len(steps) != 1 || steps[0].OK || steps[0].Name != "credentials" {
		t.Fatalf("steps = %+v", steps)
	}
	if f.calls.Load() != 0 {
		t.Fatal("must not reach the merchant step with bad credentials")
	}
}

func TestConnectionDeepCreatesThenExpiresQR(t *testing.T) {
	f, a := newFake(t)
	var paths []string
	f.respond = func(path string, m map[string]any) (int, any) {
		paths = append(paths, path)
		if path == pathGenerate {
			if m["merchantId"] != "MID-9" {
				t.Errorf("merchantId = %v", m["merchantId"])
			}
			if !strings.HasPrefix(m["partnerReferenceNo"].(string), "ERAPAYTEST") {
				t.Errorf("partnerReferenceNo = %v", m["partnerReferenceNo"])
			}
			return okGenerate(path, m)
		}
		return 200, map[string]any{"responseCode": "2004700"}
	}
	steps := a.TestConnection(context.Background(), "MID-9", true)
	for _, n := range []string{"credentials", "merchant", "cleanup"} {
		s, ok := stepByName(steps, n)
		if !ok || !s.OK {
			t.Fatalf("step %s = %+v (all %+v)", n, s, steps)
		}
	}
	if len(paths) != 2 || paths[0] != pathGenerate || paths[1] != pathExpire {
		t.Fatalf("paths = %v", paths)
	}
}

func TestConnectionDeepReportsMerchantRejection(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(path string, _ map[string]any) (int, any) {
		return 400, map[string]any{"responseCode": "4004700", "responseMessage": "Invalid Merchant"}
	}
	steps := a.TestConnection(context.Background(), "BAD", true)
	s, ok := stepByName(steps, "merchant")
	if !ok || s.OK || s.Code != "4004700" || !strings.Contains(s.Message, "Invalid Merchant") {
		t.Fatalf("steps = %+v", steps)
	}
	if _, ok := stepByName(steps, "cleanup"); ok {
		t.Fatal("no cleanup when nothing was created")
	}
}

func TestConnectionDeepEmptyRefFails(t *testing.T) {
	_, a := newFake(t)
	steps := a.TestConnection(context.Background(), "", true)
	s, ok := stepByName(steps, "merchant")
	if !ok || s.OK {
		t.Fatalf("steps = %+v", steps)
	}
}
