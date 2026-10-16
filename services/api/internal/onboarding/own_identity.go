package onboarding

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// ownEditable: status yang masih boleh diubah pemilik. Selaras dengan aturan KYC: hanya approved
// (terverifikasi) dan suspended yang terkunci; perubahan setelah itu lewat admin.
var ownEditable = []string{"draft", "submitted", "under_review", "action_required", "rejected"}

// OwnIdentity adalah data identitas pemilik apa adanya (NIK/NPWP dibuka) untuk diedit pemiliknya sendiri.
type OwnIdentity struct {
	Identity    *Identity `json:"identity"`
	AccountType string    `json:"account_type"`
	Status      string    `json:"status"`
	Editable    bool      `json:"editable"`
}

// GetOwnIdentity membaca identitas pemilik dengan NIK/NPWP terbuka. Hanya dipanggil dengan merchant dari sesi.
func (s *Service) GetOwnIdentity(ctx context.Context, merchantID string) (OwnIdentity, error) {
	m, err := s.GetForReview(ctx, merchantID)
	if err != nil {
		return OwnIdentity{}, err
	}
	out := OwnIdentity{Identity: m.Identity, Status: m.Status}
	if m.AccountType != nil {
		out.AccountType = *m.AccountType
	}
	for _, st := range ownEditable {
		if m.Status == st {
			out.Editable = true
		}
	}
	return out, nil
}

// UpdateOwnIdentity menyimpan isian formulir sebagai nilai terbaru: kolom kosong = dikosongkan,
// kolom terisi divalidasi. Tidak ada kolom wajib di sini (data dilengkapi bertahap); kelengkapan dinilai
// saat pengajuan/persetujuan. Aturan akun bisnis mengikuti tipe akun merchant, bukan request.
func (s *Service) UpdateOwnIdentity(ctx context.Context, in SetIdentityInput) (OwnIdentity, error) {
	if s.cfg.Box == nil {
		return OwnIdentity{}, errors.New("onboarding: encryption key is not configured")
	}
	raw := in.Identity
	if raw == nil {
		return OwnIdentity{}, invalid("identity is required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OwnIdentity{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockIn(ctx, tx, in.MerchantID, ownEditable); err != nil {
		return OwnIdentity{}, err
	}
	var accountType *string
	if err := tx.QueryRow(ctx, `SELECT account_type FROM merchants WHERE id = $1`, in.MerchantID).Scan(&accountType); err != nil {
		return OwnIdentity{}, err
	}
	business := accountType != nil && *accountType == "business"

	nik := digitsOnly(raw.IDNumber)
	if nik != "" && !nikRe.MatchString(nik) {
		return OwnIdentity{}, invalid("id_number must be 16 digits")
	}
	place := strings.TrimSpace(raw.PlaceOfBirth)
	if place != "" {
		if n := utf8.RuneCountInString(place); n < 2 || n > 100 {
			return OwnIdentity{}, invalid("place_of_birth must be 2-100 characters")
		}
	}
	var dob *time.Time
	if d := strings.TrimSpace(raw.DateOfBirth); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return OwnIdentity{}, invalid("date_of_birth must be a valid date in YYYY-MM-DD form")
		}
		if age := ageYears(t, time.Now()); age < 17 || age > 120 {
			return OwnIdentity{}, invalid("date_of_birth must be between 17 and 120 years of age")
		}
		dob = &t
	}
	gender := strings.ToLower(strings.TrimSpace(raw.Gender))
	if gender != "" && gender != "male" && gender != "female" {
		return OwnIdentity{}, invalid("gender must be male or female")
	}
	address := strings.TrimSpace(raw.Address)
	if address != "" {
		if n := utf8.RuneCountInString(address); n < 5 || n > 255 {
			return OwnIdentity{}, invalid("address must be 5-255 characters")
		}
	}
	npwp := digitsOnly(raw.NPWP)
	if npwp != "" && !npwpRe.MatchString(npwp) {
		return OwnIdentity{}, invalid("npwp must be 15 or 16 digits")
	}
	bizNPWP, bizNIB := digitsOnly(raw.BusinessNPWP), digitsOnly(raw.BusinessNIB)
	if !business && (bizNPWP != "" || bizNIB != "") {
		return OwnIdentity{}, invalid("business_npwp and business_nib are only for business accounts")
	}
	if bizNPWP != "" && !npwpRe.MatchString(bizNPWP) {
		return OwnIdentity{}, invalid("business_npwp must be 15 or 16 digits")
	}
	if bizNIB != "" && !nibRe.MatchString(bizNIB) {
		return OwnIdentity{}, invalid("business_nib must be 13 digits")
	}

	idEnc, idLast4, err := s.sealNumber(nik, in.MerchantID)
	if err != nil {
		return OwnIdentity{}, err
	}
	npwpEnc, npwpLast4, err := s.sealNumber(npwp, in.MerchantID)
	if err != nil {
		return OwnIdentity{}, err
	}
	bizEnc, bizLast4, err := s.sealNumber(bizNPWP, in.MerchantID)
	if err != nil {
		return OwnIdentity{}, err
	}
	nullable := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET
		owner_id_enc=$2, owner_id_last4=$3, owner_place_of_birth=$4, owner_date_of_birth=$5,
		owner_gender=$6, owner_address=$7, owner_npwp_enc=$8, owner_npwp_last4=$9,
		business_npwp_enc=$10, business_npwp_last4=$11, business_nib=$12,
		updated_at=now(), version=version+1 WHERE id=$1`,
		in.MerchantID, idEnc, nullable(idLast4), nullable(place), dob, nullable(gender), nullable(address),
		npwpEnc, nullable(npwpLast4), bizEnc, nullable(bizLast4), nullable(bizNIB)); err != nil {
		return OwnIdentity{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.identity_updated", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID,
		// Tidak pernah plaintext: hanya ekor ter-mask, seperti event rekening bank.
		Metadata: map[string]any{"id_last4": idLast4, "source": "profile"}}); err != nil {
		return OwnIdentity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OwnIdentity{}, err
	}
	return s.GetOwnIdentity(ctx, in.MerchantID)
}
