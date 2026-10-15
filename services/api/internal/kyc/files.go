package kyc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var mimeRe = regexp.MustCompile(`^application/pdf$|^image/jpe?g$|^image/png$`)

func allowedMime(m string) bool { return mimeRe.MatchString(m) }

// counter menghitung byte yang melewati reader (batas ukuran nyata, bukan header client).
type counter struct {
	r io.Reader
	n int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// storeFile: sniffing 512 byte pertama (bukan percaya header), batas ukuran, lalu simpan.
// r harus ReadSeekCloser (multipart.File memenuhinya) agar bisa mundur setelah sniffing.
func (s *Service) storeFile(ctx context.Context, prefix string, r io.ReadSeekCloser, max int64) (key, mime string, size int64, err error) {
	head := make([]byte, 512)
	n, _ := io.ReadFull(r, head)
	if n == 0 {
		return "", "", 0, fieldErr(map[string]string{"file": "Berkas kosong"})
	}
	mime = http.DetectContentType(head[:n])
	if !allowedMime(mime) {
		return "", "", 0, ErrBadFile
	}
	if _, err := r.Seek(-int64(n), io.SeekCurrent); err != nil {
		return "", "", 0, err
	}
	c := &counter{r: io.LimitReader(r, max+1)}
	if key, err = s.cfg.Storage.Put(ctx, prefix, extByMime[mime], c); err != nil {
		return "", "", 0, err
	}
	if c.n > max {
		_ = s.cfg.Storage.Delete(ctx, key)
		return "", "", 0, ErrTooLarge
	}
	if c.n == 0 {
		_ = s.cfg.Storage.Delete(ctx, key)
		return "", "", 0, fieldErr(map[string]string{"file": "Berkas kosong"})
	}
	return key, mime, c.n, nil
}

// GetDocumentFile membuka file dokumen. Hanya pemilik merchant atau reviewer yang boleh.
func (s *Service) GetDocumentFile(ctx context.Context, docID, viewerMerchantID string, isReviewer bool) (docRow, io.ReadSeekCloser, error) {
	var d docRow
	var mid string
	err := s.db.QueryRow(ctx, `SELECT id::text, doc_type, storage_key, original_name, mime, size_bytes,
		created_at, merchant_id::text FROM merchant_documents WHERE id::text = $1`,
		docID).Scan(&d.ID, &d.DocType, &d.Key, &d.Name, &d.Mime, &d.Size, &d.At, &mid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return d, nil, ErrNotFound
		}
		return d, nil, err
	}
	if !isReviewer && (viewerMerchantID == "" || mid != viewerMerchantID) {
		return d, nil, ErrForbidden
	}
	f, err := s.cfg.Storage.Open(ctx, d.Key)
	if err != nil {
		return d, nil, err
	}
	return d, f, nil
}

// GetEvidenceFile membuka satu bukti bisnis (pemilik atau reviewer).
func (s *Service) GetEvidenceFile(ctx context.Context, evID, viewerMerchantID string, isReviewer bool) (evidenceRow, io.ReadSeekCloser, error) {
	var e evidenceRow
	var mid string
	err := s.db.QueryRow(ctx, `SELECT v.id::text, v.storage_key, v.original_name, v.mime, v.size_bytes,
		b.merchant_id::text FROM merchant_brand_evidences v JOIN merchant_brands b ON b.id = v.brand_id
		WHERE v.id::text = $1`, evID).Scan(&e.ID, &e.Key, &e.Name, &e.Mime, &e.Size, &mid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return e, nil, ErrNotFound
		}
		return e, nil, err
	}
	if !isReviewer && (viewerMerchantID == "" || mid != viewerMerchantID) {
		return e, nil, ErrForbidden
	}
	f, err := s.cfg.Storage.Open(ctx, e.Key)
	if err != nil {
		return e, nil, err
	}
	return e, f, nil
}

// GetLogoFile membuka logo merek (pemilik atau reviewer). Reviewer menyebut merchantID target.
func (s *Service) GetLogoFile(ctx context.Context, merchantID string) (name, mime string, f io.ReadSeekCloser, err error) {
	b, err := s.repo.brandRow(ctx, s.db, merchantID)
	if err != nil {
		return "", "", nil, err
	}
	if b.LogoKey == nil || *b.LogoKey == "" {
		return "", "", nil, ErrNotFound
	}
	f, err = s.cfg.Storage.Open(ctx, *b.LogoKey)
	if err != nil {
		return "", "", nil, err
	}
	return val(b.LogoName), val(b.LogoMime), f, nil
}

// ensureBrand memastikan baris brand ada (dengan Brand ID server) dan mengembalikannya.
func (s *Service) ensureBrand(ctx context.Context, tx pgx.Tx, merchantID string) (brandRow, error) {
	b, err := s.repo.brandRow(ctx, tx, merchantID)
	if errors.Is(err, ErrNotFound) {
		if _, err := tx.Exec(ctx, `INSERT INTO merchant_brands (merchant_id, brand_id) VALUES ($1,$2)`,
			merchantID, genID("BRD")); err != nil {
			return b, err
		}
		return s.repo.brandRow(ctx, tx, merchantID)
	}
	return b, err
}

// AddEvidence menambah bukti bisnis (gambar saja, maks 5 MB).
func (s *Service) AddEvidence(ctx context.Context, in UploadInput) (*Evidence, error) {
	if s.cfg.Storage == nil {
		return nil, ErrStorageMissing
	}
	if err := s.throttleCheck(ctx, "upload", in.IP, s.cfg.UploadPerIP); err != nil {
		return nil, err
	}
	key, mime, size, err := s.storeFile(ctx, "kyc/"+in.MerchantID, in.Reader, MaxImageBytes)
	if err != nil {
		return nil, err
	}
	if mime == "application/pdf" {
		_ = s.cfg.Storage.Delete(ctx, key)
		return nil, ErrBadFile
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	b, err := s.ensureBrand(ctx, tx, in.MerchantID)
	if err != nil {
		return nil, err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO merchant_brand_evidences
		(brand_id, storage_key, original_name, mime, size_bytes, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		b.ID, key, trimStr(in.Name, 200), mime, size, in.ActorID).Scan(&id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Evidence{ID: id, URL: "/v1/kyc/evidences/" + id + "/file", Name: trimStr(in.Name, 200), Mime: mime, Size: size}, nil
}

// DeleteEvidence menghapus bukti bisnis milik merchant (baris + file).
func (s *Service) DeleteEvidence(ctx context.Context, merchantID, actorID, evidenceID, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var key string
	err = tx.QueryRow(ctx, `DELETE FROM merchant_brand_evidences v USING merchant_brands b
		WHERE b.id = v.brand_id AND v.id::text = $1 AND b.merchant_id::text = $2 RETURNING v.storage_key`,
		evidenceID, merchantID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "kyc.evidence_deleted",
		EntityType: "merchant_kyc", EntityID: merchantID, RequestID: requestID}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.cfg.Storage.Delete(ctx, key)
	return nil
}

// SetBrandLogo menyimpan logo merek (gambar, maks 5 MB).
func (s *Service) SetBrandLogo(ctx context.Context, in UploadInput) error {
	if s.cfg.Storage == nil {
		return ErrStorageMissing
	}
	if err := s.throttleCheck(ctx, "upload", in.IP, s.cfg.UploadPerIP); err != nil {
		return err
	}
	key, mime, size, err := s.storeFile(ctx, "kyc/"+in.MerchantID, in.Reader, MaxImageBytes)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	b, err := s.ensureBrand(ctx, tx, in.MerchantID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchant_brands SET logo_key=$2, logo_mime=$3, logo_name=$4,
		logo_size=$5, updated_at=now() WHERE merchant_id=$1`,
		in.MerchantID, key, mime, trimStr(in.Name, 200), size); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if b.LogoKey != nil && *b.LogoKey != "" {
		_ = s.cfg.Storage.Delete(ctx, *b.LogoKey)
	}
	return nil
}

// RegionSearch untuk autocomplete alamat (kode pos / kelurahan / kota).
func (s *Service) RegionSearch(ctx context.Context, q string, limit int) ([]Region, error) {
	q = strings.TrimSpace(q)
	if n := utf8.RuneCountInString(q); n < 2 {
		return []Region{}, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	return s.repo.Regions(ctx, strings.ToLower(q), limit)
}
