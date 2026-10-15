package kyc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// genID menghasilkan ID unik server-side ("BIZ-XXXXXXXX" / "BRD-XXXXXXXX").
func genID(prefix string) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-00000000"
	}
	return prefix + "-" + strings.ToUpper(hex.EncodeToString(b[:]))
}

// NoteInput adalah satu catatan revisi per bagian/field dari reviewer.
type NoteInput struct {
	Section string `json:"section"`
	Field   string `json:"field"`
	Note    string `json:"note"`
}

// StartReview: submitted -> under_review lewat mekanisme onboarding yang sudah ada.
func (s *Service) StartReview(ctx context.Context, in ReviewInput) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.lockIn(ctx, tx, in.MerchantID, []string{"submitted"}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if s.reviewer != nil {
		if err := s.reviewer.StartReview(ctx, in); err != nil {
			return err
		}
	}
	return s.reviewed(ctx, in.MerchantID, in.ActorID)
}

// RequestChanges menandai NEEDS_REVISION: catatan per bagian/field wajib; status action_required.
func (s *Service) RequestChanges(ctx context.Context, in ReviewInput, notes []NoteInput) error {
	if len(notes) == 0 {
		return invalid("minimal satu catatan revisi per bagian/field diperlukan")
	}
	if s.reviewer != nil {
		if err := s.reviewer.RequestChanges(ctx, in); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, n := range notes {
		if !ValidSection(n.Section) {
			return invalid("bagian tidak dikenal: %q", n.Section)
		}
		n.Note = strings.TrimSpace(n.Note)
		if len([]rune(n.Note)) < 3 || len([]rune(n.Note)) > 500 {
			return fieldErr(map[string]string{"note": "Catatan revisi wajib 3-500 karakter"})
		}
		if _, err := tx.Exec(ctx, `INSERT INTO kyc_review_notes (merchant_id, section, field, note, reviewer_id)
			VALUES ($1,$2,NULLIF($3,''),$4,$5)`, in.MerchantID, n.Section, strings.TrimSpace(n.Field), n.Note, in.ActorID); err != nil {
			return err
		}
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "kyc.changes_requested",
		EntityType: "merchant_kyc", EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"to": "action_required", "notes": len(notes)}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Reject menolak pendaftaran dengan alasan wajib (mekanisme reject onboarding yang sudah ada).
func (s *Service) Reject(ctx context.Context, in RejectInput) error {
	if s.reviewer == nil {
		return errors.New("kyc: reviewer bridge is not configured")
	}
	if err := s.reviewer.Reject(ctx, in); err != nil {
		return err
	}
	return s.reviewed(ctx, in.MerchantID, in.ActorID)
}

// Approve menyetujui KYC. Transisi merchants.onboarding_status + penetapan fee tetap
// lewat onboarding.Approve (mekanisme yang sudah ada): email/identitas juga dicek di sana.
func (s *Service) Approve(ctx context.Context, in ApproveInput) error {
	m, err := s.repo.merchantRow(ctx, in.MerchantID)
	if err != nil {
		return err
	}
	if m.Status == "approved" {
		// NEEDS_REVIEW_AGAIN selesai: hapus penanda, produksi tetap sesuai kebijakan.
		return s.recheckDone(ctx, in)
	}
	if s.reviewer == nil {
		return fmt.Errorf("kyc: reviewer bridge is not configured")
	}
	if err := s.requireComplete(ctx, in.MerchantID); err != nil {
		return err
	}
	if err := s.reviewer.Approve(ctx, in); err != nil {
		return err
	}
	return s.reviewed(ctx, in.MerchantID, in.ActorID)
}

// reviewed mencatat reviewed_at/reviewer pada kyc_submissions (recheck=false).
func (s *Service) reviewed(ctx context.Context, mid, actor string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE kyc_submissions SET reviewed_at=now(), reviewer_id=$2, recheck=false,
		recheck_sections='[]'::jsonb, updated_at=now() WHERE merchant_id=$1`, mid, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// requireComplete menolak approve bila KYC belum lengkap (semua bagian + dokumen).
func (s *Service) requireComplete(ctx context.Context, merchantID string) error {
	m, err := s.repo.merchantRow(ctx, merchantID)
	if err != nil {
		return err
	}
	docs, err := s.repo.docs(ctx, s.db, merchantID)
	if err != nil {
		return err
	}
	b, bErr := s.repo.brandRow(ctx, s.db, merchantID)
	var evs []evidenceRow
	if errors.Is(bErr, ErrNotFound) {
		b = brandRow{}
	} else if bErr != nil {
		return bErr
	} else if evs, err = s.repo.evidences(ctx, s.db, b.ID); err != nil {
		return err
	}
	if fields := s.completeness(&m, docs, &b, evs); len(fields) > 0 {
		return &IncompleteError{Fields: fields}
	}
	return nil
}

// Approve untuk kasus re-review (merchant sudah approved): cukup menghapus penanda recheck.
func (s *Service) recheckDone(ctx context.Context, in ApproveInput) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE kyc_submissions SET recheck=false, recheck_sections='[]'::jsonb,
		reviewed_at=now(), reviewer_id=$2, updated_at=now() WHERE merchant_id=$1`, in.MerchantID, in.ActorID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "kyc.recheck_cleared",
		EntityType: "merchant_kyc", EntityID: in.MerchantID, RequestID: in.RequestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
