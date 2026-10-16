package kyc_test

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func newSvc(t *testing.T) (*kyc.Service, string, func(status string)) {
	t.Helper()
	db := testdb.New(t)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	svc := kyc.NewService(db, kyc.Config{Box: box, Storage: &kyc.LocalFS{Dir: t.TempDir()}})
	mid, _ := testdb.SeedMerchant(t, db, "Toko Uji")
	setStatus := func(s string) {
		if _, err := db.Exec(context.Background(),
			`UPDATE merchants SET onboarding_status=$2 WHERE id::text=$1`, mid, s); err != nil {
			t.Fatal(err)
		}
	}
	setStatus("draft")
	return svc, mid, setStatus
}

func companyIn() *kyc.CompanyInput {
	return &kyc.CompanyInput{EntityType: "PT", CompanyName: "PT Uji Coba", Phone: "81234567890",
		Email: "uji@example.com", PostalCode: "10110", Kelurahan: "Gambir", Kecamatan: "Gambir",
		Kota: "Jakarta Pusat", Provinsi: "DKI Jakarta", AddressDetail: "Jl. Merdeka 1"}
}

// Simpan tiap bagian wizard harus berhasil, tersimpan, dan aman diulang.
func TestSaveSectionAllSections(t *testing.T) {
	svc, mid, _ := newSvc(t)
	ctx := context.Background()
	base := kyc.SaveSectionInput{MerchantID: mid}

	in := base
	in.Section, in.Company = "company", companyIn()
	if err := svc.SaveSection(ctx, in); err != nil {
		t.Fatalf("company: %v", err)
	}
	in = base
	in.Section = "representative"
	in.Representative = &kyc.RepresentativeInput{Name: "Budi Santoso", Position: "Pemilik",
		Phone: "81234567890", Email: "budi@example.com", IDNumber: "3175070101901234"}
	if err := svc.SaveSection(ctx, in); err != nil {
		t.Fatalf("representative: %v", err)
	}
	in = base
	in.Section = "brand"
	in.Brand = &kyc.BrandInput{Name: "Toko Uji", Category: "Fashion", MonthlyAmount: 1000000,
		MonthlyCount: 10, Links: []string{"https://example.com/toko"}}
	for i := 0; i < 2; i++ { // simpan ulang = idempoten
		if err := svc.SaveSection(ctx, in); err != nil {
			t.Fatalf("brand #%d: %v", i, err)
		}
	}

	p, err := svc.GetProfile(ctx, mid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Company.CompanyName != "PT Uji Coba" {
		t.Errorf("company tidak tersimpan: %+v", p.Company)
	}
}

// Dua simpan bersamaan pada merchant baru tidak boleh menghasilkan error (race ensureSubmission/brand).
func TestSaveSectionConcurrent(t *testing.T) {
	svc, mid, _ := newSvc(t)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- svc.SaveSection(context.Background(), kyc.SaveSectionInput{
				MerchantID: mid, Section: "brand",
				Brand: &kyc.BrandInput{Name: "Toko Uji", Category: "Fashion"}})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("simpan bersamaan gagal: %v", err)
		}
	}
}

// submitted/under_review belum terverifikasi: isian masih boleh dilengkapi (hanya approved/suspended terkunci).
func TestSaveSectionAllowedWhenNotYetVerified(t *testing.T) {
	for _, st := range []string{"submitted", "under_review"} {
		svc, mid, setStatus := newSvc(t)
		setStatus(st)
		if err := svc.SaveSection(context.Background(), kyc.SaveSectionInput{
			MerchantID: mid, Section: "company", Company: companyIn()}); err != nil {
			t.Fatalf("%s: want save ok, got %v", st, err)
		}
	}
}

// Akun terverifikasi (approved) terkunci: merchant tidak bisa menyimpan lagi, hanya admin.
func TestSaveSectionLockedWhenApproved(t *testing.T) {
	svc, mid, setStatus := newSvc(t)
	setStatus("approved")
	err := svc.SaveSection(context.Background(), kyc.SaveSectionInput{
		MerchantID: mid, Section: "company", Company: companyIn()})
	if !errors.Is(err, kyc.ErrLocked) {
		t.Fatalf("want ErrLocked, got %v", err)
	}
}

// Entitas Perorangan harus lolos validasi DAN constraint DB (migrasi 0028), dan aman diulang.
func TestSaveCompanyPerorangan(t *testing.T) {
	svc, mid, _ := newSvc(t)
	ctx := context.Background()
	c := companyIn()
	c.EntityType = "Perorangan"
	for i := 0; i < 2; i++ {
		if err := svc.SaveSection(ctx, kyc.SaveSectionInput{MerchantID: mid, Section: "company", Company: c}); err != nil {
			t.Fatalf("simpan #%d: %v", i, err)
		}
	}
	p, err := svc.GetProfile(ctx, mid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Company.EntityType != "Perorangan" {
		t.Errorf("entity_type = %q", p.Company.EntityType)
	}
	if !p.Documents.Complete {
		t.Error("perorangan: bagian dokumen harus dianggap lengkap")
	}
}

// Brand ID muncul sejak pemilik pertama kali membuka tab Merek (sebelum simpan), stabil setelahnya, dan
// tidak dibuat oleh tampilan reviewer/terkunci.
func TestBrandIDAssignedOnFirstOpen(t *testing.T) {
	svc, mid, setStatus := newSvc(t)
	ctx := context.Background()

	if p, err := svc.GetProfile(ctx, mid); err != nil || p.Brand.BrandID != "" {
		t.Fatalf("tampilan biasa/reviewer tidak boleh membuat Brand ID: %v %q", err, p.Brand.BrandID)
	}
	p1, err := svc.GetProfileForEdit(ctx, mid)
	if err != nil || !strings.HasPrefix(p1.Brand.BrandID, "BRD-") {
		t.Fatalf("brand id saat pertama dibuka: %v %q", err, p1.Brand.BrandID)
	}
	p2, _ := svc.GetProfileForEdit(ctx, mid)
	if p2.Brand.BrandID != p1.Brand.BrandID {
		t.Fatalf("brand id harus stabil: %q vs %q", p1.Brand.BrandID, p2.Brand.BrandID)
	}
	if err := svc.SaveSection(ctx, kyc.SaveSectionInput{MerchantID: mid, Section: "brand",
		Brand: &kyc.BrandInput{Name: "Toko Uji", Category: "Fashion"}}); err != nil {
		t.Fatal(err)
	}
	if p3, _ := svc.GetProfileForEdit(ctx, mid); p3.Brand.BrandID != p1.Brand.BrandID {
		t.Fatalf("simpan tidak boleh mengganti brand id: %q", p3.Brand.BrandID)
	}

	// Akun terkunci tanpa brand: membuka tab tidak membuat baris baru.
	svc2, mid2, setStatus2 := newSvc(t)
	setStatus2("approved")
	if p, _ := svc2.GetProfileForEdit(ctx, mid2); p.Brand.BrandID != "" {
		t.Fatalf("terkunci tidak boleh membuat brand id: %q", p.Brand.BrandID)
	}
	_ = setStatus
}

// Pendaftar yang sudah berstatus submitted sejak daftar tapi belum pernah mengajukan lewat wizard harus bisa
// melanjutkan ke pengajuan (kesalahan = data belum lengkap, bukan status terkunci); setelah benar-benar
// diajukan, pengajuan kedua ditolak.
func TestSubmitFromSignupSubmittedUntilTrulySubmitted(t *testing.T) {
	db := testdb.New(t)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	svc := kyc.NewService(db, kyc.Config{Box: box, Storage: &kyc.LocalFS{Dir: t.TempDir()}})
	mid, _ := testdb.SeedMerchant(t, db, "Toko Uji")
	ctx := context.Background()
	if _, err := db.Exec(ctx, `UPDATE merchants SET onboarding_status='submitted' WHERE id::text=$1`, mid); err != nil {
		t.Fatal(err)
	}
	// Menyimpan bagian membuat baris kyc_submissions (belum diajukan).
	if err := svc.SaveSection(ctx, kyc.SaveSectionInput{MerchantID: mid, Section: "company", Company: companyIn()}); err != nil {
		t.Fatal(err)
	}
	var inc *kyc.IncompleteError
	if err := svc.Submit(ctx, kyc.SubmitInput{MerchantID: mid}); !errors.As(err, &inc) {
		t.Fatalf("submitted tanpa pengajuan harus lanjut ke cek kelengkapan, bukan terkunci: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE kyc_submissions SET submitted_at=now() WHERE merchant_id::text=$1`, mid); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(ctx, kyc.SubmitInput{MerchantID: mid}); !errors.Is(err, kyc.ErrInvalidSt) {
		t.Fatalf("sudah diajukan: want ErrInvalidSt, got %v", err)
	}
}
