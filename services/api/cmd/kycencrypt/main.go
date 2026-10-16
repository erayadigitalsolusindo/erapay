// Command kycencrypt mengenkripsi ulang berkas KYC lama (enc_v=0) yang masih tersimpan polos di disk.
// Berkas baru sudah otomatis terenkripsi (AES-256-GCM) saat diunggah; perintah ini hanya untuk berkas yang
// diunggah sebelum fitur enkripsi. Aman diulang (idempoten) dan kegagalan satu berkas tidak menghentikan yang lain.
//
//	kycencrypt           enkripsi semua berkas lama
//	kycencrypt -dry-run  hanya hitung berkas yang belum terenkripsi
//
// Env: ERAPAY_DATABASE_URL, ERAPAY_ENCRYPTION_KEY (kunci yang SAMA dengan API), ERAPAY_KYC_STORAGE_DIR.
// Lakukan backup folder KYC dan database sebelum menjalankannya. Tidak ada rahasia yang dicetak.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	dry := flag.Bool("dry-run", false, "hanya hitung berkas yang belum terenkripsi")
	flag.Parse()

	url, keyStr, dir := os.Getenv("ERAPAY_DATABASE_URL"), os.Getenv("ERAPAY_ENCRYPTION_KEY"), os.Getenv("ERAPAY_KYC_STORAGE_DIR")
	if url == "" || keyStr == "" || dir == "" {
		return fmt.Errorf("ERAPAY_DATABASE_URL, ERAPAY_ENCRYPTION_KEY and ERAPAY_KYC_STORAGE_DIR are required")
	}
	key, err := secretbox.ParseKey(keyStr)
	if err != nil {
		return err
	}
	box, err := secretbox.New(key)
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer db.Close()

	var docs, evs, logos int
	if err := db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM merchant_documents WHERE enc_v = 0),
		(SELECT count(*) FROM merchant_brand_evidences WHERE enc_v = 0),
		(SELECT count(*) FROM merchant_brands WHERE logo_key IS NOT NULL AND logo_key <> '' AND logo_enc_v = 0)`).
		Scan(&docs, &evs, &logos); err != nil {
		return err
	}
	fmt.Printf("belum terenkripsi: dokumen=%d bukti=%d logo=%d\n", docs, evs, logos)
	if *dry || docs+evs+logos == 0 {
		return nil
	}

	svc := kyc.NewService(db, kyc.Config{Box: box, Storage: &kyc.LocalFS{Dir: dir}})
	res, err := svc.EncryptLegacyFiles(ctx, func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) })
	fmt.Printf("terenkripsi=%d gagal=%d\n", res.Encrypted, res.Failed)
	if err != nil {
		return err
	}
	if res.Failed > 0 {
		return fmt.Errorf("%d berkas gagal; periksa pesan di atas lalu jalankan ulang", res.Failed)
	}
	return nil
}
