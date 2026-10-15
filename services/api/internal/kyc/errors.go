package kyc

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// InvalidError membawa pesan yang aman ditampilkan (mengikuti pola onboarding.ValidationError).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &InvalidError{Msg: fmt.Sprintf(format, a...)} }

// IncompleteError berisi field yang masih kurang saat submit.
type IncompleteError struct{ Fields map[string]string }

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("belum lengkap: %d isian", len(e.Fields))
}

// FieldError adalah pesan validasi per field (format konsisten
// { "error": { "code": "VALIDATION_ERROR", "fields": {...} } }).
type FieldError struct{ Fields map[string]string }

func (e *FieldError) Error() string { return "validation failed" }

var (
	ErrNotFound         = errors.New("kyc: not found")
	ErrLocked           = errors.New("kyc: data is locked for editing")
	ErrInvalidSt        = errors.New("kyc: transition is not valid for this status")
	ErrIncomplete       = errors.New("kyc: submission is not complete yet")
	ErrStorageMissing   = errors.New("kyc: storage is not configured")
	ErrRateLimit        = errors.New("kyc: too many requests")
	ErrForbidden        = errors.New("kyc: not the owner")
	ErrTooLarge         = errors.New("kyc: file is too large")
	ErrBadFile          = errors.New("kyc: file type is not allowed")
	ErrSensitiveConfirm = errors.New("kyc: changing a sensitive field needs explicit confirmation")
)

// fieldErr membentuk FieldError; handler menerjemahkannya ke 400 + daftar fields.
func fieldErr(fields map[string]string) error { return &FieldError{Fields: fields} }

var (
	phoneRe = regexp.MustCompile(`^8\d{8,11}$`) // setelah prefix 62
	nikRe   = regexp.MustCompile(`^\d{16}$`)
	npwpRe  = regexp.MustCompile(`^\d{15,16}$`)
	nibRe   = regexp.MustCompile(`^\d{13}$`)
	kbliRe  = regexp.MustCompile(`^\d{5}$`)
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	urlRe   = regexp.MustCompile(`^https?://[^\s]+$`)
)

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

func lower(s string) string { return strings.ToLower(s) }

// trimStr mengembalikan string ter-trim; "" jika melebihi max (pemanggil menolak kosong).
func trimStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return ""
	}
	return s
}

// normPhone menghasilkan nomor nasional 8xxxxxxxxx (tanpa +62/0) atau "" bila invalid.
func normPhone(raw string) string {
	d := digitsOnly(raw)
	d = strings.TrimPrefix(strings.TrimPrefix(d, "62"), "0")
	if !phoneRe.MatchString(d) {
		return ""
	}
	return d
}

// mask membentuk "3374••••••••0001" (semua kecuali 4 digit terakhir, total `total` digit).
func maskWith(last4 *string, total int) string {
	if last4 == nil || *last4 == "" {
		return ""
	}
	if total <= 4 {
		return "•" + *last4
	}
	return strings.Repeat("•", total-4) + *last4
}

// maskShort untuk view pendek "••••7890".
func maskShort(last4 *string) string {
	if last4 == nil || *last4 == "" {
		return ""
	}
	return "••••" + *last4
}

// sectionLabel untuk checklist UI.
func sectionLabel(section string) string {
	switch section {
	case "company":
		return "Perusahaan"
	case "representative":
		return "Perwakilan Bisnis"
	case "brand":
		return "Merek"
	case "documents":
		return "Dokumen"
	}
	return section
}
