// Package kyc implements the "Data Diri Wajib" (KYC isian, DOKU-style) form flow for
// merchant verification. The status is NOT duplicated here: merchants.onboarding_status
// (draft/submitted/under_review/action_required/approved/rejected) stays the single
// source of truth used by the existing approval gate (payments, payouts). This package
// keeps the per-section data, the completeness checklist, review notes, and the
// NEEDS_REVIEW_AGAIN marker (kyc_submissions.recheck) for when an approved merchant
// edits a sensitive field.
package kyc

import "time"

// Sections is every KYC part, in wizard order.
var Sections = []string{"company", "representative", "brand", "documents"}

// ValidSection reports whether s names a known KYC section.
func ValidSection(s string) bool {
	for _, x := range Sections {
		if x == s {
			return true
		}
	}
	return false
}

// SensitiveFields are the values whose change triggers NEEDS_REVIEW_AGAIN once approved.
var sensitiveFields = map[string]bool{
	"legal_name": true, "entity_type": true, "owner_id_number": true,
	"business_npwp": true, "business_nib": true,
	"doc:akta": true, "doc:sk_kemenkumham": true, "doc:npwp_badan": true, "doc:nib": true,
}

// Masked seals-opened values for the merchant's own view; full values are only
// filled by the reveal view (owner edit mode) and the reviewer detail view.
type Company struct {
	BusinessID    string `json:"business_id"`
	EntityType    string `json:"entity_type"`
	CompanyName   string `json:"company_name"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	PostalCode    string `json:"postal_code"`
	Kelurahan     string `json:"kelurahan"`
	Kecamatan     string `json:"kecamatan"`
	Kota          string `json:"kota"`
	Provinsi      string `json:"provinsi"`
	AddressDetail string `json:"address_detail"`
	Complete      bool   `json:"complete"`
	NeedsRevision bool   `json:"needs_revision"`
	Notes         []Note `json:"notes,omitempty"`
	Recheck       bool   `json:"recheck,omitempty"` // bagian ini memicu review ulang
}

type Representative struct {
	Name          string `json:"name"`
	Position      string `json:"position"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Nationality   string `json:"nationality"`
	IDMasked      string `json:"id_masked"`           // NIK ter-mask: "3374••••••••7890"
	IDNumber      string `json:"id_number,omitempty"` // hanya mode edit pemilik / reviewer
	KTP           *File  `json:"ktp,omitempty"`
	KTPBack       *File  `json:"ktp_back,omitempty"`
	Complete      bool   `json:"complete"`
	NeedsRevision bool   `json:"needs_revision"`
	Recheck       bool   `json:"recheck,omitempty"`
	Notes         []Note `json:"notes,omitempty"`
}

type Brand struct {
	BrandID       string     `json:"brand_id"`
	Name          string     `json:"name"`
	Category      string     `json:"category"`
	Description   string     `json:"description"`
	MonthlyAmount int64      `json:"monthly_amount"`
	MonthlyCount  int64      `json:"monthly_count"`
	Logo          *File      `json:"logo,omitempty"`
	Evidences     []Evidence `json:"evidences,omitempty"`
	Links         []Link     `json:"links,omitempty"`
	Complete      bool       `json:"complete"`
	NeedsRevision bool       `json:"needs_revision"`
	Notes         []Note     `json:"notes,omitempty"`
}

// File adalah referensi file yang bisa diunduh (thumbnail modal preview di UI).
type File struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

type Link struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Evidence struct {
	ID   string `json:"id"`
	URL  string `json:"url"` // /v1/kyc/evidences/{id}/file
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

type Document struct {
	ID         string `json:"id"`
	DocType    string `json:"doc_type"`
	Name       string `json:"original_name"`
	Mime       string `json:"mime"`
	Size       int64  `json:"size"`
	Number     string `json:"number,omitempty"`     // nomor NPWP/NIB terkait (mask bila disegel)
	NumberEnc  bool   `json:"number_enc,omitempty"` // nomor disegel (NPWP); hanya edit pemilik/reviewer
	UploadedAt string `json:"uploaded_at,omitempty"`
}

type Documents struct {
	NPWP          *Document `json:"npwp_badan,omitempty"`
	NPWPNumber    string    `json:"npwp_number,omitempty"` // ter-mask ("••••7890"); penuh hanya mode edit pemilik/reviewer
	Akta          *Document `json:"akta,omitempty"`
	SK            *Document `json:"sk_kemenkumham,omitempty"`
	NIBDoc        *Document `json:"nib,omitempty"`
	Izin          *Document `json:"izin_usaha,omitempty"`
	NIBNumber     string    `json:"nib_number,omitempty"`
	KBLI          string    `json:"kbli,omitempty"`
	Complete      bool      `json:"complete"`
	NeedsRevision bool      `json:"needs_revision"`
	Recheck       bool      `json:"recheck,omitempty"`
	Notes         []Note    `json:"notes,omitempty"`
}

type Note struct {
	ID       string `json:"id"`
	Section  string `json:"section"`
	Field    string `json:"field,omitempty"`
	Note     string `json:"note"`
	Resolved bool   `json:"resolved"`
}

type SectionCheck struct {
	Key           string `json:"section"`
	Label         string `json:"label"`
	Complete      bool   `json:"complete"`
	NeedsRevision bool   `json:"needs_revision"`
	Recheck       bool   `json:"recheck,omitempty"`
}

// Profile is the merchant's own KYC view (masked). Values opens the sealed
// NIK/NPWP for the owner's edit mode; it is nil in every other audience.
type Profile struct {
	Status         string         `json:"status"`  // merchants.onboarding_status
	Recheck        bool           `json:"recheck"` // true = NEEDS_REVIEW_AGAIN
	SubmittedAt    *time.Time     `json:"submitted_at,omitempty"`
	ReviewedAt     *time.Time     `json:"reviewed_at,omitempty"`
	ReviewNote     *string        `json:"review_note,omitempty"` // catatan penolakan global
	Editable       bool           `json:"editable"`              // boleh mengubah isian sekarang
	Locked         bool           `json:"locked"`                // APPROVED/SUSPENDED: baca saja
	Company        Company        `json:"company"`
	Representative Representative `json:"representative"`
	Brand          Brand          `json:"brand"`
	Documents      Documents      `json:"documents"`
	Checklist      []SectionCheck `json:"checklist"`
	Notes          []Note         `json:"notes"`
	RecheckPolicy  string         `json:"recheck_policy"` // open|closed (info UI)
}
