package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

// The panel's session cookie is httpOnly on the web origin, so the browser cannot send it to the API origin. Instead the
// web server asks for a one-time ticket (as DEWA) and the browser uploads the APK straight here with it.
// shortcut: tickets live in memory (lost on restart, not shared between API instances); use Redis if the API is scaled out.
const apkTicketTTL = 2 * time.Minute

var apkTickets = struct {
	sync.Mutex
	m map[string]apkTicket
}{m: map[string]apkTicket{}}

type apkTicket struct {
	userID string
	exp    time.Time
}

func issueAPKTicket(userID string) (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b[:])
	apkTickets.Lock()
	defer apkTickets.Unlock()
	now := time.Now()
	for k, v := range apkTickets.m { // drop expired so the map cannot grow
		if now.After(v.exp) {
			delete(apkTickets.m, k)
		}
	}
	apkTickets.m[t] = apkTicket{userID, now.Add(apkTicketTTL)}
	return t, nil
}

// takeAPKTicket consumes the ticket: it works once, even when the upload then fails.
func takeAPKTicket(t string) (string, bool) {
	apkTickets.Lock()
	defer apkTickets.Unlock()
	v, ok := apkTickets.m[t]
	delete(apkTickets.m, t)
	return v.userID, ok && time.Now().Before(v.exp)
}

// platformAPKTicket: POST /v1/platform/listener-apk/ticket (DEWA session) -> {"ticket": "...", "expires_in": 120}.
func (d Deps) platformAPKTicket(w http.ResponseWriter, r *http.Request) {
	t, err := issueAPKTicket(PrincipalFrom(r.Context()).UserID)
	if err != nil {
		d.internalErr(w, r, "apk ticket", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "expires_in": int(apkTicketTTL.Seconds())})
}

// platformUploadListenerDirect: POST /v1/platform/listener-apk/upload?ticket=...&version=...&note=... from the browser
// (CORS only for the dashboard origin), raw APK as the body. Mounted outside the session middleware: the ticket is the auth.
func (d Deps) platformUploadListenerDirect(w http.ResponseWriter, r *http.Request) {
	if o := r.Header.Get("Origin"); o != "" && o == d.WebURL {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", o)
		h.Set("Vary", "Origin")
		h.Set("Access-Control-Allow-Methods", "POST")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		h.Set("Access-Control-Max-Age", "600")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	uid, ok := takeAPKTicket(r.URL.Query().Get("ticket"))
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "upload ticket is invalid or expired; request a new one", false)
		return
	}
	// The server-wide Read/WriteTimeout (60/75 s) would cut a 60 MB upload on a slow link.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(10 * time.Minute))
	_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Minute))
	d.platformUploadListener(w, r.WithContext(contextWithPrincipal(r, &auth.Principal{UserID: uid})))
}
