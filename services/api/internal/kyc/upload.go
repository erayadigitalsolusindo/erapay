package kyc

import (
	"context"
	"errors"
	"io"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var docTypes = []string{"ktp", "ktp_back", "npwp_badan", "akta", "sk_kemenkumham", "nib", "izin_usaha"}

// ValidDocType: enam dokumen wajib (KTP menyatu dengan bagian perwakilan).
func ValidDocType(t string) bool {
	switch t {
	case "ktp", "ktp_back", "npwp_badan", "akta", "sk_kemenkumham", "nib", "izin_usaha":
		return true
	}
	return false
}

// maxBytesFor: Akta boleh 25 MB, lainnya 15 MB.
func maxBytesFor(docType string) int64 {
	if docType == "akta" {
		return MaxAktaBytes
	}
	return MaxFileBytes
}

// UploadInput: file dari multipart handler; MIME dicek dengan sniffing, bukan header client.
type UploadInput struct {
	MerchantID string
	ActorID    string
	DocType    string
	Number     string // nomor NPWP/NIB yang menyertai dokumen (opsional)
	KBLI       string // 5 digit, pasangan NIB
	Name       string // nama asli file
	Size       int64
	Reader     io.ReadSeekCloser // multipart.File; ReadSeekCloser agar sniffing bisa mundur
	IP         string
	RequestID  string
	// ConfirmSensitive wajib saat status approved dan dokumen legal diganti.
	ConfirmSensitive bool
}

// UploadDocument menyimpan satu dokumen wajib (satu jenis = satu file; unggah ulang menimpa)
// dan menyimpan nomor NPWP (tersegel) / NIB / KBLI yang menyertainya.
func (s *Service) UploadDocument(ctx context.Context, in UploadInput) (*Document, error) {
	if s.cfg.Storage == nil {
		return nil, ErrStorageMissing
	}
	if s.cfg.Box == nil {
		return nil, errors.New("kyc: ERAPAY_ENCRYPTION_KEY is not configured")
	}
	if !ValidDocType(in.DocType) {
		return nil, fieldErr(map[string]string{"doc_type": "Jenis dokumen tidak dikenal"})
	}
	if err := s.ensureEditable(ctx, in.MerchantID); err != nil {
		return nil, err
	}
	if err := s.throttleCheck(ctx, "upload", in.IP, s.cfg.UploadPerIP); err != nil {
		return nil, err
	}
	key, mime, size, sum, err := s.storeFile(ctx, in.MerchantID, in.Reader, maxBytesFor(in.DocType))
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	m, err := s.repo.merchantRow(ctx, in.MerchantID)
	if err != nil {
		return nil, err
	}
	// Nomor yang menyertai dokumen (server memvalidasi; NIK/NPWP disegel).
	if in.Number != "" || in.KBLI != "" {
		switch in.DocType {
		case "npwp_badan":
			d := digitsOnly(in.Number)
			if d == "" || !npwpRe.MatchString(d) {
				return nil, fieldErr(map[string]string{"npwp_number": "Nomor NPWP harus 15 atau 16 digit"})
			}
			enc, last4, serr := s.seal(d, in.MerchantID)
			if serr != nil {
				return nil, serr
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET business_npwp_enc=$2, business_npwp_last4=$3,
				updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, enc, last4); err != nil {
				return nil, err
			}
		case "nib":
			d := digitsOnly(in.Number)
			if d == "" || !nibRe.MatchString(d) {
				return nil, fieldErr(map[string]string{"nib_number": "Nomor NIB harus 13 digit"})
			}
			k := digitsOnly(in.KBLI)
			if k == "" || !kbliRe.MatchString(k) {
				return nil, fieldErr(map[string]string{"kbli": "KBLI harus 5 digit"})
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET business_nib=$2, business_kbli=$3,
				updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, d, k); err != nil {
				return nil, err
			}
		}
	}
	// Ganti dokumen lama (baris + file fisik setelah commit).
	oldKey := ""
	var old string
	err = tx.QueryRow(ctx, `DELETE FROM merchant_documents WHERE merchant_id=$1 AND doc_type=$2
		RETURNING storage_key`, in.MerchantID, in.DocType).Scan(&old)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// belum ada dokumen sebelumnya
	case err != nil:
		return nil, err
	default:
		oldKey = old
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_documents
		(merchant_id, doc_type, storage_key, original_name, mime, size_bytes, uploaded_by, sha256, enc_v)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, in.MerchantID, in.DocType, key, trimStr(in.Name, 200), mime, size, in.ActorID, sum, encVersion); err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "kyc.document_uploaded",
		EntityType: "merchant_kyc", EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"doc_type": in.DocType, "size": in.Size}}); err != nil {
		return nil, err
	}
	// APPROVED + dokumen legal diganti => NEEDS_REVIEW_AGAIN (konfirmasi wajib di UI).
	if m.Status == "approved" && (in.DocType == "akta" || in.DocType == "sk_kemenkumham" ||
		in.DocType == "npwp_badan" || in.DocType == "nib") {
		if !in.ConfirmSensitive {
			return nil, ErrSensitiveConfirm
		}
		if err := s.markRecheck(ctx, tx, in.MerchantID, "documents"); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if oldKey != "" {
		_ = s.cfg.Storage.Delete(ctx, oldKey)
	}
	return &Document{DocType: in.DocType, Name: trimStr(in.Name, 200), Mime: mime, Size: size}, nil
}
