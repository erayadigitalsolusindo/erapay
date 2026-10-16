package kyc_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type rsc struct{ *bytes.Reader }

func (rsc) Close() error { return nil }

const marker = "RAHASIA-KTP-ISI-BERKAS-12345"

func pdfBytes() []byte {
	return []byte("%PDF-1.4\n% " + marker + "\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")
}

func readAll(t *testing.T, f io.ReadSeekCloser) []byte {
	t.Helper()
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// dirContains melaporkan apakah ada berkas di bawah dir yang memuat s (untuk memastikan tidak ada plaintext di disk).
func dirContains(t *testing.T, dir, s string) bool {
	t.Helper()
	found := false
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if b, e := os.ReadFile(p); e == nil && strings.Contains(string(b), s) {
			found = true
		}
		return nil
	})
	return found
}

func TestKYCFilesEncryptedAtRest(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, _ := secretbox.New(key)
	dir := t.TempDir()
	svc := kyc.NewService(db, kyc.Config{Box: box, Storage: &kyc.LocalFS{Dir: dir}})
	midA, _ := testdb.SeedMerchant(t, db, "Toko A")
	midB, _ := testdb.SeedMerchant(t, db, "Toko B")
	if _, err := db.Exec(ctx, `UPDATE merchants SET onboarding_status='draft'`); err != nil { // approved = terkunci
		t.Fatal(err)
	}
	var actor string
	if err := db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('kyc@x.test','x') RETURNING id::text`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	upload := func(mid string) string {
		t.Helper()
		if _, err := svc.UploadDocument(ctx, kyc.UploadInput{MerchantID: mid, ActorID: actor, DocType: "akta", Name: "akta.pdf",
			Reader: rsc{bytes.NewReader(pdfBytes())}}); err != nil {
			t.Fatal(err)
		}
		var id string
		if err := db.QueryRow(ctx, `SELECT id::text FROM merchant_documents WHERE merchant_id::text=$1 AND doc_type='akta'`, mid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	docA := upload(midA)
	docB := upload(midB)

	// 1. Di disk tidak ada plaintext; DB mencatat enc_v=1 dan SHA-256 isi asli.
	if dirContains(t, dir, marker) || dirContains(t, dir, "%PDF-") {
		t.Fatal("berkas di disk tidak boleh berisi plaintext")
	}
	var encV int
	var sum, keyA string
	if err := db.QueryRow(ctx, `SELECT enc_v, sha256, storage_key FROM merchant_documents WHERE id::text=$1`, docA).Scan(&encV, &sum, &keyA); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(pdfBytes())
	if encV != 1 || sum != hex.EncodeToString(want[:]) {
		t.Fatalf("enc_v=%d sha256=%s", encV, sum)
	}

	// 2. Pemilik membaca kembali plaintext yang persis sama; merchant lain ditolak.
	d, f, err := svc.GetDocumentFile(ctx, docA, midA, false)
	if err != nil || !bytes.Equal(readAll(t, f), pdfBytes()) || d.MerchantID != midA {
		t.Fatalf("baca pemilik: %v", err)
	}
	if _, _, err := svc.GetDocumentFile(ctx, docA, midB, false); !errors.Is(err, kyc.ErrForbidden) {
		t.Fatalf("merchant lain: %v", err)
	}
	if _, f, err := svc.GetDocumentFile(ctx, docA, "", true); err != nil || !bytes.Equal(readAll(t, f), pdfBytes()) {
		t.Fatalf("baca reviewer: %v", err)
	}

	// 3. Berkas terenkripsi milik A yang dipasang ke baris B tidak bisa dibuka (AAD terikat ke merchant).
	if _, err := db.Exec(ctx, `UPDATE merchant_documents SET storage_key=$2, sha256=$3 WHERE id::text=$1`, docB, keyA, sum); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.GetDocumentFile(ctx, docB, midB, false); !errors.Is(err, kyc.ErrFileCorrupt) {
		t.Fatalf("berkas tertukar harus ditolak: %v", err)
	}

	// 4. Berkas diubah di disk (tamper) terdeteksi.
	p := filepath.Join(dir, filepath.FromSlash(keyA))
	raw, _ := os.ReadFile(p)
	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.GetDocumentFile(ctx, docA, midA, false); !errors.Is(err, kyc.ErrFileCorrupt) {
		t.Fatalf("tamper harus terdeteksi: %v", err)
	}

	// 5. Log akses tercatat dan tidak memuat isi berkas.
	if err := svc.RecordFileAccess(ctx, actor, midA, "document", docA, true, "req-1"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='kyc.file.accessed' AND entity_id=$1 AND metadata->>'reviewer'='true'`, midA).Scan(&n)
	if n != 1 {
		t.Fatalf("audit akses: %d", n)
	}
}

// Berkas lama (enc_v=0) tetap terbaca sampai dimigrasi; migrasi mengenkripsi, mengganti key, menghapus berkas
// polos, dan aman diulang.
func TestEncryptLegacyFiles(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	dir := t.TempDir()
	st := &kyc.LocalFS{Dir: dir}
	svc := kyc.NewService(db, kyc.Config{Box: box, Storage: st})
	mid, _ := testdb.SeedMerchant(t, db, "Toko Lama")

	oldKey, err := st.Put(ctx, "kyc/"+mid, "pdf", bytes.NewReader(pdfBytes()))
	if err != nil {
		t.Fatal(err)
	}
	var docID string
	if err := db.QueryRow(ctx, `INSERT INTO merchant_documents (merchant_id, doc_type, storage_key, original_name, mime, size_bytes)
		VALUES ($1,'nib',$2,'nib.pdf','application/pdf',$3) RETURNING id::text`, mid, oldKey, len(pdfBytes())).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	if !dirContains(t, dir, marker) {
		t.Fatal("prasyarat: berkas lama harus polos")
	}
	if _, f, err := svc.GetDocumentFile(ctx, docID, mid, false); err != nil || !bytes.Equal(readAll(t, f), pdfBytes()) {
		t.Fatalf("berkas lama harus tetap terbaca: %v", err)
	}

	res, err := svc.EncryptLegacyFiles(ctx, nil)
	if err != nil || res.Encrypted != 1 || res.Failed != 0 {
		t.Fatalf("migrasi: %+v %v", res, err)
	}
	if dirContains(t, dir, marker) {
		t.Fatal("setelah migrasi tidak boleh ada plaintext di disk")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(oldKey))); !os.IsNotExist(err) {
		t.Fatalf("berkas lama harus dihapus: %v", err)
	}
	if _, f, err := svc.GetDocumentFile(ctx, docID, mid, false); err != nil || !bytes.Equal(readAll(t, f), pdfBytes()) {
		t.Fatalf("baca setelah migrasi: %v", err)
	}
	if res, err := svc.EncryptLegacyFiles(ctx, nil); err != nil || res.Encrypted != 0 || res.Failed != 0 {
		t.Fatalf("ulang harus no-op: %+v %v", res, err)
	}

	// Tanpa kunci enkripsi: menolak (tidak pernah menyimpan/membaca berkas tanpa kunci).
	nokey := kyc.NewService(db, kyc.Config{Storage: st})
	if _, err := nokey.EncryptLegacyFiles(ctx, nil); !errors.Is(err, kyc.ErrNoBox) {
		t.Fatalf("tanpa kunci: %v", err)
	}
}
