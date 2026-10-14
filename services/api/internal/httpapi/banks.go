package httpapi

import (
	"net/http"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/bank"
)

// GET /v1/banks — daftar bank yang didukung Kirim DOKU (GET publik, data tidak
// rahasia; dipakai form rekening merchant dan tampilan kode → nama bank).
func (d Deps) listBanks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{"data": bank.Banks})
}
