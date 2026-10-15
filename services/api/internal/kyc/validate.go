package kyc

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// validateCompany memvalidasi bagian Perusahaan; mengembalikan pesan per field (kosong = valid).
func validateCompany(in *CompanyInput) map[string]string {
	f := map[string]string{}
	in.EntityType = strings.TrimSpace(in.EntityType)
	switch in.EntityType {
	case "PT", "CV", "Yayasan", "Perorangan":
	default:
		f["entity_type"] = "Pilih jenis badan usaha (PT/CV/Yayasan/Perorangan)"
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(in.CompanyName)); n < 3 || n > 120 {
		f["company_name"] = "Nama company wajib 3-120 karakter"
	}
	if p := normPhone(in.Phone); p != "" {
		in.Phone = p
	} else {
		f["phone"] = "Nomor telepon tidak valid (mis. 81234567890)"
	}
	if e := lower(strings.TrimSpace(in.Email)); emailRe.MatchString(e) && len(e) <= 254 {
		in.Email = e
	} else {
		f["email"] = "Format email tidak valid"
	}
	in.PostalCode = trimStr(in.PostalCode, 10)
	in.Kelurahan = trimStr(in.Kelurahan, 100)
	in.Kecamatan = trimStr(in.Kecamatan, 100)
	in.Kota = trimStr(in.Kota, 100)
	in.Provinsi = trimStr(in.Provinsi, 100)
	if in.PostalCode == "" {
		f["postal_code"] = "Kode pos wajib diisi"
	}
	if in.Kelurahan == "" {
		f["kelurahan"] = "Kelurahan/Desa wajib diisi"
	}
	if in.Kecamatan == "" {
		f["kecamatan"] = "Kecamatan wajib diisi"
	}
	if in.Kota == "" {
		f["kota"] = "Kota/Kabupaten wajib diisi"
	}
	if in.Provinsi == "" {
		f["provinsi"] = "Provinsi wajib diisi"
	}
	in.AddressDetail = strings.TrimSpace(in.AddressDetail)
	if utf8.RuneCountInString(in.AddressDetail) > 500 {
		f["address_detail"] = "Detail alamat maksimal 500 karakter"
	}
	return f
}

// validateRepresentative: NIK kosong berarti "tidak berubah" (server menyimpan yang lama).
func validateRepresentative(in *RepresentativeInput) map[string]string {
	f := map[string]string{}
	if n := utf8.RuneCountInString(strings.TrimSpace(in.Name)); n < 3 || n > 100 {
		f["name"] = "Nama penanggung jawab wajib 3-100 karakter"
	}
	in.Position = strings.TrimSpace(in.Position)
	switch in.Position {
	case "Pemilik", "Direktur", "Komisaris", "Manajer":
	default:
		f["position"] = "Pilih jabatan"
	}
	if p := normPhone(in.Phone); p != "" {
		in.Phone = p
	} else {
		f["phone"] = "Nomor telepon tidak valid"
	}
	if e := lower(strings.TrimSpace(in.Email)); emailRe.MatchString(e) && len(e) <= 254 {
		in.Email = e
	} else {
		f["email"] = "Format email tidak valid"
	}
	in.IDNumber = digitsOnly(in.IDNumber)
	if in.IDNumber != "" && !nikRe.MatchString(in.IDNumber) {
		f["id_number"] = "NIK harus 16 digit"
	}
	if in.Nationality = strings.TrimSpace(in.Nationality); in.Nationality == "" {
		in.Nationality = "Indonesia"
	}
	return f
}

func validateBrand(in *BrandInput) map[string]string {
	f := map[string]string{}
	in.Name = trimStr(in.Name, 100)
	if in.Name == "" {
		f["name"] = "Nama merek wajib diisi"
	}
	in.Category = trimStr(in.Category, 60)
	if in.Category == "" {
		f["category"] = "Pilih kategori merek"
	}
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Description) > 500 {
		f["description"] = "Deskripsi maksimal 500 karakter"
	}
	if in.MonthlyAmount < 0 {
		f["monthly_amount"] = "Jumlah transaksi tidak valid"
	}
	if in.MonthlyCount < 0 {
		f["monthly_count"] = "Jumlah transaksi tidak valid"
	}
	for i, l := range in.Links {
		if u := strings.TrimSpace(l); u != "" && !urlRe.MatchString(u) {
			f[fmt.Sprintf("links.%d", i)] = "URL harus dimulai dengan http:// atau https://"
		}
	}
	return f
}

// validateDocNumber: nomor NPWP 15/16 digit, NIB 13 digit, KBLI 5 digit. Kosong = hapus/tidak diisi.
func validateDocNumber(t, v string) (string, bool) {
	d := digitsOnly(v)
	switch t {
	case "npwp_badan":
		return d, d == "" || npwpRe.MatchString(d)
	case "nib":
		return d, d == "" || nibRe.MatchString(d)
	case "kbli":
		return d, d == "" || kbliRe.MatchString(d)
	}
	return d, false
}
