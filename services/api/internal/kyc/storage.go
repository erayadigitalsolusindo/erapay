package kyc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Storage menyimpan dokumen & bukti bisnis. Interface ini siap diganti S3/MinIO:
// implementasikan Put/Get/Delete dan pasang di cmd/api/main.go.
type Storage interface {
	// Put menyimpan isi r dan mengembalikan key internal (UUID, ekstensi dari MIME).
	Put(ctx context.Context, prefix, ext string, r io.Reader) (string, error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, key string) error
}

// extByMime memetakan MIME hasil sniffing ke ekstensi; hanya jenis yang diizinkan.
var extByMime = map[string]string{
	"application/pdf": "pdf",
	"image/jpeg":      "jpg",
	"image/png":       "png",
}

// SniffMime membaca hingga 512 byte pertama untuk deteksi MIME (bukan percaya
// Content-Type dari client), lalu mengembalikan pembaca yang utuh.
func Sniff(r io.Reader) (io.Reader, string) {
	head := make([]byte, 512)
	n, _ := io.ReadFull(r, head)
	if n < 0 {
		n = 0
	}
	// http.DetectContentType menerima bytes pendek; sisanya disambung kembali.
	return io.MultiReader(strings.NewReader(string(head[:n])), r), http.DetectContentType(head[:n])
}

// LocalFS menyimpan file di bawah Dir (mis. /var/lib/erapay/kyc).
type LocalFS struct{ Dir string }

// Batas ukuran per jenis unggahan (spesifikasi UI): Akta 25 MB, dokumen lain 15 MB, gambar 5 MB.
const (
	MaxFileBytes  int64 = 15 << 20
	MaxAktaBytes  int64 = 25 << 20
	MaxImageBytes int64 = 5 << 20
)

// Put menulis file dengan nama UUID dan ekstensi dari MIME yang sudah divalidasi.
func (l *LocalFS) Put(ctx context.Context, prefix, ext string, r io.Reader) (string, error) {
	if l == nil {
		return "", ErrStorageMissing
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	key := filepath.ToSlash(filepath.Join(prefix, hex.EncodeToString(rnd[:])+"."+ext))
	dst := filepath.Join(l.Dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		_ = os.Remove(dst)
		return "", err
	}
	return key, nil
}

// Open membuka file untuk dibaca (dipakai handler unduh + http.ServeContent).
func (l *LocalFS) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	if !validKey(key) {
		return nil, ErrNotFound
	}
	f, err := os.Open(filepath.Join(l.Dir, filepath.FromSlash(key)))
	if err != nil {
		return nil, fmt.Errorf("kyc: open: %w", err)
	}
	return f, nil
}

func (l *LocalFS) Delete(ctx context.Context, key string) error {
	if !validKey(key) {
		return nil
	}
	if err := os.Remove(filepath.Join(l.Dir, filepath.FromSlash(key))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// validKey mencegah path traversal: hanya [a-z0-9/._-] dan tanpa "..".
func validKey(k string) bool {
	if k == "" || strings.Contains(k, "..") || strings.Contains(k, "\\") {
		return false
	}
	for _, r := range k {
		ok := r == '/' || r == '-' || r == '_' || r == '.' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')
		if !ok {
			return false
		}
	}
	return true
}

// MimeTypeByExt memetakan ekstensi sederhana untuk header Content-Type unduhan.
func MimeTypeByExt(name string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(strings.ToLower(name), ".png"):
		return "image/png"
	default:
		return "image/jpeg"
	}
}
