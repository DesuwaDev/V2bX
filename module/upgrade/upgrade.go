//go:build upgrade

// Package upgrade puts another release of this program in its place when the panel asks
// ("X-V2bX-Upgrade: <version>" on an answer to a node's config request: an admin clicked 更新 for
// the node), and never by itself.
//
// The release comes only from this fork's releases (DesuwaDev/V2bX, or V2BX_RELEASES, a mirror of
// them), so the panel can choose a version but not a program. Its zip is checked against its
// SHA-256 and the program in it run once before it replaces this one, which then restarts into it
// in place (same process, same arguments). If the new version can't start its nodes, the old one
// comes back and says why. Nothing stays behind: the download goes before the restart, the old
// program as soon as the new one runs.
package upgrade

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/module"
	log "github.com/sirupsen/logrus"
)

// Feature is this feature's name, as the panel hears it (module.Register)
const Feature = "upgrade"

const defaultReleases = "https://github.com/DesuwaDev/V2bX/releases/download"

// release versions: YYYYMMDD-<commit>
var releaseVersion = regexp.MustCompile(`^\d{8}-[0-9a-f]{7,40}$`)

var (
	busy atomic.Bool
	mu   sync.Mutex
	// "version: why" of the last update that failed, told to the panel in every request until the next try
	failure string
	tried   = map[string]time.Time{}
)

func init() {
	module.Register(Feature, "sing", "xray", "hysteria2")
	panel.OnAnswer(func(_ int, header http.Header) {
		if version := header.Get("X-V2bX-Upgrade"); version != "" {
			ask(version)
		}
	})
	panel.AddHeaders(func() map[string]string {
		mu.Lock()
		defer mu.Unlock()
		if failure == "" {
			return nil
		}
		return map[string]string{"X-V2bX-Upgrade-Error": failure}
	})
	module.OnStarted(started)
}

// ask starts the update to a version, unless one is under way or that one was tried a moment ago
func ask(version string) {
	if !releaseVersion.MatchString(version) || version == panel.ProgramVersion {
		return
	}
	mu.Lock()
	if at, ok := tried[version]; ok && time.Since(at) < 2*time.Minute {
		mu.Unlock()
		return
	}
	tried[version] = time.Now()
	failure = ""
	mu.Unlock()
	if !busy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer busy.Store(false)
		log.WithField("version", version).Info("Updating V2bX, as the panel asks")
		if err := update(version); err != nil {
			why := oneLine(err.Error())
			log.WithField("version", version).WithField("err", why).Error("Update failed")
			mu.Lock()
			failure = version + ": " + why
			mu.Unlock()
		}
	}()
}

// update puts the release in this program's place and restarts into it; it only returns when that
// didn't happen, with the running program where it was
func update(version string) error {
	self, err := executable()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(self), ".update-")
	if err != nil {
		return fmt.Errorf("temporary directory: %w", err)
	}
	defer os.RemoveAll(tmp)
	next, err := fetch(version, tmp)
	if err != nil {
		return err
	}
	// the running program aside until the new one runs, the new one where it was
	old := self + ".old"
	_ = os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		return fmt.Errorf("move the program aside: %w", err)
	}
	if err := os.Rename(next, self); err != nil {
		_ = os.Rename(old, self)
		return fmt.Errorf("put the new program in place: %w", err)
	}
	// the download goes now: the restart never comes back to remove it
	_ = os.RemoveAll(tmp)
	log.WithField("version", version).Info("Restarting into the new version")
	err = restart(self)
	_ = os.Rename(old, self)
	return fmt.Errorf("restart: %w", err)
}

// fetch downloads the release into dir, checks it and returns its program
func fetch(version, dir string) (string, error) {
	asset, err := assetName()
	if err != nil {
		return "", err
	}
	url := releases() + "/" + version + "/V2bX-" + asset + ".zip"
	archive := filepath.Join(dir, "V2bX.zip")
	if err := download(url, archive, 300<<20); err != nil {
		return "", err
	}
	sum, err := checksum(url + ".dgst")
	if err != nil {
		return "", err
	}
	if err := verify(archive, sum); err != nil {
		return "", err
	}
	next := filepath.Join(dir, "V2bX")
	if err := extract(archive, "V2bX", next, 400<<20); err != nil {
		return "", err
	}
	out, err := exec.Command(next, "version").CombinedOutput()
	if err != nil || !strings.Contains(string(out), version) {
		return "", fmt.Errorf("the downloaded program doesn't run here or isn't %s (%s)", version, oneLine(string(out)))
	}
	return next, nil
}

// started settles an update once the new program has started (or not): the old one goes, or comes
// back if this one can't run its nodes
func started(err error) {
	self, e := executable()
	if e != nil {
		return
	}
	old := self + ".old"
	marker := self + ".update-failed"
	if data, e := os.ReadFile(marker); e == nil {
		_ = os.Remove(marker)
		mu.Lock()
		failure = oneLine(string(data))
		mu.Unlock()
	}
	if _, e := os.Stat(old); e != nil {
		return
	}
	if err == nil {
		if os.Remove(old) == nil {
			log.WithField("version", panel.ProgramVersion).Info("Updated, the old version removed")
		}
		return
	}
	// this version can't start its nodes: back to the one before, which tells the panel why
	_ = os.WriteFile(marker, []byte(panel.ProgramVersion+": did not start: "+err.Error()), 0o600)
	if os.Rename(old, self) != nil {
		_ = os.Remove(marker)
		return
	}
	log.WithField("err", err).Error("The new version didn't start, back to the old one")
	_ = restart(self)
}

func executable() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("where this program is: %w", err)
	}
	return filepath.EvalSymlinks(self)
}

// assetName is the release zip for this machine (.github/workflows/release.yml)
func assetName() (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("updates are only for Linux")
	}
	switch runtime.GOARCH {
	case "amd64":
		return "linux-64", nil
	case "arm64":
		return "linux-arm64-v8a", nil
	}
	return "", fmt.Errorf("no release for %s", runtime.GOARCH)
}

func releases() string {
	if base := strings.TrimRight(os.Getenv("V2BX_RELEASES"), "/"); base != "" {
		return base
	}
	return defaultReleases
}

var client = &http.Client{Timeout: 10 * time.Minute}

func download(url, dst string, limit int64) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", path.Base(url), resp.Status)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download %s: %w", path.Base(url), err)
	}
	if n > limit {
		return fmt.Errorf("download %s: too large", path.Base(url))
	}
	return nil
}

// the release's SHA-256, from its .dgst ("SHA2-256= <hex>", openssl's output)
var sha256Line = regexp.MustCompile(`(?im)^SHA2?-?256\s*=\s*([0-9a-f]{64})\s*$`)

func checksum(url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("checksum: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("checksum: %w", err)
	}
	return parseChecksum(string(body))
}

func parseChecksum(dgst string) (string, error) {
	m := sha256Line.FindStringSubmatch(dgst)
	if m == nil {
		return "", errors.New("checksum: no SHA-256 in the .dgst")
	}
	return strings.ToLower(m[1]), nil
}

func verify(file, sum string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != sum {
		return errors.New("the download doesn't match its SHA-256")
	}
	return nil
}

func extract(archive, name, dst string, limit int64) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open the zip: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != name || f.FileInfo().IsDir() {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		n, err := io.Copy(out, io.LimitReader(src, limit+1))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if n > limit {
			return errors.New("the program in the zip is too large")
		}
		return nil
	}
	return fmt.Errorf("no %s in the zip", name)
}

// oneLine fits a message into a header: one line of printable ASCII, 200 characters at most
func oneLine(s string) string {
	var b strings.Builder
	for _, r := range strings.Join(strings.Fields(s), " ") {
		if r < 0x20 || r > 0x7e {
			r = '?'
		}
		b.WriteRune(r)
		if b.Len() >= 200 {
			break
		}
	}
	return b.String()
}
