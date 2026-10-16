package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
)

// zeroTime: waktu modifikasi tidak diketahui untuk http.ServeContent.
var zeroTime time.Time

// kycErr memetakan error paket kyc ke respons {error:{code,message,fields}}.
func (d Deps) kycErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var fe *kyc.FieldError
	var ie *kyc.IncompleteError
	var ve *kyc.InvalidError
	switch {
	case err == nil:
		return true
	case errors.As(err, &fe):
		writeFields(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "ada isian yang tidak valid", fe.Fields)
	case errors.As(err, &ie):
		writeFields(w, r, http.StatusBadRequest, "KYC_INCOMPLETE", "pengajuan belum lengkap", ie.Fields)
	case errors.As(err, &ve):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", ve.Msg, false)
	case errors.Is(err, kyc.ErrIncomplete):
		writeError(w, r, http.StatusBadRequest, "KYC_INCOMPLETE", "pengajuan belum lengkap", false)
	case errors.Is(err, kyc.ErrLocked):
		writeError(w, r, http.StatusConflict, "KYC_LOCKED", "data tidak bisa diubah (sedang diverifikasi atau sudah terverifikasi); hubungi admin bila perlu perubahan", false)
	case errors.Is(err, kyc.ErrInvalidSt):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "aksi tidak sah untuk status saat ini", false)
	case errors.Is(err, kyc.ErrSensitiveConfirm):
		writeError(w, r, http.StatusConflict, "SENSITIVE_CONFIRM_REQUIRED", "mengubah data sensitif saat sudah disetujui memicu peninjauan ulang; konfirmasi diperlukan", false)
	case errors.Is(err, kyc.ErrRateLimit):
		w.Header().Set("Retry-After", "3600")
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "terlalu banyak percobaan, coba lagi nanti", true)
	case errors.Is(err, kyc.ErrTooLarge):
		writeError(w, r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "ukuran berkas melebihi batas", false)
	case errors.Is(err, kyc.ErrBadFile):
		writeError(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "jenis berkas tidak didukung (hanya PDF/JPG/PNG)", false)
	case errors.Is(err, kyc.ErrForbidden):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "bukan milik merchant ini", false)
	case errors.Is(err, kyc.ErrStorageMissing):
		writeError(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "penyimpanan berkas belum dikonfigurasi", false)
	case errors.Is(err, kyc.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	default:
		d.Log.Error("kyc", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// writeFields menambah daftar pesan per-field pada bentuk error standar.
func writeFields(w http.ResponseWriter, r *http.Request, status int, code, msg string, fields map[string]string) {
	if fields == nil {
		fields = map[string]string{}
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code": code, "message": msg, "fields": fields,
		"request_id": middleware.GetReqID(r.Context()),
	}})
}

// ---- merchant side (dashboard session only) ----

// kycProfile: profil KYC milik merchant (NIK/NPWP selalu ter-mask).
func (d Deps) kycProfile(w http.ResponseWriter, r *http.Request) {
	p, err := d.KYC.GetProfile(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// kycProfileForEdit: profil dengan NIK/NPWP terbuka untuk mode edit pemilik.
func (d Deps) kycProfileForEdit(w http.ResponseWriter, r *http.Request) {
	p, err := d.KYC.GetProfileForEdit(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// kycSaveSection: simpan satu bagian (company|representative|brand).
func (d Deps) kycSaveSection(w http.ResponseWriter, r *http.Request) {
	section := chi.URLParam(r, "section")
	if !kyc.ValidSection(section) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	var body struct {
		ConfirmSensitive bool                     `json:"confirm_sensitive"`
		Company          *kyc.CompanyInput        `json:"company,omitempty"`
		Representative   *kyc.RepresentativeInput `json:"representative,omitempty"`
		Brand            *kyc.BrandInput          `json:"brand,omitempty"`
	}
	if _, ok := decodeStrict(w, r, 16<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	err := d.KYC.SaveSection(r.Context(), kyc.SaveSectionInput{
		MerchantID: p.MerchantID, ActorID: p.UserID, RequestID: middleware.GetReqID(r.Context()),
		ConfirmSensitive: body.ConfirmSensitive, Section: section,
		Company: body.Company, Representative: body.Representative, Brand: body.Brand,
	})
	if !d.kycErr(w, r, err) {
		return
	}
	prof, err := d.KYC.GetProfile(r.Context(), p.MerchantID)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

// kycSubmit: kirim seluruh isian untuk verifikasi (DRAFT/NEEDS_REVISION -> SUBMITTED).
func (d Deps) kycSubmit(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	err := d.KYC.Submit(r.Context(), kyc.SubmitInput{
		MerchantID: p.MerchantID, ActorID: p.UserID, IP: d.clientIP(r),
		RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.kycErr(w, r, err) {
		return
	}
	prof, err := d.KYC.GetProfile(r.Context(), p.MerchantID)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

// uploadLimit: batas body multipart = batas berkas + ruang untuk field lain.
func uploadLimit(docType string) int64 {
	limit := kyc.MaxFileBytes
	if docType == "akta" {
		limit = kyc.MaxAktaBytes
	}
	if docType == "" {
		limit = kyc.MaxImageBytes
	}
	return limit + (1 << 20)
}

// kycUploadDocument: unggah satu dokumen wajib (multipart: file, number, kbli, confirm).
func (d Deps) kycUploadDocument(w http.ResponseWriter, r *http.Request) {
	if !d.requireFileUpload(w, r) {
		return
	}
	p := PrincipalFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit(chi.URLParam(r, "doc_type")))
	file, head, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "field file wajib berupa multipart/form-data", false)
		return
	}
	defer file.Close()
	in := kyc.UploadInput{
		MerchantID: p.MerchantID, ActorID: p.UserID,
		DocType:          chi.URLParam(r, "doc_type"),
		Number:           r.FormValue("number"),
		KBLI:             r.FormValue("kbli"),
		Name:             head.Filename,
		Size:             head.Size,
		Reader:           file,
		IP:               d.clientIP(r),
		RequestID:        middleware.GetReqID(r.Context()),
		ConfirmSensitive: r.FormValue("confirm") == "true",
	}
	doc, err := d.KYC.UploadDocument(r.Context(), in)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

// kycSetBrandLogo: unggah logo merek (multipart: file).
func (d Deps) kycSetBrandLogo(w http.ResponseWriter, r *http.Request) {
	if !d.requireFileUpload(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit(""))
	file, head, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "field file wajib berupa multipart/form-data", false)
		return
	}
	defer file.Close()
	p := PrincipalFrom(r.Context())
	err = d.KYC.SetBrandLogo(r.Context(), kyc.UploadInput{
		MerchantID: p.MerchantID, ActorID: p.UserID, Name: head.Filename,
		Size: head.Size, Reader: file,
		IP: d.clientIP(r), RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// kycAddEvidence: tambah satu bukti bisnis (gambar, maks 5 MB).
func (d Deps) kycAddEvidence(w http.ResponseWriter, r *http.Request) {
	if !d.requireFileUpload(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit(""))
	file, head, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "field file wajib berupa multipart/form-data", false)
		return
	}
	defer file.Close()
	p := PrincipalFrom(r.Context())
	ev, err := d.KYC.AddEvidence(r.Context(), kyc.UploadInput{
		MerchantID: p.MerchantID, ActorID: p.UserID, Name: head.Filename,
		Size: head.Size, Reader: file,
		IP: d.clientIP(r), RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, ev)
}

// kycDeleteEvidence: hapus satu bukti bisnis milik merchant.
func (d Deps) kycDeleteEvidence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	err := d.KYC.DeleteEvidence(r.Context(), p.MerchantID, p.UserID, id, middleware.GetReqID(r.Context()))
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// requireFileUpload menolak request tanpa multipart form (pesan konsisten sebelum parsing besar).
func (d Deps) requireFileUpload(w http.ResponseWriter, r *http.Request) bool {
	if !startsWithMultipart(r) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "wajib multipart/form-data", false)
		return false
	}
	return true
}

func startsWithMultipart(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	mt, _, err := mime.ParseMediaType(ct)
	return err == nil && mt == "multipart/form-data"
}

// kycDocumentFile: unduh dokumen (pemilik atau reviewer platform).
func (d Deps) kycDocumentFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	isReviewer := p.MerchantID == ""
	var viewer string
	if !isReviewer {
		viewer = p.MerchantID
	}
	doc, f, err := d.KYC.GetDocumentFile(r.Context(), id, viewer, isReviewer)
	if !d.kycErr(w, r, err) {
		return
	}
	defer f.Close()
	if !d.kycAuditAccess(w, r, p, doc.MerchantID, "document", doc.ID, isReviewer) {
		return
	}
	serveKycFile(w, r, doc.Name, doc.Mime, f)
}

// kycEvidenceFile: unduh bukti bisnis (pemilik atau reviewer platform).
func (d Deps) kycEvidenceFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	isReviewer := p.MerchantID == ""
	var viewer string
	if !isReviewer {
		viewer = p.MerchantID
	}
	ev, f, err := d.KYC.GetEvidenceFile(r.Context(), id, viewer, isReviewer)
	if !d.kycErr(w, r, err) {
		return
	}
	defer f.Close()
	if !d.kycAuditAccess(w, r, p, ev.MerchantID, "evidence", ev.ID, isReviewer) {
		return
	}
	serveKycFile(w, r, ev.Name, ev.Mime, f)
}

// kycLogoFile: logo merek milik merchant yang sedang login.
func (d Deps) kycLogoFile(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p.MerchantID == "" {
		http.NotFound(w, r)
		return
	}
	name, mimeType, f, err := d.KYC.GetLogoFile(r.Context(), p.MerchantID)
	if !d.kycErr(w, r, err) {
		return
	}
	defer f.Close()
	serveKycFile(w, r, name, mimeType, f)
}

// adminKycLogoFile: logo merek merchant {id} untuk reviewer platform.
func (d Deps) adminKycLogoFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, mimeType, f, err := d.KYC.GetLogoFile(r.Context(), id)
	if !d.kycErr(w, r, err) {
		return
	}
	defer f.Close()
	if !d.kycAuditAccess(w, r, PrincipalFrom(r.Context()), id, "logo", id, true) {
		return
	}
	serveKycFile(w, r, name, mimeType, f)
}

// kycAuditAccess mencatat pembukaan berkas KYC (audit append-only) sebelum berkas dikirim; gagal mencatat =
// berkas tidak dikirim. Logo milik pemilik sendiri tidak dicatat (aset merek yang tampil di tiap halaman).
func (d Deps) kycAuditAccess(w http.ResponseWriter, r *http.Request, p *auth.Principal, merchantID, kind, fileID string, reviewer bool) bool {
	if err := d.KYC.RecordFileAccess(r.Context(), p.UserID, merchantID, kind, fileID, reviewer, requestIDOf(r)); err != nil {
		d.Log.Error("kyc.file.audit", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return false
	}
	return true
}

// serveKycFile mengalirkan berkas dari Storage dengan header aman.
func serveKycFile(w http.ResponseWriter, r *http.Request, name, mimeType string, f io.ReadSeekCloser) {
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType("inline", map[string]string{"filename": name}))
	http.ServeContent(w, r, name, zeroTime, f)
}

// kycRegions: autocomplete wilayah (?q= &limit=).
func (d Deps) kycRegions(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rs, err := d.KYC.RegionSearch(r.Context(), r.URL.Query().Get("q"), limit)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rs})
}

// ---- platform side (reviewer) ----

// adminKycProfile: profil KYC merchant (ter-mask) untuk tinjauan platform.
func (d Deps) adminKycProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	prof, err := d.KYC.GetProfile(r.Context(), id)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

// kycReviewAction: start-review | approve | reject.
func (d Deps) kycReviewAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		p := PrincipalFrom(r.Context())
		reqID := middleware.GetReqID(r.Context())
		var err error
		switch action {
		case "start-review":
			err = d.KYC.StartReview(r.Context(), kyc.ReviewInput{
				MerchantID: id, ActorID: p.UserID, RequestID: middleware.GetReqID(r.Context())})
		case "approve":
			var body struct {
				MDRBPS      *int `json:"mdr_bps"`
				PlatformBPS *int `json:"platform_bps"`
			}
			if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
				return
			}
			if body.MDRBPS == nil || body.PlatformBPS == nil {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "mdr_bps dan platform_bps wajib diisi", false)
				return
			}
			err = d.KYC.Approve(r.Context(), kyc.ApproveInput{
				MerchantID: id, ActorID: p.UserID,
				MDRBPS: *body.MDRBPS, PlatformBPS: *body.PlatformBPS,
				RequestID: reqID})
		case "reject":
			var body struct {
				Note string `json:"note"`
			}
			if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
				return
			}
			err = d.KYC.Reject(r.Context(), kyc.RejectInput{
				MerchantID: id, ActorID: p.UserID, Note: body.Note, RequestID: reqID})
		}
		if !d.kycErr(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "action": action})
	}
}

// kycRequestChanges: NEEDS_REVISION dengan catatan per bagian/field (wajib >= 1 catatan).
func (d Deps) kycRequestChanges(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Notes []kyc.NoteInput `json:"notes"`
	}
	if _, ok := decodeStrict(w, r, 16<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	err := d.KYC.RequestChanges(r.Context(), kyc.ReviewInput{
		MerchantID: id, ActorID: p.UserID, RequestID: middleware.GetReqID(r.Context()),
	}, body.Notes)
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// kycUnlock: admin membuka kunci data merchant terverifikasi (approved -> action_required), alasan wajib.
func (d Deps) kycUnlock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	err := d.KYC.Unlock(r.Context(), kyc.UnlockInput{
		MerchantID: id, ActorID: p.UserID, Note: body.Note, RequestID: middleware.GetReqID(r.Context())})
	if !d.kycErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
