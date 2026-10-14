// Package bank berisi daftar bank yang didukung Kirim DOKU — sumber tunggal
// validasi bank_code rekening merchant.
//
// Data ini [V] salinan tabel "List of Supported Banks" halaman Kirim DOKU
// (https://developers.doku.com/payout/kirim-doku.md, diakses 14 Okt 2026).
// Selama EraPay hanya terhubung DOKU, daftar ini ekuivalen dengan daftar bank
// gateway; bila gateway kedua muncul, kode bank menjadi data per-gateway dan
// paket ini harus dipecah sesuai mapping gateway (invarian: domain tetap netral).
package bank

// Bank adalah satu entri daftar bank Kirim DOKU.
type Bank struct {
	Code   string `json:"code"`    // kode BI, mis. "014"; unit syariah berakhiran "SY"
	Name   string `json:"name"`    // nama bank sesuai tabel DOKU
	SWIFT  string `json:"swift"`   // kode SWIFT
	Online bool   `json:"online"`  // memiliki layanan perbankan online
	BIFast bool   `json:"bi_fast"` // mendukung kanal BI-FAST
}

// Banks adalah 125 bank pada daftar Kirim DOKU (termasuk unit usaha syariah).
//
//nolint:gochecknoglobals // daftar statis referensi
var Banks = [125]Bank{
	{"002", "BANK BRI", "BRINIDJA", true, true},
	{"008", "BANK MANDIRI", "BMRIIDJA", true, true},
	{"009", "BANK BNI 46", "BNINIDJA", true, true},
	{"011", "BANK DANAMON", "BDINIDJA", true, true},
	{"011SY", "BANK DANAMON UUS (SYARIAH)", "SYBDIDJ1", false, true},
	{"013", "BANK PERMATA", "BBBAIDJA", true, true},
	{"013SY", "BANK PERMATA UUS (SYARIAH)", "SYBBIDJ1", false, true},
	{"014", "BANK BCA", "CENAIDJA", true, true},
	{"016", "BANK MAYBANK", "IBBKIDJA", true, true},
	{"016SY", "BANK MAYBANK UUS (SYARIAH)", "SYBKIDJ1", false, true},
	{"019", "BANK PANIN", "PINBIDJA", true, true},
	{"022", "BANK CIMB NIAGA", "BNIAIDJA", true, true},
	{"022SY", "BANK CIMB NIAGA UUS (SYARIAH)", "SYNAIDJ1", false, true},
	{"023", "BANK UOB INDONESIA", "BBIJIDJA", true, true},
	{"028", "BANK OCBC NISP", "NISPIDJA", true, true},
	{"028SY", "BANK OCBS NISP UUS (SYARIAH)", "SYONIDJ1", false, true},
	{"031", "CITIBANK", "CITIIDJX", true, true},
	{"032", "JPMORGAN BANK", "CHASIDJX", false, true},
	{"033", "BANK OF AMERICA NA", "BOFAID2X", false, true},
	{"036", "BANK CCB INDONESIA", "MCORIDJA", true, true},
	{"037", "BANK ARTHA GRAHA", "ARTGIDJA", true, true},
	{"042", "MUFG BANK", "BOTKIDJX", true, true},
	{"046", "BANK DBS INDONESIA", "DBSBIDJA", true, true},
	{"047", "BANK RESONA PERDANIA", "BPIAIDJA", false, true},
	{"048", "BANK MIZUHO INDONESIA", "MHCCIDJA", false, true},
	{"050", "BANK STANDARD CHARTERED", "SCBLIDJX", true, true},
	{"054", "BANK CAPITAL", "BCIAIDJA", true, true},
	{"057", "BANK BNP PARIBAS", "BNPAIDJA", false, true},
	{"061", "BANK ANZ INDONESIA", "ANZBIDJX", true, true},
	{"067", "DEUTSCHE BANK", "DEUTIDJA", false, true},
	{"069", "BANK OF CHINA HK. LTD.", "BKCHIDJA", true, true},
	{"076", "BANK BUMI ARTA", "BBAIIDJA", true, true},
	{"087", "BANK HSBC INDONESIA", "HSBCIDJA", true, true},
	{"095", "BANK JTRUST INDONESIA", "CICTIDJA", true, true},
	{"097", "BANK MAYAPADA INTERNATIONAL", "MAYAIDJA", true, true},
	{"110", "BANK JABAR", "PDJBIDJ1", true, true},
	{"111", "BANK DKI", "BDKIIDJ1", true, true},
	{"111SY", "BANK DKI UUS (SYARIAH)", "SYDKIDJ1", false, true},
	{"112", "BANK BPD DIY", "PDYKIDJ1", true, true},
	{"112SY", "BANK BPD DIY UUS (SYARIAH)", "SYYKIDJ1", false, true},
	{"113", "BANK JATENG", "PDJGIDJ1", true, true},
	{"113SY", "BANK JATENG UUS (SYARIAH)", "SYJGIDJ1", false, true},
	{"114", "BANK JATIM", "PDJTIDJ1", true, true},
	{"114SY", "BANK JATIM UUS (SYARIAH)", "SYJTIDJ1", false, true},
	{"115", "BPD JAMBI", "PDJMIDJ1", true, true},
	{"115SY", "BPD JAMBI UUS (SYARIAH)", "SYJMIDJ1", false, true},
	{"116", "BANK ACEH", "SYACIDJ1", true, true},
	{"117", "BANK SUMUT", "PDSUIDJ1", true, true},
	{"117SY", "BANK SUMUT UUS (SYARIAH)", "SYSUIDJ1", false, true},
	{"118", "BPD SUMATERA BARAT/BANK NAGARI", "PDSBIDJ1", true, true},
	{"118SY", "BANK NAGARI UUS (SYARIAH)", "SYSBIDJ1", false, true},
	{"119", "BANK RIAU", "PDRIIDJA", true, true},
	{"120", "BPD SUMSEL BABEL", "BSSPIDSP", true, true},
	{"120SY", "BPD SUMSEL BABEL UUS (SYARIAH)", "SYSSIDJ1", false, true},
	{"121", "BANK LAMPUNG", "PDLPIDJ1", true, true},
	{"122", "BPD KALSEL", "PDKSIDBB", true, true},
	{"122SY", "BPD KALSEL UUS (SYARIAH)", "SYKSIDJ1", false, true},
	{"123", "BPD KALBAR", "PDKBIDJ1", true, true},
	{"123SY", "BPD KALBAR UUS (SYARIAH)", "SYKBIDJ1", false, true},
	{"124", "BPD KALTIMTARA", "PDKTIDJ1", true, true},
	{"124SY", "BPD KALTIMTARA UUS (SYARIAH)", "SYKTIDJ1", false, true},
	{"125", "BPD KALTENG", "PDKGIDJ1", true, true},
	{"126", "BANK SULSELBAR", "PDWSIDJA", true, true},
	{"126SY", "BANK SULSELBAR UUS (SYARIAH)", "SYWSIDJ1", false, true},
	{"127", "BANK SULUTGO", "PDWUIDJ1", true, true},
	{"128", "BANK NTB", "PDNBIDJ1", true, true},
	{"129", "BPD BALI", "ABALIDBS", true, true},
	{"130", "BANK NTT", "PDNTIDJA", true, true},
	{"131", "BPD MALUKU", "PDMLIDJ1", true, true},
	{"132", "BPD PAPUA", "PDIJIDJ1", true, true},
	{"133", "BPD BENGKULU", "PDBKIDJ1", true, true},
	{"134", "BPD SULTENG", "PDWGIDJ1", true, true},
	{"135", "BPD SULTRA", "PDWRIDJ1", true, true},
	{"137", "BPD BANTEN", "PDBBIDJ1", true, true},
	{"146", "BANK OF INDIA INDONESIA", "BKIDIDJA", true, false},
	{"147", "BANK MUAMALAT INDONESIA", "MUABIDJA", true, true},
	{"151", "BANK MESTIKA DHARMA", "MEDHIDS1", true, true},
	{"152", "BANK SHINHAN INDONESIA", "MEEKIDJ1", true, true},
	{"153", "BANK SINARMAS", "SBJKIDJA", true, true},
	{"153SY", "BANK NANO SYARIAH", "NANOIDJ1", false, true},
	{"157", "BANK MASPION INDONESIA", "MASDIDJ1", true, true},
	{"161", "BANK GANESHA", "GNESIDJA", true, true},
	{"164", "BANK ICBC", "ICBKIDJA", true, true},
	{"167", "BANK QNB INDONESIA", "AWANIDJA", true, true},
	{"200", "BANK BTN", "BTANIDJA", true, true},
	{"212", "BANK WOORI SAUDARA", "BSDRIDJA", true, true},
	{"213", "BANK SMBC INDONESIA", "SUNIIDJA", true, true},
	{"405", "BANK SYARIAH NASIONAL", "SWAGIDJ1", true, true},
	{"425", "BANK JABAR BANTEN SYARIAH", "SYJBIDJ1", true, true},
	{"426", "BANK MEGA", "MEGAIDJA", true, true},
	{"441", "BANK KB BUKOPIN", "BBUKIDJA", true, true},
	{"451", "BANK BSI (BANK SYARIAH INDONESIA)", "BSMDIDJA", true, true},
	{"459", "KROM BANK", "BUSTIDJ1", true, true},
	{"472", "BANK JASA JAKARTA", "JSABDJI1", true, true},
	{"484", "BANK KEB HANA", "HNBNIDJA", true, true},
	{"485", "BANK MNC INTERNATIONAL", "BUMIIDJA", true, true},
	{"490", "BANK NEO COMMERCE", "YUDBIDJ1", true, true},
	{"494", "BANK RAYA INDONESIA", "AGTBIDJA", true, true},
	{"498", "BANK SBI INDONESIA", "IDMOIDJ1", true, true},
	{"501", "BANK DIGITAL BCA", "BBLUIDJA", true, true},
	{"503", "BANK NATIONAL NOBU", "NOBUIDJA", true, true},
	{"506", "BANK MEGA SYARIAH", "BUTGIDJ1", true, true},
	{"513", "BANK INA PERDANA", "IAPTIDJA", false, true},
	{"517", "BANK PANIN DUBAI SYARIAH", "ARFAIDJ1", true, true},
	{"520", "BANK PRIMA MASTER", "PMASIDJ1", true, true},
	{"521", "BANK BUKOPIN SYARIAH", "SDOBIDJ1", true, true},
	{"523", "BANK SAHABAT SAMPOERNA", "SAHMIDJA", true, true},
	{"526", "BANK OKE INDONESIA", "LMANIDJ1", true, true},
	{"531", "BANK AMAR", "LOMAIDJ1", false, true},
	{"535", "BANK SEABANK INDONESIA", "SSPIIDJA", true, true},
	{"536", "BANK BCA SYARIAH", "BSYAIDJA", true, true},
	{"542", "BANK JAGO", "JAGBIDJA", true, true},
	{"547", "BANK BTPN SYARIAH", "PUBAIDJ1", true, true},
	{"548", "BANK MULTI ARTA SENTOSA", "BMSEIDJA", true, true},
	{"553", "BANK HIBANK INDONESIA", "MAYOIDJA", true, true},
	{"555", "BANK INDEX", "BIDXIDJA", true, true},
	{"562", "SUPERBANK INDONESIA", "FAMAIDJ1", true, true},
	{"564", "BANK MANDIRI TASPEN", "SIHBIDJ1", true, true},
	{"566", "BANK VICTORIA INTERNATIONAL", "VICTIDJ1", true, true},
	{"567", "BANK ALLO", "ALOBIDJ1", true, true},
	{"945", "BANK IBK INDONESIA", "IBKOIDJA", true, true},
	{"947", "BANK ALADIN SYARIAH", "NETBIDJA", true, true},
	{"949", "BANK CTBC INDONESIA", "CTCBIDJA", true, true},
	{"950", "BANK COMMONWEALTH", "BICNIDJA", true, true},
	{"542SY", "PT. BANK JAGO SYARIAH", "SYATIDJ1", true, true},
}

// EWallet adalah e-wallet yang didukung Kirim DOKU (channelCode "07"; nomor
// rekening = nomor HP penerima). EraPay v1 hanya mencairkan ke rekening bank,
// jadi kode e-wallet TIDAK valid untuk rekening merchant — daftar ini disimpan
// untuk referensi dan uji masa depan (doku.md §6).
type EWallet struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// EWallets adalah 5 e-wallet pada daftar Kirim DOKU.
//
//nolint:gochecknoglobals
var EWallets = [5]EWallet{
	{"GOPAY", "Gopay"},
	{"DANA", "DANA"},
	{"SHOPEEPAY", "Shopeepay"},
	{"OVO", "OVO"},
	{"911", "LinkAja"},
}

// byCode adalah indeks kode → bank, dibangun sekali di init.
//
//nolint:gochecknoglobals
var byCode = func() map[string]Bank {
	m := make(map[string]Bank, len(Banks))
	for _, b := range Banks {
		m[b.Code] = b
	}
	return m
}()

// Lookup mengembalikan bank berdasarkan kode BI.
func Lookup(code string) (Bank, bool) {
	b, ok := byCode[code]
	return b, ok
}

// Valid apakah kode bank didukung Kirim DOKU (rekening bank, bukan e-wallet).
func Valid(code string) bool {
	_, ok := byCode[code]
	return ok
}

// Name mengembalikan nama bank dari kode BI; "" bila tidak dikenal.
func Name(code string) string {
	if b, ok := byCode[code]; ok {
		return b.Name
	}
	return ""
}
