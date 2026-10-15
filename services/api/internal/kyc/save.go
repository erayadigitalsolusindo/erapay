package kyc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/throttle"
)

const throttleWindow = time.Hour

// throttleCheck memakai tabel register_attempts yang sudah ada dengan kind khusus KYC.
func (s *Service) throttleCheck(ctx context.Context, kind, ip string, perIP int) error {
	if _, err := throttle.Reserve(ctx, s.db, "kyc_"+kind, ip, "", throttle.Limits{PerIP: perIP, Window: throttleWindow}); err != nil {
		if errors.Is(err, throttle.ErrIPLimited) {
			return ErrRateLimit
		}
		return err
	}
	return nil
}

// editableStatus: boleh mengubah isian sekarang (SUBMITTED/IN_REVIEW = terkunci).
func editableStatus(status string) bool {
	return status == "draft" || status == "action_required" || status == "approved"
}

// SaveSection menyimpan draft per bagian (partial save) dengan aturan edit per status.
func (s *Service) SaveSection(ctx context.Context, in SaveSectionInput) error {
	if !ValidSection(in.Section) || in.Section == "documents" {
		return invalid("gunakan endpoint unggah dokumen untuk bagian ini")
	}
	if s.cfg.Box == nil {
		return errors.New("kyc: ERAPAY_ENCRYPTION_KEY is not configured")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	m, err := s.repo.merchantRow(ctx, in.MerchantID)
	if err != nil {
		return err
	}
	if err := s.repo.ensureSubmission(ctx, tx, in.MerchantID); err != nil {
		return err
	}
	notes, err := s.repo.notes(ctx, tx, in.MerchantID)
	if err != nil {
		return err
	}
	switch m.Status {
	case "submitted", "under_review", "rejected", "suspended":
		return ErrLocked
	}

	var sensitive []string
	switch in.Section {
	case "company":
		if in.Company == nil {
			return fieldErr(map[string]string{"company": "Isian Perusahaan wajib diisi"})
		}
		if f := validateCompany(in.Company); len(f) > 0 {
			return fieldErr(f)
		}
		if m.Status == "action_required" && !sectionMarked(notes, "company") && companyComplete(&m) {
			return ErrLocked
		}
		sensitive = s.saveCompany(ctx, tx, in, &m)
	case "representative":
		if in.Representative == nil {
			return fieldErr(map[string]string{"representative": "Isian perwakilan wajib diisi"})
		}
		if f := validateRepresentative(in.Representative); len(f) > 0 {
			return fieldErr(f)
		}
		if m.Status == "action_required" && !sectionMarked(notes, "representative") {
			return ErrLocked
		}
		if sensitive, err = s.saveRepresentative(ctx, tx, in, m); err != nil {
			return err
		}
	case "brand":
		if in.Brand == nil {
			return fieldErr(map[string]string{"brand": "Isian merek wajib diisi"})
		}
		if f := validateBrand(in.Brand); len(f) > 0 {
			return fieldErr(f)
		}
		if m.Status == "action_required" && !sectionMarked(notes, "brand") {
			return ErrLocked
		}
		if err := s.saveBrand(ctx, tx, in); err != nil {
			return err
		}
	}

	// APPROVED: perubahan field sensitif memicu NEEDS_REVIEW_AGAIN (wajib konfirmasi UI).
	if m.Status == "approved" && len(sensitive) > 0 {
		if !in.ConfirmSensitive {
			return ErrSensitiveConfirm
		}
		if err := s.markRecheck(ctx, tx, in.MerchantID, in.Section); err != nil {
			return err
		}
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "kyc.draft_saved",
		EntityType: "merchant_kyc", EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"section": in.Section, "sensitive": sensitive}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// markRecheck menandai NEEDS_REVIEW_AGAIN (APPROVED + field sensitif berubah).
func (s *Service) markRecheck(ctx context.Context, tx pgx.Tx, merchantID, section string) error {
	if _, err := tx.Exec(ctx, `UPDATE kyc_submissions SET recheck=true,
		recheck_sections=(SELECT COALESCE(jsonb_agg(DISTINCT x), '[]'::jsonb)
			FROM jsonb_array_elements(recheck_sections || to_jsonb($2::text)) x),
		updated_at=now() WHERE merchant_id=$1`, merchantID, section); err != nil {
		return err
	}
	return nil
}

// saveCompany menulis bagian Perusahaan; mengembalikan field sensitif yang berubah.
func (s *Service) saveCompany(ctx context.Context, tx pgx.Tx, in SaveSectionInput, m *rawMerchant) []string {
	c := in.Company
	if _, err := tx.Exec(ctx, `UPDATE merchants SET entity_type=$2, legal_name=$3,
		display_name = CASE WHEN display_name = legal_name THEN $3 ELSE display_name END,
		contact_phone=$4, contact_email=$5, addr_postal_code=$6, addr_kelurahan=$7, addr_kecamatan=$8,
		addr_kota=$9, addr_provinsi=$10, addr_detail=$11, updated_at=now(), version=version+1
		WHERE id=$1`, in.MerchantID, c.EntityType, c.CompanyName, c.Phone, c.Email,
		c.PostalCode, c.Kelurahan, c.Kecamatan, c.Kota, c.Provinsi, c.AddressDetail); err != nil {
		return nil
	}
	var sensitive []string
	if val(m.EntityType) != c.EntityType {
		sensitive = append(sensitive, "entity_type")
	}
	if c.CompanyName != m.CompanyName {
		sensitive = append(sensitive, "legal_name")
	}
	return sensitive
}

// saveRepresentative menulis bagian Perwakilan; NIK baru disegel (AES-GCM, AAD = merchant id).
func (s *Service) saveRepresentative(ctx context.Context, tx pgx.Tx, in SaveSectionInput, m rawMerchant) (sensitive []string, err error) {
	r := in.Representative
	if r.IDNumber != "" {
		idEnc, last4, err := s.seal(r.IDNumber, in.MerchantID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE merchants SET owner_name=$2, owner_position=$3, owner_phone=$4,
			owner_email=$5, owner_nationality=$6, owner_id_enc=$7, owner_id_last4=$8,
			updated_at=now(), version=version+1 WHERE id=$1`,
			in.MerchantID, r.Name, r.Position, r.Phone, r.Email, r.Nationality, idEnc, last4); err != nil {
			return nil, err
		}
		// NIK dianggap berubah bila 4 digit terakhirnya berbeda dari yang tersimpan.
		if m.IDLast4 != nil && last4 != "" && *m.IDLast4 != last4 {
			sensitive = append(sensitive, "owner_id_number")
		}
		return sensitive, nil
	}
	_, err = tx.Exec(ctx, `UPDATE merchants SET owner_name=$2, owner_position=$3, owner_phone=$4,
		owner_email=$5, owner_nationality=$6, updated_at=now(), version=version+1 WHERE id=$1`,
		in.MerchantID, r.Name, r.Position, r.Phone, r.Email, r.Nationality)
	return nil, err
}

// saveBrand menulis bagian Merek (membuat baris + Brand ID saat pertama kali) dan menulis ulang link.
func (s *Service) saveBrand(ctx context.Context, tx pgx.Tx, in SaveSectionInput) error {
	b, err := s.repo.brandRow(ctx, tx, in.MerchantID)
	if errors.Is(err, ErrNotFound) {
		if _, err := tx.Exec(ctx, `INSERT INTO merchant_brands (merchant_id, brand_id) VALUES ($1,$2)`,
			in.MerchantID, genID("BRD")); err != nil {
			return err
		}
		if b, err = s.repo.brandRow(ctx, tx, in.MerchantID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	in2 := *in.Brand
	if _, err := tx.Exec(ctx, `UPDATE merchant_brands SET name=$2, category=$3, description=$4,
		monthly_amount=$5, monthly_count=$6, updated_at=now() WHERE merchant_id=$1`,
		in.MerchantID, in2.Name, in2.Category, in2.Description, in2.MonthlyAmount, in2.MonthlyCount); err != nil {
		return err
	}
	// Link jualan online ditulis ulang sesuai urutan form.
	if _, err := tx.Exec(ctx, `DELETE FROM merchant_brand_links WHERE brand_id=$1`, b.ID); err != nil {
		return err
	}
	for _, l := range in2.Links {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO merchant_brand_links (brand_id, url) VALUES ($1,$2)`,
			b.ID, strings.TrimSpace(l)); err != nil {
			return err
		}
	}
	return nil
}

// SubmitInput: identitas selalu dari sesi; IP untuk throttle.
type SubmitInput struct {
	MerchantID string
	ActorID    string
	IP         string
	RequestID  string
}

// Submit memvalidasi kelengkapan SEMUA bagian lalu mengirim untuk verifikasi.
func (s *Service) Submit(ctx context.Context, in SubmitInput) error {
	if err := s.throttleCheck(ctx, "submit", in.IP, s.cfg.SubmitPerIP); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.lockIn(ctx, tx, in.MerchantID, []string{"draft", "action_required"}); err != nil {
		return err
	}
	if err := s.repo.ensureSubmission(ctx, tx, in.MerchantID); err != nil {
		return err
	}
	m, err := s.repo.merchantRow(ctx, in.MerchantID)
	if err != nil {
		return err
	}
	docs, err := s.repo.docs(ctx, tx, in.MerchantID)
	if err != nil {
		return err
	}
	b, bErr := s.repo.brandRow(ctx, tx, in.MerchantID)
	var evs []evidenceRow
	if errors.Is(bErr, ErrNotFound) {
		b = brandRow{}
	} else if bErr != nil {
		return bErr
	} else if evs, err = s.repo.evidences(ctx, tx, b.ID); err != nil {
		return err
	}
	if fields := s.completeness(&m, docs, &b, evs); len(fields) > 0 {
		return &IncompleteError{Fields: fields}
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status='submitted',
		updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE kyc_submissions SET submitted_at=now(), updated_at=now()
		WHERE merchant_id=$1`, in.MerchantID); err != nil {
		return err
	}
	// Catatan revisi lama dianggap selesai setelah merchant koreksi dan submit ulang.
	if _, err := tx.Exec(ctx, `UPDATE kyc_review_notes SET resolved=true, resolved_at=now()
		WHERE merchant_id=$1 AND resolved=false`, in.MerchantID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "kyc.submitted",
		EntityType: "merchant_kyc", EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"from": m.Status, "to": "submitted"}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// lockIn mengunci baris merchant dan menolak bila status di luar daftar (transisi invalid ditolak).
func (s *Service) lockIn(ctx context.Context, tx pgx.Tx, id string, allowed []string) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id::text = $1 FOR NO KEY UPDATE`, id).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	for _, ok := range allowed {
		if status == ok {
			return nil
		}
	}
	return ErrInvalidSt
}
