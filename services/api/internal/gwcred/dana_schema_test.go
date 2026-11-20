package gwcred

import "testing"

// DANA key fields accept bare base64 (as the DANA dashboard copies it); other PEM fields (DOKU) still need PEM.
func TestDanaKeyFieldsAcceptBareBase64(t *testing.T) {
	d, _ := SchemaOf("dana")
	doku, _ := SchemaOf("doku")
	for _, k := range []string{"private_key", "dana_public_key"} {
		f, _ := d.field(k)
		if err := f.check("MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A\nMIIBCgKCAQEA"); err != nil {
			t.Errorf("%s bare base64: %v", k, err)
		}
		if err := f.check("not base64 !!"); err == nil {
			t.Errorf("%s accepted junk", k)
		}
		if err := f.check("-----BEGIN PUBLIC KEY-----\nQUJD\n  -----END PUBLIC KEY-----"); err != nil {
			t.Errorf("%s PEM with stray space: %v", k, err)
		}
	}
	if f, _ := doku.field("private_key"); f.check("MIIBIjANBgkqhkiG9w0B") == nil {
		t.Error("DOKU private key must still require PEM")
	}
	if f, _ := d.field("channel_id"); f.Required {
		t.Error("channel_id must be optional (defaults to 95221)")
	}
}
