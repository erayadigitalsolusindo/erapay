package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
)

const sessionCookie = "erapay_session"

type ctxKey struct{}

// PrincipalFrom returns the authenticated principal set by requireAuth.
func PrincipalFrom(ctx context.Context) *auth.Principal {
	p, _ := ctx.Value(ctxKey{}).(*auth.Principal)
	return p
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Retryable bool   `json:"retryable"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, retryable bool) {
	writeJSON(w, status, map[string]apiError{"error": {code, msg, middleware.GetReqID(r.Context()), retryable}})
}

func tokenFrom(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// requireAuth authenticates the session and stores the Principal. Tenant identity
// comes only from here; headers/query/body merchant IDs are never read.
func (d Deps) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := d.Auth.Authenticate(r.Context(), tokenFrom(r))
		if errors.Is(err, auth.ErrUnauthenticated) {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", false)
			return
		}
		if err != nil {
			d.Log.Error("authenticate", "err", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
			return
		}
		if p.ImpersonatorID != "" && !d.guardImpersonation(w, r, p) {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	})
}

func requirePerm(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := PrincipalFrom(r.Context()); p == nil || !p.Has(perm) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "permission denied", false)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireTenant rejects platform sessions on tenant-scoped routes.
func requireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFrom(r.Context()); p == nil || p.MerchantID == "" {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "merchant session required", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		MerchantID string `json:"merchant_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "email and password are required", false)
		return
	}
	token, p, err := d.Auth.Login(r.Context(), auth.LoginInput{
		Email: body.Email, Password: body.Password, MerchantID: body.MerchantID,
		IP: d.clientIP(r), UserAgent: r.UserAgent(), RequestID: middleware.GetReqID(r.Context()),
	})
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		secs := 900
		var rl *auth.RateLimitedError
		if errors.As(err, &rl) {
			secs = int(math.Ceil(rl.RetryAfter.Seconds()))
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, try again later", true)
		return
	case errors.Is(err, auth.ErrMerchantRequired):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "merchant_id is required for this account", false)
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		var bc *auth.BadCredentialsError
		if errors.As(err, &bc) {
			w.Header().Set("X-Attempts-Left", strconv.Itoa(max(bc.AttemptsLeft, 0)))
		}
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", false)
		return
	case errors.Is(err, auth.ErrMFARequired):
		var mfaErr *auth.MFARequiredError
		if errors.As(err, &mfaErr) {
			writeJSON(w, http.StatusOK, map[string]any{
				"mfa_required": true,
				"challenge_id": mfaErr.ChallengeID,
				"expires_at":   mfaErr.ExpiresAt,
			})
			return
		}
		writeError(w, r, http.StatusUnauthorized, "MFA_REQUIRED", "MFA required", false)
		return
	case err != nil:
		d.Log.Error("login", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(d.Auth.SessionTTL().Seconds()),
	})
	writeJSON(w, http.StatusOK, meResponse(p))
}

func (d Deps) logout(w http.ResponseWriter, r *http.Request) {
	if err := d.Auth.Logout(r.Context(), PrincipalFrom(r.Context()), middleware.GetReqID(r.Context())); err != nil {
		d.Log.Error("logout", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

// mfaChallenge generates an MFA challenge for the authenticated user.
// Requires an active session (platform admin with MFA enrolled).
func (d Deps) mfaChallenge(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.MerchantID != "" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "platform admin session required", false)
		return
	}
	challengeID, secret, err := d.Auth.GenerateMFAChallenge(r.Context(), p.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrMFANotEnrolled) {
			writeError(w, r, http.StatusBadRequest, "MFA_NOT_ENROLLED", "MFA not enrolled", false)
			return
		}
		d.Log.Error("mfa.challenge", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"challenge_id": challengeID,
		"secret":       secret, // for QR code display (only on enrollment)
		"expires_at":   time.Now().Add(5 * time.Minute),
	})
}

// mfaVerify verifies an MFA code (TOTP or recovery code) and completes login.
// Called after login returns MFA_REQUIRED with a challenge_id.
func (d Deps) mfaVerify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		MerchantID  string `json:"merchant_id"`
		Code        string `json:"code"`
		ChallengeID string `json:"challenge_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" || body.Code == "" || body.ChallengeID == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "email, password, code, and challenge_id are required", false)
		return
	}
	token, p, err := d.Auth.CompleteMFALogin(r.Context(), auth.LoginInput{
		Email: body.Email, Password: body.Password, MerchantID: body.MerchantID,
		IP: d.clientIP(r), UserAgent: r.UserAgent(), RequestID: middleware.GetReqID(r.Context()),
	}, body.Code, body.ChallengeID)
	switch {
	case errors.Is(err, auth.ErrMFAInvalidCode):
		writeError(w, r, http.StatusUnauthorized, "MFA_INVALID_CODE", "invalid MFA code", false)
		return
	case errors.Is(err, auth.ErrMFAChallengeExpired):
		writeError(w, r, http.StatusUnauthorized, "MFA_CHALLENGE_EXPIRED", "MFA challenge expired", false)
		return
	case errors.Is(err, auth.ErrRateLimited):
		var rl *auth.RateLimitedError
		secs := 900
		if errors.As(err, &rl) {
			secs = int(math.Ceil(rl.RetryAfter.Seconds()))
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many MFA attempts, try again later", true)
		return
	case errors.Is(err, auth.ErrMerchantRequired):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "merchant_id is required for this account", false)
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		var bc *auth.BadCredentialsError
		if errors.As(err, &bc) {
			w.Header().Set("X-Attempts-Left", strconv.Itoa(max(bc.AttemptsLeft, 0)))
		}
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", false)
		return
	case err != nil:
		d.Log.Error("mfa.verify", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(d.Auth.SessionTTL().Seconds()),
	})
	writeJSON(w, http.StatusOK, meResponse(p))
}

// mfaStatus returns the current MFA enrollment status for the authenticated user.
func (d Deps) mfaStatus(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", false)
		return
	}
	info, err := d.Auth.GetMFAEnrollmentStatus(r.Context(), p.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrMFANotEnrolled) {
			writeJSON(w, http.StatusOK, map[string]any{"enrolled": false})
			return
		}
		d.Log.Error("mfa.status", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enrolled":      info.MFAEnrolled,
		"registered_at": info.MFARegisteredAt,
	})
}

// mfaEnroll starts MFA enrollment for the authenticated platform admin.
func (d Deps) mfaEnroll(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil || p.MerchantID != "" {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "platform admin session required", false)
		return
	}
	if !p.AdminMFARequired() {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "MFA enrollment not required for this role", false)
		return
	}
	enrollment, err := d.Auth.EnrollMFA(r.Context(), p.UserID, p.Email)
	if err != nil {
		if errors.Is(err, auth.ErrMFAAlreadyEnrolled) {
			writeError(w, r, http.StatusConflict, "MFA_ALREADY_ENROLLED", "MFA already enrolled", false)
			return
		}
		if errors.Is(err, auth.ErrMFAEnrollmentPending) {
			writeError(w, r, http.StatusConflict, "MFA_ENROLLMENT_PENDING", "MFA enrollment already pending", false)
			return
		}
		d.Log.Error("mfa.enroll", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":         enrollment.Secret,
		"qr_code_url":    enrollment.QRCodeURL,
		"recovery_codes": enrollment.RecoveryCodes,
		"expires_at":     enrollment.ExpiresAt,
	})
}

// mfaConfirm confirms MFA enrollment with the first TOTP code.
func (d Deps) mfaConfirm(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", false)
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "code is required", false)
		return
	}
	err := d.Auth.ConfirmMFAEnrollment(r.Context(), p.UserID, body.Code, middleware.GetReqID(r.Context()))
	if err != nil {
		if errors.Is(err, auth.ErrMFAEnrollmentExpired) {
			writeError(w, r, http.StatusBadRequest, "MFA_ENROLLMENT_EXPIRED", "MFA enrollment expired", false)
			return
		}
		if errors.Is(err, auth.ErrMFAInvalidCode) {
			writeError(w, r, http.StatusUnauthorized, "MFA_INVALID_CODE", "invalid TOTP code", false)
			return
		}
		d.Log.Error("mfa.confirm", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "MFA enrolled successfully"})
}

// mfaDisable disables MFA for the authenticated user.
func (d Deps) mfaDisable(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", false)
		return
	}
	err := d.Auth.DisableMFA(r.Context(), p.UserID, p.UserID, middleware.GetReqID(r.Context()))
	if err != nil {
		d.Log.Error("mfa.disable", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "MFA disabled successfully"})
}

// mfaRegenerateRecoveryCodes generates new recovery codes for the authenticated user.
func (d Deps) mfaRegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	if p == nil {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", false)
		return
	}
	codes, err := d.Auth.RegenerateRecoveryCodes(r.Context(), p.UserID, middleware.GetReqID(r.Context()))
	if err != nil {
		if errors.Is(err, auth.ErrMFANotEnrolled) {
			writeError(w, r, http.StatusBadRequest, "MFA_NOT_ENROLLED", "MFA not enrolled", false)
			return
		}
		d.Log.Error("mfa.regenerate_recovery_codes", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func meResponse(p *auth.Principal) map[string]any {
	perms := make([]string, 0, len(p.Permissions))
	for c := range p.Permissions {
		perms = append(perms, c)
	}
	m := map[string]any{"user_id": p.UserID, "email": p.Email, "merchant_id": p.MerchantID, "email_verified": p.EmailVerified, "permissions": perms}
	if p.ImpersonatorID != "" {
		m["impersonating"] = true
		m["impersonation_reason"] = p.ImpersonationReason
	}
	return m
}

func (d Deps) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, meResponse(PrincipalFrom(r.Context())))
}

// merchantProfile and merchantOutlets read the tenant ONLY from the Principal.
func (d Deps) merchantProfile(w http.ResponseWriter, r *http.Request) {
	prof, err := d.Merchants.Profile(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if errors.Is(err, merchant.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	if err != nil {
		d.Log.Error("profile", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

func (d Deps) merchantOutlets(w http.ResponseWriter, r *http.Request) {
	o, err := d.Merchants.Outlets(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.Log.Error("outlets", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": o})
}

// requireJSON blocks cross-site form posts (defense in depth beside SameSite=Strict).
func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, r, http.StatusUnsupportedMediaType, "VALIDATION_ERROR", "Content-Type must be application/json", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) mountV1(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		// Auth + account: JSON bodies only (requireJSON blocks cross-site form posts).
		r.Group(func(r chi.Router) {
			r.Use(requireJSON)
			r.Post("/auth/login", d.login)
			r.Post("/auth/mfa/verify", d.mfaVerify) // Complete login with MFA code
			if d.Onboarding != nil && d.Onboarding.CanRegister() {
				r.Post("/auth/register", d.register)
			}
			if d.Account != nil {
				r.Post("/auth/verify-email", d.verifyEmail)
				r.Post("/auth/resend-verification", d.resendVerification)
				r.Post("/auth/forgot-password", d.forgotPassword)
				r.Post("/auth/reset-password", d.resetPassword)
			}
		})
		if d.APIKeys != nil {
			r.With(d.requireAPIKey).Get("/api-keys/self", d.apiKeySelf)
		}
		if d.APIKeys != nil && d.Payments != nil {
			r.With(d.requireAnyAuth, requireTenant, requirePerm("payment:read")).Get("/payments/{id}", d.getPayment)
			r.With(d.requireAnyAuth, requireTenant, requirePerm("payment:read")).Get("/payments/{id}/timeline", d.paymentTimeline)
			r.With(d.requireAnyAuth, requireTenant, requirePerm("payment:read")).Get("/payments", d.listPayments)
			r.With(d.requireAnyAuth, requireTenant, requirePerm("report:read")).Get("/reports/summary", d.reportSummary)
		}
		if d.APIKeys != nil && d.PaymentFlow != nil {
			r.With(d.requireAnyAuth, requireJSON, requireTenant, requirePerm("payment:create")).Post("/payments", d.createPayment)
			// Cek status manual: menanyakan ulang status ke gateway (DOKU) berdasarkan
			// nomor referensi transaksi, lalu menerapkan hasil dengan source=inquiry.
			r.With(d.requireAnyAuth, requireJSON, requireTenant, requirePerm("payment:read")).Post("/payments/{id}/inquiry", d.paymentInquiry)
		}
		if d.APIKeys != nil && d.Settlement != nil {
			r.With(d.requireAnyAuth, requireTenant, requirePerm("report:read")).Get("/balance", d.getBalance)
			r.With(d.requireAnyAuth, requireTenant, requirePerm("settlement:read")).Get("/settlement/unsettled", d.listMyUnsettled)
		}
		d.mountPlatform(r) // panel DEWA (/platform), auth terpisah dari dashboard (/v1)
		r.Group(func(r chi.Router) {
			r.Use(d.requireAuth, requireJSON)
			r.Get("/menus", d.listMenus)
			r.Get("/banks", d.listBanks) // daftar bank Kirim DOKU untuk form rekening
			r.Post("/auth/logout", d.logout)
			r.Get("/auth/me", d.me)
			// MFA endpoints (platform admin only, gated by role in handlers)
			r.Get("/auth/mfa/status", d.mfaStatus)
			r.Post("/auth/mfa/enroll", d.mfaEnroll)
			r.Post("/auth/mfa/confirm", d.mfaConfirm)
			r.Post("/auth/mfa/disable", d.mfaDisable)
			r.Post("/auth/mfa/regenerate-recovery-codes", d.mfaRegenerateRecoveryCodes)
			// Platform (Finance / Ops) endpoints: cross-tenant by design, gated by permission only.
			r.Route("/admin", func(r chi.Router) {
				// Seluruh /admin lintas-tenant: sesi merchant ditolak walau role-nya punya izin yang sama
				// (mis. settlement:read dipakai juga oleh tampilan tenant).
				r.Use(requirePlatform)
				if d.Onboarding != nil {
					r.Group(func(r chi.Router) {
						r.Use(requirePlatform)
						r.With(requirePerm("merchant:review")).Get("/merchants", d.adminListMerchants)
						r.With(requirePerm("merchant:review")).Get("/merchants/{id}", d.adminGetMerchant)
						r.With(requirePerm("merchant:review")).Post("/merchants/{id}/start-review", d.adminReviewAction("start-review"))
						r.With(requirePerm("merchant:review")).Post("/merchants/{id}/request-changes", d.adminReviewAction("request-changes"))
						r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/approve", d.adminApproveMerchant)
						r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/reject", d.adminRejectMerchant)
						r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/revoke", d.adminRevokeMerchant)
						r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/suspend", d.adminSuspendMerchant(false))
						r.With(requirePerm("merchant:approve")).Post("/merchants/{id}/reactivate", d.adminSuspendMerchant(true))
						r.With(requirePerm("merchant:review")).Post("/merchants/{id}/verify-email", d.adminVerifyMerchantEmail)
					})
				}
				if d.KYC != nil {
					d.mountKYCAdmin(r)
				}
				if d.Settlement != nil {
					r.With(requirePerm("settlement:read")).Get("/unsettled-payments", d.listUnsettled)
					r.With(requirePerm("settlement:read")).Get("/settlement-batches", d.listSettlementBatches)
					r.With(requirePerm("settlement:read")).Get("/reconciliation-exceptions", d.listExceptions)
					r.With(requirePerm("settlement:manage")).Post("/settlement-batches", d.recordSettlementBatch)
					r.With(requirePerm("settlement:manage")).Post("/reconciliation-exceptions/{id}/resolve", d.resolveException)
				}
				if d.Refunds != nil {
					r.With(requirePerm("refund:review")).Get("/refunds", d.adminListRefunds)
					r.With(requirePerm("refund:review")).Get("/refunds/{id}", d.adminGetRefund)
					r.With(requirePerm("refund:approve")).Post("/refunds/{id}/approve", d.adminRefundAction("approve"))
					r.With(requirePerm("refund:approve")).Post("/refunds/{id}/reject", d.adminRefundAction("reject"))
					r.With(requirePerm("refund:execute")).Post("/refunds/{id}/mark-refunded", d.adminRefundAction("mark-refunded"))
					r.With(requirePerm("refund:execute")).Post("/refunds/{id}/mark-failed", d.adminRefundAction("mark-failed"))
				}
				if d.Payouts != nil {
					r.With(requirePerm("payout:review")).Get("/bank-accounts", d.adminListBankAccounts)
					r.With(requirePerm("bank_account:verify")).Post("/bank-accounts/{id}/verify", d.adminVerifyBankAccount)
					r.With(requirePerm("payout:review")).Get("/payouts", d.adminListPayouts)
					r.With(requirePerm("payout:review")).Get("/payouts/{id}", d.adminGetPayout)
					r.With(requirePerm("payout:approve")).Post("/payouts/{id}/approve", d.adminPayoutAction("approve"))
					r.With(requirePerm("payout:approve")).Post("/payouts/{id}/reject", d.adminPayoutAction("reject"))
					r.With(requirePerm("payout:execute")).Post("/payouts/{id}/execute", d.adminPayoutAction("execute"))
					r.With(requirePerm("payout:execute")).Post("/payouts/{id}/check-status", d.adminPayoutAction("check-status"))
					r.With(requirePerm("payout:execute")).Post("/payouts/{id}/mark-paid", d.adminPayoutAction("mark-paid"))
					r.With(requirePerm("payout:execute")).Post("/payouts/{id}/mark-failed", d.adminPayoutAction("mark-failed"))
					r.With(requirePerm("payout:execute")).Get("/payouts/{id}/destination", d.adminPayoutDestination)
				}
			})
			r.Group(func(r chi.Router) {
				r.Use(requireTenant)
				r.With(requirePerm("merchant:read")).Get("/merchant/profile", d.merchantProfile)
				r.With(requirePerm("outlet:read")).Get("/merchant/outlets", d.merchantOutlets)
				if d.Onboarding != nil {
					r.With(requirePerm("merchant:read")).Get("/merchant/identity", d.merchantIdentity)
					r.With(requirePerm("merchant:write")).Post("/merchant/identity", d.merchantSetIdentity)
					r.With(requirePerm("merchant:write")).Post("/merchant/resubmit", d.merchantResubmit)
				}
				if d.APIKeys != nil {
					r.Group(func(r chi.Router) {
						r.Use(requirePerm("api_key:manage"))
						r.Post("/api-keys", d.createAPIKey)
						r.Get("/api-keys", d.listAPIKeys)
						r.Post("/api-keys/{id}/revoke", d.revokeAPIKey)
						r.Post("/api-keys/{id}/rotate", d.rotateAPIKey)
					})
				}
				if d.Refunds != nil {
					// Session only, like payouts.
					r.With(requirePerm("refund:request")).Post("/payments/{id}/refund", d.requestRefund)
					r.With(requirePerm("refund:request")).Post("/refunds/{id}/cancel", d.cancelRefund)
					r.With(requirePerm("refund:read")).Get("/refunds", d.listRefunds)
					r.With(requirePerm("refund:read")).Get("/refunds/{id}", d.getRefund)
				}
				if d.Payouts != nil {
					// Session only: money-out is never reachable with an API key.
					r.With(requirePerm("bank_account:manage")).Post("/bank-accounts", d.createBankAccount)
					r.With(requirePerm("bank_account:manage")).Get("/bank-accounts", d.listBankAccounts)
					r.With(requirePerm("bank_account:manage")).Post("/bank-accounts/{id}/disable", d.disableBankAccount)
					r.With(requirePerm("payout:request")).Post("/payouts", d.requestPayout)
					r.With(requirePerm("payout:request")).Post("/payouts/{id}/cancel", d.cancelPayout)
					r.With(requirePerm("payout:read")).Get("/payouts", d.listPayouts)
					r.With(requirePerm("payout:read")).Get("/payouts/{id}", d.getPayout)
				}
				if d.Outbound != nil {
					r.Group(func(r chi.Router) {
						r.Use(requirePerm("webhook:manage"))
						r.Post("/webhook-endpoints", d.createWebhookEndpoint)
						r.Get("/webhook-endpoints", d.listWebhookEndpoints)
						r.Post("/webhook-endpoints/{id}/rotate-secret", d.rotateWebhookSecret)
						r.Post("/webhook-endpoints/{id}/disable", d.disableWebhookEndpoint)
						r.Get("/webhook-deliveries", d.listWebhookDeliveries)
						r.Post("/webhook-deliveries/{id}/redeliver", d.redeliverWebhook)
					})
				}
			})
		})
		// KYC (dashboard): unggah dokumen/logo/bukti memakai multipart dan DELETE tanpa
		// Content-Type, sehingga TIDAK berada di bawah requireJSON. requireAuth +
		// requireTenant dipasang di dalam mountKYC.
		if d.KYC != nil {
			d.mountKYC(r)
		}
	})
}

// clientIP returns the peer address. X-Forwarded-For is honoured ONLY when the
// peer is a configured trusted proxy (e.g. the SvelteKit server); its last entry
// is the one the trusted proxy appended.
func (d Deps) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, perr := netip.ParseAddr(host)
	if perr != nil {
		return host
	}
	for _, p := range d.TrustedProxies {
		if p.Contains(peer.Unmap()) {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				parts := strings.Split(xff, ",")
				if a, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
					return a.Unmap().String()
				}
			}
			break
		}
	}
	return peer.String()
}
