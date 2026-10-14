// Command kdtest men-probe gateway Kirim DOKU LANGSUNG ke endpoint production DOKU,
// tanpa melibatkan ledger/domain EraPay — untuk membuktikan kontrak yang masih [?]
// di docs/integrations/doku.md §6 (service code, mekanik fee, identitas pengirim
// badan usaha, CHANNEL-ID). ALAT UJI SEMENTARA: tidak ada retry otomatis
// (invarian #3), tidak ada dry-run, dan tiap stage inquiry/transfer mengeluarkan
// biaya sungguhan.
//
// Stages (urutan uji direkomendasikan DOKU):
//
//	kdtest balance                            GRAtis: cek saldo DOKU (juga bukti kredensial/token tervalidasi)
//	kdtest inquiry                           Rp500/success: validasi rekening + sessionId + feeAmount
//	kdtest transfer <sessionId>              UANG ASLI: transfer nominal penuh ke rekening tujuan
//	kdtest status <partnerRef> [gatewayRef]  cek hasil transfer (baca service code dari responseCode transfer: digit 4-5)
//
// Semua nilai dari .env (cari naik ke root repo bila perlu):
//
//	DOKU_CLIENT_ID / DOKU_SECRET_KEY / DOKU_PRIVATE_KEY (PEM, \n boleh di-escape) /
//	DOKU_PRIVATE_KEY_FILE (alternatif) / DOKU_TERMINAL_ID / DOKU_POSTAL_CODE /
//	DOKU_BASE_URL=https://api.doku.com
//	KD_TO_BANK / KD_TO_ACCOUNT / KD_TO_NAME        rekening tujuan uji (sebaiknya milik PT sendiri)
//	ERAPAY_PAYOUT_SENDER_PHONE / _FIRST_NAME / _LAST_NAME / _ID / _ID_TYPE
//	  identitas pengirim = config produksi yang sama (probe menguji persisnya)
//	KD_AMOUNT       nominal transfer (default 10000 = Rp10.000, minimum Paket 1)
//	KD_SERVICE_CODE service code check-status (kosong dulu; isi setelah terbaca dari responseCode)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/doku"
)

// loadDotEnv membaca KEY=VALUE sederhana (tanpa ekspansi shell) ke env proses.
// Mencari .env di direktori kerja lalu naik ke atas sampai root repo.
func loadDotEnv(paths ...string) error {
	var lastErr error
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			lastErr = err
			continue
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024) // private key bisa panjang
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				os.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
			}
		}
		if err := sc.Err(); err != nil {
			return err
		}
		return nil
	}
	return lastErr
}

func env(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "KDTEST: "+format+"\n", args...)
	os.Exit(1)
}

func splitName(full string) (string, string) {
	f := strings.Fields(full)
	if len(f) == 0 {
		return "", ""
	}
	if len(f) == 1 {
		return f[0], ""
	}
	return f[0], strings.Join(f[1:], " ")
}

func printResult(stage string, res any, err error) {
	fmt.Printf("=== %s\n", stage)
	if err != nil {
		var ge *gateway.Error
		if errors.As(err, &ge) {
			fmt.Printf("  ERROR kind=%d http=%d code=%q msg=%q retryable=%v\n", ge.Kind, ge.HTTPStatus, ge.Code, ge.Message, ge.Retryable)
		} else {
			fmt.Printf("  ERROR %v\n", err)
		}
		os.Exit(1)
	}
	j, _ := json.MarshalIndent(res, "", "  ")
	fmt.Printf("  %s\n", j)
}

func main() {
	if err := loadDotEnv(".env", "../.env", "../../.env"); err != nil {
		die("tidak bisa membaca .env (.env / ../.env / ../../.env): %v", err)
	}
	stage := "inquiry"
	if len(os.Args) > 1 {
		stage = os.Args[1]
	}
	dc, err := config.LoadDOKU()
	if err != nil {
		die("config DOKU: %v", err)
	}
	if dc == nil {
		die("DOKU_CLIENT_ID belum ada di .env")
	}
	if env("DOKU_BASE_URL") == "" {
		die("DOKU_BASE_URL wajib diisi untuk uji (production: https://api.doku.com)")
	}
	key, err := doku.ParsePrivateKey(dc.PrivateKeyPEM)
	if err != nil {
		die("private key: %v", err)
	}
	a, err := doku.New(doku.Config{BaseURL: env("DOKU_BASE_URL"), ClientID: dc.ClientID, ClientSecret: dc.ClientSecret,
		PrivateKey: key, TerminalID: dc.TerminalID, PostalCode: dc.PostalCode,
		HTTP: &http.Client{Timeout: 30 * time.Second}})
	if err != nil {
		die("adapter: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	switch stage {
	case "balance":
		res, err := a.BalanceInquiry(ctx, env("KD_SENDER_PHONE"))
		printResult("BALANCE (gratis; bukti kredensial tervalidasi)", res, err)
		if err == nil {
			fmt.Printf("  → saldo tersedia %d (hold %d) pada akun %q atas nama %q\n",
				res.AvailableBalance, res.HoldAmount, res.AccountNo, res.Name)
		}
	case "inquiry":
		bank, acct, name := dest()
		res, err := a.InquiryBankAccount(ctx, gateway.BankInquiryRequest{
			CustomerNumber: env("ERAPAY_PAYOUT_SENDER_PHONE"),
			AccountNo:      acct,
			AccountName:    name,
			BankCode:       bank,
			Amount:         10000,
			Currency:       "IDR",
			SenderCountry:  "ID",
		})
		printResult("INQUIRY (Rp500 bila berhasil)", res, err)
		fmt.Printf("  → sessionId=%q — wajib untuk stage transfer. feeAmount=%d (−1 = tidak dilaporkan).\n", res.SessionID, res.FeeAmount)
	case "transfer":
		if len(os.Args) < 3 {
			die("pakai: kdtest transfer <sessionId-dari-inquiry>")
		}
		bank, acct, name := dest()
		amount := int64(10000)
		if v := env("KD_AMOUNT"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				die("KD_AMOUNT harus integer positif (rupiah penuh)")
			}
			amount = n
		}
		ref := "KDTEST-" + time.Now().UTC().Format("20060102150405")
		bf, bl := splitName(name)
		res, err := a.TransferBank(ctx, gateway.TransferRequest{
			Reference:              ref,
			CustomerNumber:         env("KD_SENDER_PHONE"),
			SessionID:              os.Args[2],
			AccountNo:              acct,
			BankCode:               bank,
			Amount:                 amount,
			Currency:               "IDR",
			BeneficiaryFirstName:   bf,
			BeneficiaryLastName:    bl,
			BeneficiaryPhone:       env("ERAPAY_PAYOUT_SENDER_PHONE"),
			BeneficiaryAccountName: name,
			SenderCountry:          "ID",
			SenderFirstName:        env("ERAPAY_PAYOUT_SENDER_FIRST_NAME"),
			SenderLastName:         env("ERAPAY_PAYOUT_SENDER_LAST_NAME"),
			SenderPersonalID:       env("ERAPAY_PAYOUT_SENDER_ID"),
			SenderIDType:           env("ERAPAY_PAYOUT_SENDER_ID_TYPE"),
		})
		printResult("TRANSFER "+ref+" (UANG ASLI)", res, err)
		fmt.Printf("  → referenceNo=%q transactionDate=%v\n", res.GatewayReference, res.TransactionDate)
		fmt.Println("  → Catat responseCode di atas: digit 4-5 = service code untuk stage status.")
	case "status":
		if len(os.Args) < 3 {
			die("pakai: kdtest status <partnerRef> [gatewayRef]")
		}
		gwRef := ""
		if len(os.Args) > 3 {
			gwRef = os.Args[3]
		}
		bank, acct, _ := dest()
		res, err := a.TransferStatus(ctx, gateway.TransferStatusRequest{
			OriginalPartnerReference: os.Args[2],
			OriginalReference:        gwRef,
			AccountNo:                acct,
			BankCode:                 bank,
			ServiceCode:              env("KD_SERVICE_CODE"),
		})
		printResult("STATUS "+os.Args[2], res, err)
		if st, ok := a.MapTransferStatus(res.RawStatus); ok {
			fmt.Printf("  → status termapping: %s\n", st)
		} else {
			fmt.Printf("  → status %q TIDAK dikenali (hanya 00/03/04/06 yang dipetakan)\n", res.RawStatus)
		}
	default:
		die("stage tak dikenal: %s (balance|inquiry|transfer|status)", stage)
	}
}

func dest() (bank, account, name string) {
	bank, account, name = env("KD_TO_BANK"), env("KD_TO_ACCOUNT"), env("KD_TO_NAME")
	if bank == "" || account == "" || name == "" {
		die("KD_TO_BANK / KD_TO_ACCOUNT / KD_TO_NAME wajib di .env")
	}
	return bank, account, name
}
