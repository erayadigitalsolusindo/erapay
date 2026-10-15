package kyc

import (
	"context"
	"errors"
	"strings"
	"time"
)

// GetProfile menyusun seluruh isian + checklist + catatan revisi untuk pemilik (ter-mask).
func (s *Service) GetProfile(ctx context.Context, merchantID string) (Profile, error) {
	return s.profile(ctx, merchantID, false)
}

// GetProfileForEdit menambahkan nilai penuh NIK/NPWP untuk form edit pemilik.
func (s *Service) GetProfileForEdit(ctx context.Context, merchantID string) (Profile, error) {
	return s.profile(ctx, merchantID, true)
}

func (s *Service) profile(ctx context.Context, merchantID string, reveal bool) (Profile, error) {
	re := s.repo
	m, err := re.merchantRow(ctx, merchantID)
	if err != nil {
		return Profile{}, err
	}
	sub, err := s.loadSub(ctx, merchantID)
	if err != nil {
		return Profile{}, err
	}
	notes, err := re.notes(ctx, s.db, merchantID)
	if err != nil {
		return Profile{}, err
	}
	docs, err := re.docs(ctx, s.db, merchantID)
	if err != nil {
		return Profile{}, err
	}
	b, bErr := re.brandRow(ctx, s.db, merchantID)
	var evs []evidenceRow
	var lks []Link
	if bErr == nil {
		if evs, err = re.evidences(ctx, s.db, b.ID); err != nil {
			return Profile{}, err
		}
		if lks, err = re.links(ctx, s.db, b.ID); err != nil {
			return Profile{}, err
		}
	} else if !errors.Is(bErr, ErrNotFound) {
		return Profile{}, bErr
	}

	p := Profile{Status: m.Status, Recheck: sub.Recheck, SubmittedAt: sub.SubmittedAt,
		ReviewedAt: sub.ReviewedAt, ReviewNote: m.ReviewNote, RecheckPolicy: s.cfg.RecheckPolicy}
	p.Editable = editableStatus(m.Status)
	p.Locked = m.Status == "submitted" || m.Status == "under_review"

	p.Company = buildCompany(&m, notes)
	p.Company.Recheck = sub.rechecking("company")
	p.Representative = buildRepresentative(&m, docs, notes)
	p.Representative.Recheck = sub.rechecking("representative")
	p.Brand = buildBrand(&b, evs, lks, notes)
	p.Documents = buildDocuments(&m, docs, notes)
	p.Documents.Recheck = sub.rechecking("documents")
	p.Notes = notes
	if reveal {
		s.revealOwner(&p, &m, merchantID)
	}
	p.Checklist = buildChecklist(&p, sub)
	return p, nil
}

func (s *Service) loadSub(ctx context.Context, merchantID string) (submission, error) {
	if err := s.repo.ensureSubmission(ctx, s.db, merchantID); err != nil {
		return submission{}, err
	}
	return s.repo.submission(ctx, s.db, merchantID)
}

// ============================ HELPER BACA ============================

func notesFor(notes []Note, section string) []Note {
	out := []Note{}
	for _, n := range notes {
		if n.Section == section {
			out = append(out, n)
		}
	}
	return out
}

// sectionMarked: bagian ditandai reviewer dan belum diselesaikan.
func sectionMarked(notes []Note, section string) bool {
	for _, n := range notes {
		if n.Section == section && !n.Resolved {
			return true
		}
	}
	return false
}

func val(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func i64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// ============================ KEPENUHAN ============================

// companyComplete: semua field bagian Perusahaan wajib terisi.
func companyComplete(m *rawMerchant) bool {
	return val(m.EntityType) != "" && m.CompanyName != "" &&
		m.Phone != nil && *m.Phone != "" && val(m.Email) != "" &&
		val(m.PostalCode) != "" && val(m.Kelurahan) != "" && val(m.Kecamatan) != "" &&
		val(m.Kota) != "" && val(m.Provinsi) != "" && val(m.Detail) != ""
}

func representativeComplete(m *rawMerchant, docs []docRow) bool {
	hasKTP := false
	for _, d := range docs {
		if d.DocType == "ktp" {
			hasKTP = true
		}
	}
	return val(m.OwnerName) != "" && val(m.OwnerPosition) != "" &&
		val(m.OwnerPhone) != "" && val(m.OwnerEmail) != "" && len(m.idEnc) > 0 && hasKTP
}

func brandComplete(b *brandRow, evs []evidenceRow) bool {
	return b.ID != "" && val(b.Name) != "" && val(b.Category) != "" &&
		val(b.Description) != "" && b.Amount != nil && *b.Amount > 0 &&
		b.Count != nil && *b.Count > 0 && len(evs) > 0
}

func docsComplete(m *rawMerchant, docs []docRow) bool {
	have := map[string]bool{}
	for _, d := range docs {
		have[d.DocType] = true
	}
	for _, t := range []string{"ktp", "npwp_badan", "akta", "sk_kemenkumham", "nib", "izin_usaha"} {
		if !have[t] {
			return false
		}
	}
	// nomor NPWP badan (tersegel) + NIB + KBLI wajib
	return m.bizEnc != nil && val(m.NIB) != "" && val(m.KBLI) != ""
}

// ============================ REVEAL PEMILIK ============================

// revealOwner membuka NIK/NPWP tersegel untuk mode edit pemilik.
func (s *Service) revealOwner(p *Profile, m *rawMerchant, merchantID string) {
	if v, err := s.openSealed(m.idEnc, merchantID); err == nil {
		p.Representative.IDNumber = v
	}
	if v, err := s.openSealed(m.bizEnc, merchantID); err == nil {
		p.Documents.NPWPNumber = v
	}
	p.Documents.NIBNumber = val(m.NIB)
	p.Documents.KBLI = val(m.KBLI)
}

// ============================ BANGUN TAMPILAN ============================

// intlPhone shows a stored phone as +62…; the column holds either the national number
// ("81234567890") or the sign-up form "+6281234567890".
func intlPhone(stored string) string {
	return "+62" + strings.TrimPrefix(strings.TrimPrefix(stored, "+"), "62")
}

func buildCompany(m *rawMerchant, notes []Note) Company {
	c := Company{Notes: notesFor(notes, "company"), NeedsRevision: sectionMarked(notes, "company")}
	c.BusinessID = val(m.BusinessID)
	c.EntityType = val(m.EntityType)
	c.CompanyName = m.CompanyName
	if m.Phone != nil && *m.Phone != "" {
		c.Phone = intlPhone(*m.Phone)
	}
	c.Email = val(m.Email)
	c.PostalCode, c.Kelurahan, c.Kecamatan = val(m.PostalCode), val(m.Kelurahan), val(m.Kecamatan)
	c.Kota, c.Provinsi, c.AddressDetail = val(m.Kota), val(m.Provinsi), val(m.Detail)
	c.Complete = companyComplete(m)
	return c
}

func buildRepresentative(m *rawMerchant, docs []docRow, notes []Note) Representative {
	r := Representative{Notes: notesFor(notes, "representative"),
		NeedsRevision: sectionMarked(notes, "representative")}
	r.Name = val(m.OwnerName)
	if m.OwnerPosition != nil {
		r.Position = *m.OwnerPosition
	}
	if m.OwnerPhone != nil && *m.OwnerPhone != "" {
		r.Phone = intlPhone(*m.OwnerPhone)
	}
	r.Email = val(m.OwnerEmail)
	r.Nationality = "Indonesia"
	if m.OwnerNationality != nil && *m.OwnerNationality != "" {
		r.Nationality = *m.OwnerNationality
	}
	r.IDMasked = maskWith(m.IDLast4, 16)
	for _, d := range docs {
		if d.DocType == "ktp" {
			r.KTP = &File{ID: d.ID, URL: "/v1/kyc/documents/" + d.ID + "/file", Name: d.Name, Mime: d.Mime, Size: d.Size}
		}
	}
	r.Complete = representativeComplete(m, docs)
	return r
}

func buildBrand(b *brandRow, evs []evidenceRow, lks []Link, notes []Note) Brand {
	out := Brand{Evidences: []Evidence{}, Links: []Link{},
		Notes: notesFor(notes, "brand"), NeedsRevision: sectionMarked(notes, "brand")}
	if b.ID == "" {
		return out
	}
	out.BrandID = b.BrandID
	out.Name = val(b.Name)
	out.Category = val(b.Category)
	out.Description = val(b.Description)
	if b.Amount != nil {
		out.MonthlyAmount = *b.Amount
	}
	if b.Count != nil {
		out.MonthlyCount = *b.Count
	}
	if b.LogoKey != nil && *b.LogoKey != "" {
		out.Logo = &File{Name: val(b.LogoName), Mime: val(b.LogoMime), Size: i64(b.LogoSize)}
	}
	out.Evidences = make([]Evidence, 0, len(evs))
	for _, e := range evs {
		out.Evidences = append(out.Evidences, Evidence{ID: e.ID,
			URL: "/v1/kyc/evidences/" + e.ID + "/file", Name: e.Name, Mime: e.Mime, Size: e.Size})
	}
	out.Links = lks
	out.Complete = brandComplete(b, evs)
	return out
}

func fileDoc(f *docRow) *Document {
	return &Document{ID: f.ID, DocType: f.DocType, Name: f.Name, Mime: f.Mime, Size: f.Size,
		UploadedAt: f.At.Format(time.RFC3339)}
}

func buildDocuments(m *rawMerchant, docs []docRow, notes []Note) Documents {
	d := Documents{Notes: notesFor(notes, "documents"),
		NeedsRevision: sectionMarked(notes, "documents")}
	byType := map[string]*docRow{}
	for i := range docs {
		byType[docs[i].DocType] = &docs[i]
	}
	if f := byType["npwp_badan"]; f != nil {
		d.NPWP = &Document{ID: f.ID, DocType: "npwp_badan", Name: f.Name, Mime: f.Mime, Size: f.Size,
			UploadedAt: f.At.Format(time.RFC3339)}
	}
	if f := byType["akta"]; f != nil {
		d.Akta = fileDoc(f)
	}
	if f := byType["sk_kemenkumham"]; f != nil {
		d.SK = fileDoc(f)
	}
	if f := byType["nib"]; f != nil {
		d.NIBDoc = fileDoc(f)
	}
	if f := byType["izin_usaha"]; f != nil {
		d.Izin = fileDoc(f)
	}
	// Nomor NPWP badan disegel: tampil ter-mask; penuh hanya saat reveal (mode edit pemilik/reviewer).
	d.NPWPNumber = maskWith(m.BizLast4, 16)
	d.NIBNumber = val(m.NIB)
	d.KBLI = val(m.KBLI)
	d.Complete = docsComplete(m, docs)
	return d
}

func buildChecklist(p *Profile, sub submission) []SectionCheck {
	return []SectionCheck{
		{Key: "company", Label: "Perusahaan", Complete: p.Company.Complete,
			NeedsRevision: p.Company.NeedsRevision, Recheck: p.Company.Recheck},
		{Key: "representative", Label: "Perwakilan Bisnis", Complete: p.Representative.Complete,
			NeedsRevision: p.Representative.NeedsRevision, Recheck: p.Representative.Recheck},
		{Key: "brand", Label: "Merek", Complete: p.Brand.Complete,
			NeedsRevision: p.Brand.NeedsRevision, Recheck: sub.rechecking("brand")},
		{Key: "documents", Label: "Dokumen", Complete: p.Documents.Complete,
			NeedsRevision: p.Documents.NeedsRevision, Recheck: p.Documents.Recheck},
	}
}

// completeness daftar semua isian yang masih kurang (untuk submit; key bergaya "company.email").
func (s *Service) completeness(m *rawMerchant, docs []docRow, b *brandRow, evs []evidenceRow) map[string]string {
	f := map[string]string{}
	if val(m.EntityType) == "" {
		f["company.entity_type"] = "Jenis badan usaha wajib dipilih"
	}
	if m.CompanyName == "" {
		f["company.company_name"] = "Nama company wajib diisi"
	}
	if m.Phone == nil || *m.Phone == "" {
		f["company.phone"] = "Nomor telepon perusahaan wajib diisi"
	}
	if val(m.Email) == "" {
		f["company.email"] = "Email perusahaan wajib diisi"
	}
	for k, v := range map[string]*string{
		"company.postal_code": m.PostalCode, "company.kelurahan": m.Kelurahan,
		"company.kecamatan": m.Kecamatan, "company.kota": m.Kota, "company.provinsi": m.Provinsi,
		"company.address_detail": m.Detail,
	} {
		if val(v) == "" {
			f[k] = "Wajib diisi"
		}
	}
	if val(m.OwnerName) == "" {
		f["representative.name"] = "Nama penanggung jawab wajib diisi"
	}
	if val(m.OwnerPosition) == "" {
		f["representative.position"] = "Pilih jabatan"
	}
	if val(m.OwnerPhone) == "" {
		f["representative.phone"] = "Nomor telepon perwakilan wajib diisi"
	}
	if val(m.OwnerEmail) == "" {
		f["representative.email"] = "Email perwakilan wajib diisi"
	}
	if len(m.idEnc) == 0 {
		f["representative.id_number"] = "NIK wajib diisi"
	}
	have := map[string]bool{}
	for _, d := range docs {
		have[d.DocType] = true
	}
	if !have["ktp"] {
		f["representative.ktp"] = "Kartu Tanda Penduduk wajib diunggah"
	}
	if !have["npwp_badan"] {
		f["documents.npwp_badan"] = "Dokumen NPWP Badan wajib diunggah"
	}
	if m.bizEnc == nil {
		f["documents.npwp_number"] = "Nomor NPWP badan wajib diisi"
	}
	if !have["akta"] {
		f["documents.akta"] = "Akta Pendirian dan Perubahan Perusahaan wajib diunggah"
	}
	if !have["sk_kemenkumham"] {
		f["documents.sk_kemenkumham"] = "SK Kementerian Hukum & HAM wajib diunggah"
	}
	if !have["nib"] {
		f["documents.nib"] = "NIB wajib diunggah"
	}
	if val(m.NIB) == "" {
		f["documents.nib_number"] = "Nomor NIB (13 digit) wajib diisi"
	}
	if val(m.KBLI) == "" {
		f["documents.kbli"] = "KBLI 5 digit wajib diisi"
	}
	if !have["izin_usaha"] {
		f["documents.izin_usaha"] = "Surat Izin Usaha wajib diunggah"
	}
	if b.ID == "" || val(b.Name) == "" {
		f["brand.name"] = "Nama merek wajib diisi"
	}
	if val(b.Category) == "" {
		f["brand.category"] = "Kategori merek wajib dipilih"
	}
	if val(b.Description) == "" {
		f["brand.description"] = "Deskripsi merek wajib diisi"
	}
	if b.Amount == nil || *b.Amount <= 0 {
		f["brand.monthly_amount"] = "Jumlah transaksi bulanan wajib dipilih"
	}
	if b.Count == nil || *b.Count <= 0 {
		f["brand.monthly_count"] = "Total jumlah transaksi wajib dipilih"
	}
	if len(evs) == 0 {
		f["brand.evidences"] = "Minimal satu bukti bisnis wajib diunggah"
	}
	return f
}
