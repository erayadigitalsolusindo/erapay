package kyc

// Input dari handler httpapi (JSON). NIK/NPWP dikirim penuh hanya saat edit oleh pemilik;
// server menerima kosong = "tidak berubah".

type CompanyInput struct {
	EntityType    string `json:"entity_type"`
	CompanyName   string `json:"company_name"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	PostalCode    string `json:"postal_code"`
	Kelurahan     string `json:"kelurahan"`
	Kecamatan     string `json:"kecamatan"`
	Kota          string `json:"kota"`
	Provinsi      string `json:"provinsi"`
	AddressDetail string `json:"address_detail"`
}

type RepresentativeInput struct {
	Name        string `json:"name"`
	Position    string `json:"position"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Nationality string `json:"nationality"`
	IDNumber    string `json:"id_number"` // 16 digit; kosong = tetap (ter-mask di UI)
}

type BrandInput struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Description   string   `json:"description"`
	MonthlyAmount int64    `json:"monthly_amount"`
	MonthlyCount  int64    `json:"monthly_count"`
	Links         []string `json:"links"` // urutan tetap; ditulis ulang saat simpan
	// DeleteEvidenceIDs / AddEvidence ditangani endpoint tersendiri (multipart).
}

// DocumentsInput: nomor pendamping dokumen legal. Kosong = tidak berubah (NPWP tampil ter-mask di UI).
type DocumentsInput struct {
	NPWPNumber string `json:"npwp_number"`
	NIBNumber  string `json:"nib_number"`
	KBLI       string `json:"kbli"`
}

type SaveSectionInput struct {
	MerchantID string
	ActorID    string
	RequestID  string
	// ConfirmSensitive wajib true bila status approved dan bagian mengubah field sensitif
	// (UI wajib menampilkan konfirmasi terlebih dahulu).
	ConfirmSensitive bool
	// Section + payload satu dari tiga bagian isian (dokumen lewat endpoint upload).
	Section        string
	Company        *CompanyInput
	Representative *RepresentativeInput
	Brand          *BrandInput
	Documents      *DocumentsInput // hanya nomor; berkas lewat endpoint unggah
}
