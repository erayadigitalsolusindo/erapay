package httpapi

import (
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

type ackGW struct{ gateway.Adapter }

func (ackGW) WebhookAck() []byte { return []byte(`{"responseCode":"2005600"}`) }

type plainGW struct{ gateway.Adapter }

type adapters map[string]gateway.Adapter

func (a adapters) Get(code string) (gateway.Adapter, bool) { x, ok := a[code]; return x, ok }

// A gateway that needs its own success body gets it; every other gateway keeps the generic answer, and the
// sandbox URL only ever consults the sandbox adapters.
func TestWebhookAck(t *testing.T) {
	d := Deps{Gateways: adapters{"acker": ackGW{}, "plain": plainGW{}}, SandboxGateways: adapters{"sbx": ackGW{}}}
	for name, c := range map[string]struct {
		sandbox bool
		gw      string
		want    string
	}{
		"gateway with ack":         {false, "acker", `{"responseCode":"2005600"}`},
		"gateway without ack":      {false, "plain", ""},
		"unknown gateway":          {false, "nope", ""},
		"sandbox uses sandbox set": {true, "sbx", `{"responseCode":"2005600"}`},
		"sandbox ignores live set": {true, "acker", ""},
		"live ignores sandbox set": {false, "sbx", ""},
	} {
		if got := string(d.webhookAck(c.sandbox, c.gw)); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
	if (Deps{}).webhookAck(false, "x") != nil {
		t.Error("no gateways configured must give the generic answer")
	}
}
