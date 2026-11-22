package gwcred

import (
	"encoding/base64"
	"encoding/pem"
	"regexp"
	"strings"
	"unicode"
)

// Field describes one credential a gateway needs. Only fields declared here can be stored: the
// panel is not a generic env editor, so it can never overwrite DATABASE_URL or the encryption key.
type Field struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Hint      string   `json:"hint,omitempty"`
	Secret    bool     `json:"secret"`    // never returned after saving (only a masked preview)
	Multiline bool     `json:"multiline"` // PEM and the like
	Required  bool     `json:"required"`
	Options   []string `json:"options,omitempty"` // closed set of allowed values (e.g. base URLs)

	pattern *regexp.Regexp
	pem     bool
	b64     bool // with pem: bare base64 DER (no BEGIN/END lines) is accepted too, as DANA's dashboard copies it
}

// Schema is the credential form of one gateway.
type Schema struct {
	Gateway string  `json:"gateway"`
	Label   string  `json:"label"`
	Fields  []Field `json:"fields"`
}

var schemas = []Schema{{
	Gateway: "midtrans",
	Label:   "Midtrans",
	Fields: []Field{
		{Key: "server_key", Label: "Server key", Hint: "SB-Mid-server-... (sandbox) atau Mid-server-... (produksi), Settings > Access Keys di dasbor Midtrans, tampil sebagai pratinjau tersamar setelah disimpan", Required: true, Secret: true, pattern: regexp.MustCompile(`^(SB-)?Mid-server-[A-Za-z0-9_-]{8,100}$`)},
		{Key: "signature_key", Label: "Signature key", Hint: "Secret untuk signature_key notifikasi (Mid-server-...). Di sandbox BUKAN server key SB-; konfirmasi nilainya ke Midtrans. Tanpa ini webhook ditolak", Required: true, Secret: true, pattern: regexp.MustCompile(`^(SB-)?Mid-server-[A-Za-z0-9_-]{8,100}$`)},
	},
}, {
	Gateway: "finpay",
	Label:   "FINPAY",
	Fields: []Field{
		{Key: "merchant_id", Label: "Merchant ID", Required: true, pattern: regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)},
		{Key: "merchant_key", Label: "Merchant key", Hint: "Secret dari dasbor FINPAY, tampil sebagai pratinjau tersamar setelah disimpan", Required: true, Secret: true, pattern: regexp.MustCompile(`^[\x21-\x7e]{1,256}$`)},
		{Key: "callback_url", Label: "Callback URL", Hint: "FINPAY mewajibkannya saat membuat QR. HTTPS publik ke EraPay, mis. https://api.contoh.id/webhooks/finpay", Required: true, pattern: regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:[0-9]{2,5})?(/[\x21-\x7e]*)?$`)},
	},
}, {
	Gateway: "dana",
	Label:   "DANA",
	Fields: []Field{
		{Key: "merchant_id", Label: "Merchant ID", Required: true, pattern: regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)},
		{Key: "client_id", Label: "Client ID (X-PARTNER-ID)", Required: true, pattern: regexp.MustCompile(`^[A-Za-z0-9._-]{1,36}$`)},
		{Key: "store_id", Label: "Store ID", Hint: "Wajib untuk membuat QRIS. Asalnya belum terkonfirmasi (kemungkinan externalShopId dari Create Shop)", Required: true, pattern: regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)},
		{Key: "channel_id", Label: "Channel ID", Hint: "Header CHANNEL-ID, maks 5 karakter. Kosong = 95221, nilai tetap di SDK resmi DANA", pattern: regexp.MustCompile(`^[A-Za-z0-9]{1,5}$`)},
		{Key: "private_key", Label: "Private key (PEM)", Hint: "RSA milik Anda, PKCS#8 atau PKCS#1; PEM atau teks base64 polos seperti disalin dari dasbor DANA", Required: true, Secret: true, Multiline: true, pem: true, b64: true},
		{Key: "mode", Label: "Jalur", Hint: "core = QRIS Acquirer langsung (bawaan). gapura = Gapura Custom Checkout, hanya QRIS; wajib mengisi MCC, Callback URL, dan Return URL", Options: []string{"core", "gapura"}},
		{Key: "mcc", Label: "MCC (Gapura)", Hint: "Kode kategori merchant, mis. 5814. Hanya untuk jalur gapura", pattern: regexp.MustCompile(`^[0-9]{4}$`)},
		{Key: "callback_url", Label: "Callback URL (Gapura)", Hint: "Alamat Finish Notify di host API, https. Hanya untuk jalur gapura", pattern: regexp.MustCompile(`^https://[^\s]{1,500}$`)},
		{Key: "return_url", Label: "Return URL (Gapura)", Hint: "PAY_RETURN, wajib di Create Order walau pembayar QRIS tidak dialihkan. Hanya untuk jalur gapura", pattern: regexp.MustCompile(`^https://[^\s]{1,500}$`)},
		{Key: "dana_public_key", Label: "Public key DANA (PEM)", Hint: "Memverifikasi notifikasi Finish Notify. Bukan public key Anda di dasbor. Sandbox: key di docs DANA (authentication-asymmetric); produksi diminta ke tim DANA. Kosong = notifikasi ditolak, status hanya lewat inquiry", Multiline: true, pem: true, b64: true},
	},
}, {
	Gateway: "ids",
	Label:   "IDS Payments",
	Fields: []Field{
		{Key: "base_url", Label: "Base URL", Hint: "Host API IDS, mis. https://payment.inodreamstudio.com", Required: true, pattern: regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:[0-9]{2,5})?$`)},
		{Key: "api_key", Label: "API key channel", Hint: "ids_<env>_<income_channel_id>_<secret>, tampil sekali di IDS", Required: true, Secret: true, pattern: regexp.MustCompile(`^ids_[A-Za-z0-9-]+_[A-Za-z0-9-]+_[\x21-\x7e]+$`)},
		{Key: "callback_secret", Label: "Callback secret", Hint: "Sama dengan callback_secret channel di IDS (min 8 karakter)", Required: true, Secret: true, pattern: regexp.MustCompile(`^[\x21-\x7e]{8,256}$`)},
	},
}, {
	Gateway: "doku",
	Label:   "DOKU",
	Fields: []Field{
		{Key: "client_id", Label: "Client ID", Required: true, pattern: regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)},
		{Key: "secret_key", Label: "Secret key", Required: true, Secret: true, pattern: regexp.MustCompile(`^[\x21-\x7e]{1,256}$`)},
		{Key: "private_key", Label: "Private key (PEM)", Hint: "RSA, format PKCS#8 atau PKCS#1", Required: true, Secret: true, Multiline: true, pem: true},
		{Key: "terminal_id", Label: "Terminal ID", Hint: "3-16 huruf/angka", Required: true, pattern: regexp.MustCompile(`^[A-Za-z0-9]{3,16}$`)},
		{Key: "postal_code", Label: "Kode pos", Required: true, pattern: regexp.MustCompile(`^[0-9]{5}$`)},
		{Key: "base_url", Label: "Base URL", Hint: "Kosong = bawaan sesuai environment", Options: []string{"https://api-sandbox.doku.com", "https://api.doku.com"}},
		{Key: "status_map_confirmed", Label: "Status QRIS sudah diverifikasi", Hint: "true setelah kode status QRIS dicek di sandbox", Options: []string{"true", "false"}},
	},
}}

// SchemaOf returns the schema for a gateway code.
func SchemaOf(gateway string) (Schema, bool) {
	for _, s := range schemas {
		if s.Gateway == gateway {
			return s, true
		}
	}
	return Schema{}, false
}

// Schemas lists every gateway whose credentials can be managed.
func Schemas() []Schema { return append([]Schema(nil), schemas...) }

func (s Schema) field(key string) (Field, bool) {
	for _, f := range s.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// check validates one value. The message is safe to show to the admin and never echoes the value.
func (f Field) check(v string) error {
	switch {
	case len(v) > 8192:
		return invalid("%s terlalu panjang", f.Label)
	case f.pem && f.b64 && !strings.Contains(v, "-----BEGIN"):
		if raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(v), "")); err != nil || len(raw) == 0 {
			return invalid("%s bukan PEM atau base64 yang valid", f.Label)
		}
		return nil
	case f.pem:
		if blk, _ := pem.Decode([]byte(f.normalize(v))); blk == nil {
			return invalid("%s bukan PEM yang valid", f.Label)
		}
		return nil
	case len(f.Options) > 0:
		for _, o := range f.Options {
			if v == o {
				return nil
			}
		}
		return invalid("%s harus salah satu dari: %s", f.Label, strings.Join(f.Options, ", "))
	case f.pattern != nil:
		if !f.pattern.MatchString(v) {
			return invalid("format %s tidak valid", f.Label)
		}
		return nil
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return invalid("%s mengandung karakter kontrol", f.Label)
		}
	}
	return nil
}

// normalize canonicalizes a value before it is stored (PEM pasted with literal \n).
func (f Field) normalize(v string) string {
	v = strings.TrimSpace(v)
	if f.pem && f.b64 && !strings.Contains(v, "-----BEGIN") {
		return v // bare base64: the adapter decodes it
	}
	if f.pem {
		v = strings.ReplaceAll(v, `\n`, "\n")
		// Keys copied from docs often carry a stray space before a line (DANA's END line): trim every line.
		lines := strings.Split(v, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimSpace(l)
		}
		v = strings.Join(lines, "\n")
	}
	return v
}

// preview is what the panel may show for a stored value: the value itself for ordinary fields,
// only a hint for secrets.
func (f Field) preview(v string) string {
	if !f.Secret {
		return v
	}
	if f.Multiline || len(v) < 12 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}
