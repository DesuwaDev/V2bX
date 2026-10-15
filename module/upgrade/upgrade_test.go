//go:build upgrade

package upgrade

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersion(t *testing.T) {
	for v, want := range map[string]bool{
		"20261015-28f9218": true,
		"20261015-28f9218cd7c5ef7b83c19a3520d94bd76206d658": true,
		"chain-test":        false,
		"20261015":          false,
		"20261015-XYZ1234":  false,
		"../../etc/passwd":  false,
		"20261015-28f9218/": false,
	} {
		if releaseVersion.MatchString(v) != want {
			t.Errorf("%q: want %v", v, want)
		}
	}
}

func TestParseChecksum(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	for _, dgst := range []string{
		"MD5= 00\nSHA1= 11\nSHA2-256= " + sum + "\nSHA2-512= 22\n",
		"SHA256= " + strings.ToUpper(sum) + "\n",
	} {
		if got, err := parseChecksum(dgst); err != nil || got != sum {
			t.Errorf("%q: got %q, %v", dgst, got, err)
		}
	}
	if _, err := parseChecksum("MD5= 00\n"); err == nil {
		t.Error("no SHA-256: want an error")
	}
}

func TestOneLine(t *testing.T) {
	if got := oneLine("a\nb\r\n\tc 中"); got != "a b c ?" {
		t.Errorf("got %q", got)
	}
	if got := oneLine(strings.Repeat("x", 500)); len(got) != 200 {
		t.Errorf("length %d", len(got))
	}
}

func zipWith(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write(data)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	_ = os.WriteFile(archive, zipWith(t, map[string][]byte{"README.md": []byte("r"), "V2bX": []byte("program")}), 0o600)
	dst := filepath.Join(dir, "V2bX")
	if err := extract(archive, "V2bX", dst, 1<<20); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "program" {
		t.Fatalf("got %q", data)
	}
	if err := extract(archive, "missing", dst, 1<<20); err == nil {
		t.Error("missing file: want an error")
	}
	if err := extract(archive, "V2bX", dst, 3); err == nil {
		t.Error("over the limit: want an error")
	}
}

func TestDownloadAndVerify(t *testing.T) {
	body := []byte("zip bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ok" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	dst := filepath.Join(dir, "f")
	if err := download(srv.URL+"/ok", dst, 1<<20); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if err := verify(dst, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if err := verify(dst, strings.Repeat("0", 64)); err == nil {
		t.Error("wrong checksum: want an error")
	}
	if err := download(srv.URL+"/missing", dst, 1<<20); err == nil {
		t.Error("404: want an error")
	}
	if err := download(srv.URL+"/ok", dst, 3); err == nil {
		t.Error("over the limit: want an error")
	}
}
