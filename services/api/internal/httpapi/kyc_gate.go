package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// requireProduction adalah gate fitur produksi: hanya merchant berstatus approved yang
// boleh lewat. NEEDS_REVIEW_AGAIN (recheck) mengikuti kebijakan ERAPAY_KYC_RECHECK_POLICY
// (default "open": tetap boleh bertransaksi, ditandai untuk ditinjau ulang).
// Gate pembayaran/payout yang sudah ada (payment.CreateTx / payout menolak non-approved)
// tetap sumber kebenaran utama di level service; middleware ini dipakai untuk endpoint
// fitur produksi tambahan di luar uang.
func (d Deps) requireProduction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := PrincipalFrom(r.Context())
		if p == nil || p.MerchantID == "" {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "merchant session required", false)
			return
		}
		allowed, _, err := d.KYC.ProductionAccess(r.Context(), p.MerchantID)
		if err != nil {
			d.kycErr(w, r, err)
			return
		}
		if !allowed {
			writeError(w, r, http.StatusForbidden, "KYC_NOT_APPROVED",
				"fitur ini terbuka setelah verifikasi data diri disetujui", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mountKYC mendaftarkan seluruh route KYC. PANGGIL dari mountV1 (auth.go):
//
//	if d.KYC != nil {
//		d.mountKYC(r) // di dalam r.Route("/v1", ...)
//	}
//
// Gate produksi (requireProduction) tidak dipasang otomatis: tempel pada endpoint
// fitur produksi baru, mis. r.With(d.requireProduction).Post("/...", ...).
func (d Deps) mountKYC(r chi.Router) {
	// Merchant (dashboard): data diri, unggah dokumen, submit.
	r.Group(func(r chi.Router) {
		r.Use(d.requireAuth, requireTenant)
		r.Get("/kyc/profile", d.kycProfile)
		r.Get("/kyc/profile/edit", d.kycProfileForEdit)
		r.Get("/kyc/regions", d.kycRegions)
		r.Post("/kyc/submit", d.kycSubmit)
		r.Post("/kyc/sections/{section}", d.kycSaveSection)
		r.Post("/kyc/documents/{doc_type}", d.kycUploadDocument)
		r.Get("/kyc/documents/{id}/file", d.kycDocumentFile)
		r.Post("/kyc/brand/logo", d.kycSetBrandLogo)
		r.Post("/kyc/evidences", d.kycAddEvidence)
		r.Delete("/kyc/evidences/{id}", d.kycDeleteEvidence)
		r.Get("/kyc/evidences/{id}/file", d.kycEvidenceFile)
	})
	// Platform (reviewer): tinjauan + keputusan. Sesi merchant ditolak requirePlatform.
	r.Route("/admin", func(r chi.Router) {
		r.Use(d.requireAuth, requirePlatform)
		r.With(requirePerm("merchant:review")).Get("/merchants/{id}/kyc", d.adminKycProfile)
		r.With(requirePerm("merchant:review")).Post("/merchants/{id}/kyc/start-review", d.kycReviewAction("start-review"))
		r.With(requirePerm("merchant:review")).Post("/merchants/{id}/kyc/request-changes", d.kycRequestChanges)
		r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/kyc/approve", d.kycReviewAction("approve"))
		r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/kyc/reject", d.kycReviewAction("reject"))
	})
}
