package kyc

import (
	"context"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// UnlockInput: admin membuka kunci data merchant yang sudah terverifikasi.
type UnlockInput struct {
	MerchantID string
	ActorID    string
	Note       string // alasan (wajib; dibaca merchant dan masuk audit)
	RequestID  string
}

// Unlock mengembalikan merchant approved -> action_required agar bisa memperbaiki data.
// Data terverifikasi terkunci bagi merchant; hanya admin yang bisa membukanya. Setiap bagian
// diberi catatan revisi (alasan admin) supaya wizard mengizinkan edit semua bagian, lalu
// merchant mengirim ulang untuk verifikasi.
func (s *Service) Unlock(ctx context.Context, in UnlockInput) error {
	in.Note = strings.TrimSpace(in.Note)
	if n := len([]rune(in.Note)); n < 3 || n > 500 {
		return fieldErr(map[string]string{"note": "Alasan wajib 3-500 karakter"})
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.lockIn(ctx, tx, in.MerchantID, []string{"approved"}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status='action_required', review_note=$3,
		reviewed_by=$2, reviewed_at=now(), updated_at=now(), version=version+1 WHERE id::text=$1`,
		in.MerchantID, in.ActorID, in.Note); err != nil {
		return err
	}
	if err := s.repo.ensureSubmission(ctx, tx, in.MerchantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE kyc_submissions SET recheck=false, recheck_sections='[]'::jsonb,
		updated_at=now() WHERE merchant_id=$1`, in.MerchantID); err != nil {
		return err
	}
	for _, sec := range []string{"company", "representative", "brand", "documents"} {
		if _, err := tx.Exec(ctx, `INSERT INTO kyc_review_notes (merchant_id, section, note, reviewer_id)
			VALUES ($1,$2,$3,$4)`, in.MerchantID, sec, "Data dibuka admin: "+in.Note, in.ActorID); err != nil {
			return err
		}
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "kyc.unlocked",
		EntityType: "merchant_kyc", EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"from": "approved", "to": "action_required", "note": in.Note}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
