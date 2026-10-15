package kyc

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

// Reviewer adalah bridge ke mekanisme review yang SUDAH ADA (onboarding.Service):
// transisi merchants.onboarding_status, verifikasi email/identitas, dan penetapan fee
// tetap lewat mekanisme lama; paket ini hanya menambah data KYC + catatan per bagian.
type Reviewer interface {
	Approve(ctx context.Context, in ApproveInput) error
	Reject(ctx context.Context, in RejectInput) error
	RequestChanges(ctx context.Context, in ReviewInput) error
	StartReview(ctx context.Context, in ReviewInput) error
}

// Input bridge dipetakan 1:1 ke tipe onboarding oleh adapter kecil di cmd/api.
type (
	ApproveInput struct {
		MerchantID  string
		ActorID     string
		MDRBPS      int
		PlatformBPS int
		// Nominal tetap per transaksi (rupiah); 0 = tidak ada.
		MDRFixed      int64
		PlatformFixed int64
		RequestID     string
	}
	RejectInput struct {
		MerchantID string
		ActorID    string
		Note       string
		RequestID  string
	}
	ReviewInput struct {
		MerchantID string
		ActorID    string
		Note       string
		RequestID  string
	}
)

type Config struct {
	Box     *secretbox.Box
	Storage Storage
	Log     *slog.Logger
	// RecheckPolicy: akses produksi saat NEEDS_REVIEW_AGAIN.
	// "open" (default): tetap terbuka, ditandai review ulang. "closed": produksi diblokir.
	RecheckPolicy string
	// UploadPerIP / SubmitPerIP: batas per IP per jam (throttle DB yang sudah ada).
	UploadPerIP, SubmitPerIP int
}

type Service struct {
	db       *pgxpool.Pool
	repo     *Repo
	reviewer Reviewer // di-set cmd/api memakai *onboarding.Service yang sudah ada
	cfg      Config
}

func NewService(db *pgxpool.Pool, cfg Config) *Service {
	if cfg.RecheckPolicy == "" {
		cfg.RecheckPolicy = "open"
	}
	if cfg.UploadPerIP <= 0 {
		cfg.UploadPerIP = 60
	}
	if cfg.SubmitPerIP <= 0 {
		cfg.SubmitPerIP = 10
	}
	return &Service{db: db, cfg: cfg, repo: NewRepo(db)}
}

// SetReviewer memasang bridge ke onboarding.Service (mekanisme review yang sudah ada).
func (s *Service) SetReviewer(r Reviewer) { s.reviewer = r }

// seal menyegel nilai kuat (NIK/NPWP) dengan AAD = merchant id (pola bank account).
func (s *Service) seal(value, aad string) (sealed []byte, last4 string, err error) {
	if value == "" {
		return nil, "", nil
	}
	sealed, err = s.cfg.Box.Seal([]byte(value), []byte(aad))
	if err != nil {
		return nil, "", err
	}
	return sealed, value[len(value)-4:], nil
}

// openSealed membuka nilai tersegel untuk reveal (pemilik mode edit / reviewer).
func (s *Service) openSealed(sealed []byte, aad string) (string, error) {
	if s.cfg.Box == nil || len(sealed) == 0 {
		return "", errors.New("kyc: value is not readable")
	}
	v, err := s.cfg.Box.Open(sealed, []byte(aad))
	if err != nil {
		return "", err
	}
	return string(v), nil
}

// ProductionAllowed memutuskan gerbang fitur produksi: hanya approved yang membuka;
// recheck (NEEDS_REVIEW_AGAIN) mengikuti kebijakan (default tetap terbuka).
func (s *Service) ProductionAllowed(status string, recheck bool) bool {
	return status == "approved" && (s.cfg.RecheckPolicy != "closed" || !recheck)
}

// ProductionAccess membaca status + recheck dari DB untuk gate endpoint produksi.
// Mengembalikan (boleh, status saat ini, error). Merchant tanpa baris kyc_submissions
// diperlakukan belum pernah submit (recheck=false).
func (s *Service) ProductionAccess(ctx context.Context, merchantID string) (bool, string, error) {
	m, err := s.repo.merchantRow(ctx, merchantID)
	if err != nil {
		return false, "", err
	}
	sub, err := s.repo.submission(ctx, s.db, merchantID)
	recheck := false
	if err == nil {
		recheck = sub.Recheck
	} else if !errors.Is(err, ErrNotFound) {
		return false, m.Status, err
	}
	return s.ProductionAllowed(m.Status, recheck), m.Status, nil
}
