package apprelease

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range names {
		f, _ := w.Create(n)
		_, _ = f.Write([]byte("x-" + n))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestUploadValidatesStoresAndServesTheLatest(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	s, err := NewService(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	apk := zipOf(t, "AndroidManifest.xml", "classes.dex", "res/x")

	for name, c := range map[string]struct {
		version string
		body    []byte
		want    error
	}{
		"not a zip":        {"1.0.0", []byte("hello"), ErrNotAnAPK},
		"zip without dex":  {"1.0.0", zipOf(t, "AndroidManifest.xml"), ErrNotAnAPK},
		"empty":            {"1.0.0", nil, ErrNotAnAPK},
		"bad version":      {"../etc", apk, ErrInvalid},
		"version too long": {strings.Repeat("1", 33), apk, ErrInvalid},
	} {
		if _, err := s.Upload(ctx, KindQRISGateListener, c.version, "", bytes.NewReader(c.body), "", ""); !errors.Is(err, c.want) {
			t.Errorf("%s: %v; want %v", name, err, c.want)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("rejected uploads left files behind: %v", entries)
	}
	if _, err := s.Latest(ctx, KindQRISGateListener); !errors.Is(err, ErrNone) {
		t.Fatalf("no release yet: %v", err)
	}

	rel, err := s.Upload(ctx, KindQRISGateListener, "1.0.0", "rilis awal", bytes.NewReader(apk), "", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(apk)
	if rel.SHA256 != hex.EncodeToString(sum[:]) || rel.SizeBytes != int64(len(apk)) || rel.Note != "rilis awal" {
		t.Fatalf("release %+v", rel)
	}
	if _, err := s.Upload(ctx, KindQRISGateListener, "1.0.0", "", bytes.NewReader(apk), "", ""); !errors.Is(err, ErrExists) {
		t.Fatalf("same version twice: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("expected exactly one stored file, got %d", len(entries))
	}
	apk2 := zipOf(t, "AndroidManifest.xml", "classes.dex", "res/y")
	if _, err := s.Upload(ctx, KindQRISGateListener, "1.0.1", "", bytes.NewReader(apk2), "", ""); err != nil {
		t.Fatal(err)
	}
	f, latest, err := s.OpenLatest(ctx, KindQRISGateListener)
	if err != nil || latest.VersionName != "1.0.1" {
		t.Fatalf("latest %+v %v", latest, err)
	}
	defer f.Close()
	got, _ := io.ReadAll(f)
	if !bytes.Equal(got, apk2) {
		t.Fatal("downloaded bytes differ from the uploaded APK")
	}
	if list, _ := s.List(ctx, KindQRISGateListener, 10); len(list) != 2 || list[0].VersionName != "1.0.1" {
		t.Fatalf("list %+v", list)
	}
}

func TestConcurrentUploadOfOneVersionKeepsExactlyOne(t *testing.T) {
	db := testdb.New(t)
	dir := t.TempDir()
	s, _ := NewService(db, dir)
	apk := zipOf(t, "AndroidManifest.xml", "classes.dex")
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Upload(context.Background(), KindQRISGateListener, "2.0.0", "", bytes.NewReader(apk), "", "")
			if err == nil {
				ok.Add(1)
			} else if !errors.Is(err, ErrExists) {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if entries, _ := os.ReadDir(dir); ok.Load() != 1 || len(entries) != 1 {
		t.Fatalf("%d successes, %d files; want 1 and 1", ok.Load(), len(entries))
	}
}
