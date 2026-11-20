package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apprelease"
)

// platformUploadListener (called by platformUploadListenerDirect): version=1.0.1&note=... with the raw APK as the body.
func (d Deps) platformUploadListener(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	body := http.MaxBytesReader(w, r.Body, apprelease.MaxBytes+1)
	rel, err := d.AppReleases.Upload(r.Context(), apprelease.KindQRISGateListener, q.Get("version"), q.Get("note"), body,
		PrincipalFrom(r.Context()).UserID, middleware.GetReqID(r.Context()))
	switch {
	case errors.Is(err, apprelease.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "version must be 1-32 chars (letters, digits, . _ + -) and note at most 500", false)
	case errors.Is(err, apprelease.ErrNotAnAPK):
		writeError(w, r, http.StatusUnprocessableEntity, "NOT_AN_APK", "the file is not an APK", false)
	case errors.Is(err, apprelease.ErrTooLarge):
		writeError(w, r, http.StatusRequestEntityTooLarge, "TOO_LARGE", "the APK is larger than 60 MB", false)
	case errors.Is(err, apprelease.ErrExists):
		writeError(w, r, http.StatusConflict, "VERSION_EXISTS", "this version was already uploaded; upload a new version", false)
	case err != nil:
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "TOO_LARGE", "the APK is larger than 60 MB", false)
			return
		}
		d.internalErr(w, r, "upload listener apk", err)
	default:
		writeJSON(w, http.StatusCreated, rel)
	}
}

func (d Deps) platformListListener(w http.ResponseWriter, r *http.Request) {
	rows, err := d.AppReleases.List(r.Context(), apprelease.KindQRISGateListener, 20)
	if err != nil {
		d.internalErr(w, r, "list listener apk", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// qrisgateListenerInfo: GET /v1/qrisgate/listener (tenant): the newest release, or {"available": false}.
func (d Deps) qrisgateListenerInfo(w http.ResponseWriter, r *http.Request) {
	rel, err := d.AppReleases.Latest(r.Context(), apprelease.KindQRISGateListener)
	if errors.Is(err, apprelease.ErrNone) {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		d.internalErr(w, r, "listener apk info", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "release": rel})
}

// qrisgateListenerDownload: GET /v1/qrisgate/listener/download (tenant session): streams the newest APK.
func (d Deps) qrisgateListenerDownload(w http.ResponseWriter, r *http.Request) {
	f, rel, err := d.AppReleases.OpenLatest(r.Context(), apprelease.KindQRISGateListener)
	if errors.Is(err, apprelease.ErrNone) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "no listener APK has been published yet", false)
		return
	}
	if err != nil {
		d.internalErr(w, r, "open listener apk", err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="erapay-listener-`+rel.VersionName+`.apk"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-APK-SHA256", rel.SHA256)
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "", rel.CreatedAt, f)
}
