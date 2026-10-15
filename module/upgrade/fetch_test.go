//go:build upgrade && linux

package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// a release server holding one version, whose "program" prints the version it is told to
func releaseServer(t *testing.T, version, prints string, breakSum bool) *httptest.Server {
	t.Helper()
	asset, err := assetName()
	if err != nil {
		t.Skip(err)
	}
	program := []byte("#!/bin/sh\necho 'V2bX " + prints + " (A V2board backend based on multi core)'\n")
	archive := zipWith(t, map[string][]byte{"V2bX": program, "LICENSE": []byte("l")})
	sum := sha256.Sum256(archive)
	dgst := "SHA2-256= " + hex.EncodeToString(sum[:]) + "\n"
	if breakSum {
		dgst = "SHA2-256= " + strings.Repeat("0", 64) + "\n"
	}
	base := "/" + version + "/V2bX-" + asset + ".zip"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case base:
			_, _ = w.Write(archive)
		case base + ".dgst":
			_, _ = w.Write([]byte(dgst))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestFetch(t *testing.T) {
	const version = "20261017-bbbbbbb"
	for name, c := range map[string]struct {
		prints   string
		breakSum bool
		ask      string
		ok       bool
	}{
		"a good release":         {version, false, version, true},
		"a checksum that's off":  {version, true, version, false},
		"another version inside": {"20261016-aaaaaaa", false, version, false},
		"a version not released": {version, false, "20261018-ccccccc", false},
	} {
		t.Run(name, func(t *testing.T) {
			srv := releaseServer(t, version, c.prints, c.breakSum)
			defer srv.Close()
			t.Setenv("V2BX_RELEASES", srv.URL)
			next, err := fetch(c.ask, t.TempDir())
			if (err == nil) != c.ok {
				t.Fatalf("want ok=%v, got %q, %v", c.ok, next, err)
			}
		})
	}
}
