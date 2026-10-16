package kyc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// Enkripsi berkas KYC saat disimpan (at rest). Format enc_v=1: nonce||AES-256-GCM(isi), AAD = "kyc/<merchantID>"
// sehingga berkas yang disalin ke merchant lain tidak bisa dibuka. sha256 = hash isi asli (bukti keutuhan).
const encVersion int16 = 1

var (
	// ErrFileCorrupt: berkas gagal didekripsi atau checksum tidak cocok (rusak atau diubah di luar aplikasi).
	ErrFileCorrupt = errors.New("kyc: stored file is corrupt or was tampered with")
	// ErrNoBox: kunci enkripsi tidak diset; berkas tidak boleh disimpan/dibaca tanpa kunci.
	ErrNoBox = errors.New("kyc: ERAPAY_ENCRYPTION_KEY is not configured")
)

func fileAAD(merchantID string) []byte { return []byte("kyc/" + merchantID) }

// memFile membungkus isi yang sudah didekripsi agar memenuhi io.ReadSeekCloser (untuk http.ServeContent).
type memFile struct{ *bytes.Reader }

func (memFile) Close() error { return nil }

func sumHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// sealAndPut mengenkripsi isi lalu menyimpannya; mengembalikan key penyimpanan dan SHA-256 isi asli.
func (s *Service) sealAndPut(ctx context.Context, merchantID, ext string, plain []byte) (key, sum string, err error) {
	if s.cfg.Box == nil {
		return "", "", ErrNoBox
	}
	sealed, err := s.cfg.Box.Seal(plain, fileAAD(merchantID))
	if err != nil {
		return "", "", err
	}
	key, err = s.cfg.Storage.Put(ctx, "kyc/"+merchantID, ext, bytes.NewReader(sealed))
	if err != nil {
		return "", "", err
	}
	return key, sumHex(plain), nil
}

// openFile membuka berkas. enc_v=0 (berkas lama) dikembalikan apa adanya; enc_v=1 didekripsi dan diverifikasi
// terhadap checksum. Isi terdekripsi dipegang di memori (maksimum 25 MB, batas unggahan tertinggi).
func (s *Service) openFile(ctx context.Context, key string, v int16, sum *string, merchantID string) (io.ReadSeekCloser, error) {
	f, err := s.cfg.Storage.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	if v == 0 {
		return f, nil
	}
	defer f.Close()
	if s.cfg.Box == nil {
		return nil, ErrNoBox
	}
	sealed, err := io.ReadAll(io.LimitReader(f, MaxAktaBytes+1<<10))
	if err != nil {
		return nil, err
	}
	plain, err := s.cfg.Box.Open(sealed, fileAAD(merchantID))
	if err != nil {
		return nil, ErrFileCorrupt
	}
	if sum != nil && *sum != "" && sumHex(plain) != *sum {
		return nil, ErrFileCorrupt
	}
	return memFile{bytes.NewReader(plain)}, nil
}

// RecordFileAccess mencatat siapa membuka berkas KYC (audit append-only). Dipanggil sebelum berkas dikirim;
// gagal mencatat = berkas tidak dikirim.
func (s *Service) RecordFileAccess(ctx context.Context, actorID, merchantID, kind, fileID string, reviewer bool, requestID string) error {
	return audit.Record(ctx, s.db, audit.Entry{ActorID: actorID, Action: "kyc.file.accessed", EntityType: "merchant_kyc",
		EntityID: merchantID, RequestID: requestID,
		Metadata: map[string]any{"kind": kind, "file_id": fileID, "reviewer": reviewer}})
}

// LegacyResult adalah ringkasan EncryptLegacyFiles.
type LegacyResult struct{ Encrypted, Failed int }

type legacyRow struct {
	id, merchantID, key, mime string
}

// EncryptLegacyFiles mengenkripsi ulang semua berkas yang masih enc_v=0: baca berkas lama, simpan versi
// terenkripsi dengan key baru, perbarui baris (key, sha256, enc_v), lalu hapus berkas lama. Aman diulang:
// baris yang sudah enc_v=1 dilewati, dan kegagalan satu berkas tidak menghentikan yang lain.
func (s *Service) EncryptLegacyFiles(ctx context.Context, logf func(format string, args ...any)) (LegacyResult, error) {
	var res LegacyResult
	if s.cfg.Box == nil {
		return res, ErrNoBox
	}
	if s.cfg.Storage == nil {
		return res, ErrStorageMissing
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	sets := []struct {
		name, list, update string
	}{
		{"document",
			`SELECT id::text, merchant_id::text, storage_key, mime FROM merchant_documents WHERE enc_v = 0`,
			`UPDATE merchant_documents SET storage_key=$2, sha256=$3, enc_v=1 WHERE id::text=$1 AND enc_v=0`},
		{"evidence",
			`SELECT v.id::text, b.merchant_id::text, v.storage_key, v.mime FROM merchant_brand_evidences v
			   JOIN merchant_brands b ON b.id = v.brand_id WHERE v.enc_v = 0`,
			`UPDATE merchant_brand_evidences SET storage_key=$2, sha256=$3, enc_v=1 WHERE id::text=$1 AND enc_v=0`},
		{"logo",
			`SELECT id::text, merchant_id::text, logo_key, COALESCE(logo_mime,'') FROM merchant_brands
			   WHERE logo_key IS NOT NULL AND logo_key <> '' AND logo_enc_v = 0`,
			`UPDATE merchant_brands SET logo_key=$2, logo_sha256=$3, logo_enc_v=1 WHERE id::text=$1 AND logo_enc_v=0`},
	}
	for _, set := range sets {
		rows, err := s.db.Query(ctx, set.list)
		if err != nil {
			return res, fmt.Errorf("%s: %w", set.name, err)
		}
		var todo []legacyRow
		for rows.Next() {
			var r legacyRow
			if err := rows.Scan(&r.id, &r.merchantID, &r.key, &r.mime); err != nil {
				rows.Close()
				return res, err
			}
			todo = append(todo, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
		for _, r := range todo {
			if err := s.encryptOne(ctx, set.update, r); err != nil {
				res.Failed++
				logf("gagal %s %s: %v", set.name, r.id, err)
				continue
			}
			res.Encrypted++
		}
	}
	return res, nil
}

func (s *Service) encryptOne(ctx context.Context, updateSQL string, r legacyRow) error {
	f, err := s.cfg.Storage.Open(ctx, r.key)
	if err != nil {
		return err
	}
	plain, err := io.ReadAll(io.LimitReader(f, MaxAktaBytes+1))
	f.Close()
	if err != nil {
		return err
	}
	if len(plain) == 0 || int64(len(plain)) > MaxAktaBytes {
		return errors.New("ukuran berkas tidak wajar")
	}
	ext := extByMime[r.mime]
	if ext == "" {
		ext = "bin"
	}
	newKey, sum, err := s.sealAndPut(ctx, r.merchantID, ext, plain)
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, updateSQL, r.id, newKey, sum)
	if err != nil || tag.RowsAffected() != 1 {
		_ = s.cfg.Storage.Delete(ctx, newKey) // jangan tinggalkan berkas terenkripsi yatim
		if err == nil {
			err = errors.New("baris berubah saat migrasi; ulangi")
		}
		return err
	}
	_ = s.cfg.Storage.Delete(ctx, r.key)
	return nil
}
