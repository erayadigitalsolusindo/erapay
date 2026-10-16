package gwcred

import (
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
}

// Schema is the credential form of one gateway.
type Schema struct {
	Gateway string  `json:"gateway"`
	Label   string  `json:"label"`
	Fields  []Field `json:"fields"`
}

var schemas = []Schema{{
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
	case f.pem:
		v = strings.ReplaceAll(strings.TrimSpace(v), `\n`, "\n")
		if blk, _ := pem.Decode([]byte(v)); blk == nil {
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
	if f.pem {
		v = strings.ReplaceAll(v, `\n`, "\n")
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
