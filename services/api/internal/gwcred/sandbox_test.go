package gwcred

import (
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

type liveRec struct{ replaced []string }

func (l *liveRec) Replace(a gateway.Adapter) { l.replaced = append(l.replaced, a.Code()) }

// A production server serves both worlds; any other server serves only its own.
func TestLiveForWorlds(t *testing.T) {
	prod, sbx := &liveRec{}, &liveRec{}
	s := NewService(nil, nil, "production", prod, nil)
	if s.liveFor("sandbox") != nil {
		t.Fatal("sandbox served before SetSandbox")
	}
	s.SetSandbox(sbx)
	if s.liveFor("production") != Live(prod) || s.liveFor("sandbox") != Live(sbx) {
		t.Fatal("production server must route each environment to its own registry")
	}
	dev := NewService(nil, nil, "sandbox", sbx, nil)
	dev.SetSandbox(prod) // ignored: only a production server hosts a second world
	if dev.liveFor("production") != nil || dev.liveFor("sandbox") != Live(sbx) {
		t.Fatal("non-production server must serve only its own environment")
	}
}
