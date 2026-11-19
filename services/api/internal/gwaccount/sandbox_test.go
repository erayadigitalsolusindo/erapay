package gwaccount

import "testing"

func TestGatewaysForWorlds(t *testing.T) {
	s := NewService(nil, nil, "production")
	if _, ok := s.gatewaysFor("sandbox"); ok {
		t.Fatal("sandbox served before SetSandbox")
	}
	if _, ok := s.gatewaysFor("production"); !ok {
		t.Fatal("own environment must be served")
	}
	dev := NewService(nil, nil, "sandbox")
	dev.SetSandbox(nil)
	if _, ok := dev.gatewaysFor("production"); ok {
		t.Fatal("non-production server must not serve production")
	}
}
