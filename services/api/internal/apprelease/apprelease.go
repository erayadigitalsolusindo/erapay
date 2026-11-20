// Package apprelease stores APK releases that DEWA uploads and tenants download (the QRISGate notification
// listener). The file lives on local disk under a generated name; the table keeps version, size and sha256.
// An upload is only accepted when it is a ZIP that contains AndroidManifest.xml and a classes.dex (a cheap check
// that it is an APK, not a proof that it is the right one); the sha256 is shown so tenants can compare it.
package apprelease

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

const (
	KindQRISGateListener = "qrisgate_listener"
	// MaxBytes bounds one APK (the listener is ~3 MB; this leaves room for a growing app).
	MaxBytes = 60 << 20
)

var (
	ErrInvalid  = errors.New("apprelease: invalid upload")
	ErrNotAnAPK = errors.New("apprelease: file is not an APK")
	ErrTooLarge = errors.New("apprelease: file too large")
	ErrExists   = errors.New("apprelease: this version already exists")
	ErrNone     = errors.New("apprelease: no release yet")
)

var versionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$`)

type Service struct {
	db  *pgxpool.Pool
	dir string
}

// NewService creates dir if needed (0750).
func NewService(db *pgxpool.Pool, dir string) (*Service, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Service{db: db, dir: dir}, nil
}

type Release struct {
	ID          string    `json:"id"`
	VersionName string    `json:"version_name"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"size_bytes"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Upload streams r (at most MaxBytes) to a temp file next to the target, validates it, then moves it into place
// and records it. A version string can be uploaded once; fix a bad file by uploading a new version.
func (s *Service) Upload(ctx context.Context, kind, version, note string, r io.Reader, actorID, requestID string) (Release, error) {
	if kind != KindQRISGateListener || !versionRe.MatchString(version) || utf8.RuneCountInString(note) > 500 {
		return Release{}, ErrInvalid
	}
	tmp, err := os.CreateTemp(s.dir, ".upload-*")
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, MaxBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Release{}, err
	}
	if n > MaxBytes {
		return Release{}, ErrTooLarge
	}
	if n == 0 || !looksLikeAPK(tmp.Name()) {
		return Release{}, ErrNotAnAPK
	}
	var rnd [12]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return Release{}, err
	}
	key := hex.EncodeToString(rnd[:]) + ".apk"
	if err := os.Rename(tmp.Name(), filepath.Join(s.dir, key)); err != nil {
		return Release{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		_ = os.Remove(filepath.Join(s.dir, key))
		return Release{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var rel Release
	err = tx.QueryRow(ctx, `
		INSERT INTO app_releases (kind, version_name, sha256, size_bytes, storage_key, note, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,'')::uuid)
		ON CONFLICT (kind, version_name) DO NOTHING
		RETURNING id::text, version_name, sha256, size_bytes, COALESCE(note,''), created_at`,
		kind, version, hex.EncodeToString(h.Sum(nil)), n, key, note, actorID).Scan(&rel.ID, &rel.VersionName, &rel.SHA256, &rel.SizeBytes, &rel.Note, &rel.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = os.Remove(filepath.Join(s.dir, key))
		return Release{}, ErrExists
	}
	if err == nil {
		err = audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "app_release.upload", EntityType: "app_release", EntityID: rel.ID, RequestID: requestID,
			Metadata: map[string]any{"kind": kind, "version": version, "sha256": rel.SHA256, "size": n}})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = os.Remove(filepath.Join(s.dir, key))
		return Release{}, err
	}
	return rel, nil
}

func looksLikeAPK(path string) bool {
	z, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer z.Close()
	var manifest, dex bool
	for _, f := range z.File {
		switch f.Name {
		case "AndroidManifest.xml":
			manifest = true
		case "classes.dex":
			dex = true
		}
	}
	return manifest && dex
}

// Latest returns the newest release of a kind, or ErrNone.
func (s *Service) Latest(ctx context.Context, kind string) (Release, error) {
	var rel Release
	err := s.db.QueryRow(ctx, `SELECT id::text, version_name, sha256, size_bytes, COALESCE(note,''), created_at FROM app_releases
		WHERE kind = $1 ORDER BY created_at DESC LIMIT 1`, kind).Scan(&rel.ID, &rel.VersionName, &rel.SHA256, &rel.SizeBytes, &rel.Note, &rel.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Release{}, ErrNone
	}
	return rel, err
}

func (s *Service) List(ctx context.Context, kind string, limit int) ([]Release, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, version_name, sha256, size_bytes, COALESCE(note,''), created_at FROM app_releases
		WHERE kind = $1 ORDER BY created_at DESC LIMIT $2`, kind, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Release, error) {
		var x Release
		return x, r.Scan(&x.ID, &x.VersionName, &x.SHA256, &x.SizeBytes, &x.Note, &x.CreatedAt)
	})
}

// OpenLatest opens the newest release file for download.
func (s *Service) OpenLatest(ctx context.Context, kind string) (*os.File, Release, error) {
	var rel Release
	var key string
	err := s.db.QueryRow(ctx, `SELECT id::text, version_name, sha256, size_bytes, COALESCE(note,''), created_at, storage_key FROM app_releases
		WHERE kind = $1 ORDER BY created_at DESC LIMIT 1`, kind).Scan(&rel.ID, &rel.VersionName, &rel.SHA256, &rel.SizeBytes, &rel.Note, &rel.CreatedAt, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, Release{}, ErrNone
	}
	if err != nil {
		return nil, Release{}, err
	}
	f, err := os.Open(filepath.Join(s.dir, filepath.Base(key))) // Base: the key is ours, but never trust a path
	return f, rel, err
}
