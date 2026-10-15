package kyc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo mengumpulkan kueri KYC. Setiap metode scoped ke merchantID yang datang dari
// sesi (Principal), bukan dari request (PRD §0 #7).
type Repo struct{ db *pgxpool.Pool }

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

// Querier dipenuhi *pgxpool.Pool maupun pgx.Tx (dipakai di dalam transaksi).
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// rawMerchant adalah baris merchants untuk KYC. Nilai tersegel (NIK/NPWP) tidak pernah
// keluar dari paket ini kecuali lewat reveal (pemilik, mode edit) / reviewer.
type rawMerchant struct {
	Status                                                             string
	ReviewNote                                                         *string
	BusinessID                                                         *string
	EntityType                                                         *string
	CompanyName                                                        string
	Phone                                                              *string
	Email                                                              *string
	PostalCode, Kelurahan, Kecamatan, Kota, Provinsi, Detail           *string
	OwnerName, OwnerPosition, OwnerNationality, OwnerPhone, OwnerEmail *string
	IDLast4                                                            *string
	idEnc                                                              []byte
	BizLast4                                                           *string
	bizEnc                                                             []byte
	NIB                                                                *string
	KBLI                                                               *string
}

const merchantCols = `m.onboarding_status, m.review_note, m.business_id, m.entity_type, m.legal_name,
	m.contact_phone, m.contact_email, m.addr_postal_code, m.addr_kelurahan, m.addr_kecamatan,
	m.addr_kota, m.addr_provinsi, m.addr_detail,
	m.owner_name, m.owner_position, m.owner_nationality, m.owner_phone, m.owner_email,
	m.owner_id_last4, m.owner_id_enc, m.business_npwp_last4, m.business_npwp_enc,
	m.business_nib, m.business_kbli`

func scanMerchantRow(row pgx.Row) (rawMerchant, error) {
	var r rawMerchant
	err := row.Scan(&r.Status, &r.ReviewNote, &r.BusinessID, &r.EntityType, &r.CompanyName,
		&r.Phone, &r.Email, &r.PostalCode, &r.Kelurahan, &r.Kecamatan,
		&r.Kota, &r.Provinsi, &r.Detail,
		&r.OwnerName, &r.OwnerPosition, &r.OwnerNationality, &r.OwnerPhone, &r.OwnerEmail,
		&r.IDLast4, &r.idEnc, &r.BizLast4, &r.bizEnc,
		&r.NIB, &r.KBLI)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (r *Repo) merchantRow(ctx context.Context, id string) (rawMerchant, error) {
	return scanMerchantRow(r.db.QueryRow(ctx,
		`SELECT `+merchantCols+` FROM merchants m WHERE m.id::text = $1`, id))
}

// submission: penanda proses KYC; status tetap merchants.onboarding_status.
type submission struct {
	SubmittedAt, ReviewedAt *time.Time
	Reviewer                *string
	Recheck                 bool
	RecheckSections         []string
	recheckRaw              []byte
}

func (s submission) rechecking(section string) bool {
	for _, x := range s.RecheckSections {
		if x == section {
			return true
		}
	}
	return false
}

// Region: satu baris wilayah untuk autocomplete alamat.
type Region struct {
	PostalCode string `json:"postal_code"`
	Kelurahan  string `json:"kelurahan"`
	Kecamatan  string `json:"kecamatan"`
	Kota       string `json:"kota"`
	Provinsi   string `json:"provinsi"`
}

// submissionRow: penanda proses KYC; status tetap merchants.onboarding_status.
func (r *Repo) submission(ctx context.Context, q Querier, merchantID string) (submission, error) {
	var s submission
	err := q.QueryRow(ctx, `SELECT submitted_at, reviewed_at, reviewer_id::text, recheck, recheck_sections
		FROM kyc_submissions WHERE merchant_id = $1`, merchantID).
		Scan(&s.SubmittedAt, &s.ReviewedAt, &s.Reviewer, &s.Recheck, &s.recheckRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	if err == nil {
		s.RecheckSections = parseSections(s.recheckRaw)
	}
	return s, err
}

// parseSections membaca kolom recheck_sections (jsonb array of string) dengan aman.
func parseSections(b []byte) []string {
	var out []string
	_ = json.Unmarshal(b, &out)
	return out
}

func (r *Repo) ensureSubmission(ctx context.Context, q Querier, merchantID string) error {
	_, err := q.Exec(ctx, `INSERT INTO kyc_submissions (merchant_id) VALUES ($1) ON CONFLICT DO NOTHING`, merchantID)
	return err
}

type brandRow struct {
	ID, BrandID                 string
	Name, Category, Description *string
	Amount, Count               *int64
	LogoKey, LogoMime, LogoName *string
	LogoSize                    *int64
}

func (r *Repo) brandRow(ctx context.Context, q Querier, merchantID string) (brandRow, error) {
	var b brandRow
	err := q.QueryRow(ctx, `SELECT id::text, brand_id, name, category, description,
		monthly_amount, monthly_count, logo_key, logo_mime, logo_name, logo_size
		FROM merchant_brands WHERE merchant_id = $1`, merchantID).
		Scan(&b.ID, &b.BrandID, &b.Name, &b.Category, &b.Description, &b.Amount, &b.Count,
			&b.LogoKey, &b.LogoMime, &b.LogoName, &b.LogoSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

type docRow struct {
	ID      string
	DocType string
	Key     string
	Name    string
	Mime    string
	Size    int64
	At      time.Time
}

func (r *Repo) docs(ctx context.Context, q Querier, merchantID string) ([]docRow, error) {
	rows, err := q.Query(ctx, `SELECT id::text, doc_type, storage_key, original_name, mime, size_bytes, created_at
		FROM merchant_documents WHERE merchant_id = $1 ORDER BY created_at`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []docRow{}
	for rows.Next() {
		var d docRow
		if err := rows.Scan(&d.ID, &d.DocType, &d.Key, &d.Name, &d.Mime, &d.Size, &d.At); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type evidenceRow struct {
	ID   string
	Key  string
	Name string
	Mime string
	Size int64
}

func (r *Repo) evidences(ctx context.Context, q Querier, brandID string) ([]evidenceRow, error) {
	rows, err := q.Query(ctx, `SELECT id::text, storage_key, original_name, mime, size_bytes
		FROM merchant_brand_evidences WHERE brand_id = $1 ORDER BY created_at`, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []evidenceRow{}
	for rows.Next() {
		var e evidenceRow
		if err := rows.Scan(&e.ID, &e.Key, &e.Name, &e.Mime, &e.Size); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) links(ctx context.Context, q Querier, brandID string) ([]Link, error) {
	rows, err := q.Query(ctx, `SELECT id::text, url FROM merchant_brand_links WHERE brand_id = $1 ORDER BY created_at, id`, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.URL); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repo) notes(ctx context.Context, q Querier, merchantID string) ([]Note, error) {
	rows, err := q.Query(ctx, `SELECT id::text, section, COALESCE(field,''), note, resolved
		FROM kyc_review_notes WHERE merchant_id = $1 ORDER BY created_at`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Section, &n.Field, &n.Note, &n.Resolved); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Regions mencari wilayah untuk autocomplete alamat (kode pos / kelurahan / kota).
func (r *Repo) Regions(ctx context.Context, q string, limit int) ([]Region, error) {
	lq := "%" + lower(q) + "%"
	rows, err := r.db.Query(ctx, `SELECT postal_code, kelurahan, kecamatan, kota, provinsi FROM kyc_regions
		WHERE postal_code LIKE $1 OR lower(kelurahan) LIKE $2 OR lower(kota) LIKE $2
		ORDER BY postal_code, kelurahan LIMIT $3`, q+"%", lq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Region{}
	for rows.Next() {
		var v Region
		if err := rows.Scan(&v.PostalCode, &v.Kelurahan, &v.Kecamatan, &v.Kota, &v.Provinsi); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
