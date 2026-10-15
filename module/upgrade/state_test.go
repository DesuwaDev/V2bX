//go:build upgrade

package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const ticket = "0123abcd"

// a program in a directory of its own, and the restart that would replace this process, recorded
func fakeProgram(t *testing.T) (self string, restarts *[]string) {
	t.Helper()
	self = filepath.Join(t.TempDir(), "V2bX")
	if err := os.WriteFile(self, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	restarts = &[]string{}
	oldExecutable, oldRestart := executable, restart
	executable = func() (string, error) { return self, nil }
	restart = func(path string) error {
		*restarts = append(*restarts, path)
		return errors.New("not in a test")
	}
	t.Cleanup(func() {
		executable, restart = oldExecutable, oldRestart
		mu.Lock()
		busy, lastTry, tried, failure, failedTicket = false, time.Time{}, map[string]bool{}, "", ""
		mu.Unlock()
	})
	return self, restarts
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note")
	if err := writeNote(path, ticket, "20261017-bbbbbbb", ""); err != nil {
		t.Fatal(err)
	}
	if tk, v, why, ok := readNote(path); !ok || tk != ticket || v != "20261017-bbbbbbb" || why != "" {
		t.Fatalf("got %q %q %q %v", tk, v, why, ok)
	}
	_ = writeNote(path, ticket, "20261017-bbbbbbb", "did not start:\nboom")
	if _, _, why, ok := readNote(path); !ok || why != "did not start: boom" {
		t.Fatalf("got %q %v", why, ok)
	}
	for _, bad := range []string{"", "0123abcd", "0123abcd ../x", "XYZ 20261017-bbbbbbb", "0123abcd 20261017-bbbbbbb extra"} {
		_ = os.WriteFile(path, []byte(bad), 0o600)
		if _, _, _, ok := readNote(path); ok {
			t.Errorf("%q: want not ok", bad)
		}
	}
	if _, _, _, ok := readNote(path + ".missing"); ok {
		t.Error("missing: want not ok")
	}
}

func TestStartedKeepsTheNewVersion(t *testing.T) {
	self, restarts := fakeProgram(t)
	_ = os.WriteFile(self+".old", []byte("old"), 0o755)
	_ = writeNote(self+".update", ticket, "20261017-bbbbbbb", "")
	started(nil)
	if exists(self+".old") || exists(self+".update") || len(*restarts) != 0 {
		t.Fatalf("old %v, note %v, restarts %v", exists(self+".old"), exists(self+".update"), *restarts)
	}
	if data, _ := os.ReadFile(self); string(data) != "new" {
		t.Fatalf("program %q", data)
	}
}

func TestStartedGoesBack(t *testing.T) {
	self, restarts := fakeProgram(t)
	_ = os.WriteFile(self+".old", []byte("old"), 0o755)
	_ = writeNote(self+".update", ticket, "20261017-bbbbbbb", "")
	started(errors.New("Run nodes failed"))
	if data, _ := os.ReadFile(self); string(data) != "old" || exists(self+".old") {
		t.Fatalf("program %q, old %v", data, exists(self+".old"))
	}
	if len(*restarts) != 1 || (*restarts)[0] != self {
		t.Fatalf("restarts %v", *restarts)
	}
	// the old program, back, says why and doesn't try that ticket again
	load()
	if exists(self + ".update") {
		t.Fatal("the note stayed")
	}
	if failure != "20261017-bbbbbbb: did not start: Run nodes failed" || failedTicket != ticket || !tried[ticket] {
		t.Fatalf("failure %q, ticket %q, tried %v", failure, failedTicket, tried)
	}
}

func TestStartedWithoutAnUpdate(t *testing.T) {
	self, restarts := fakeProgram(t)
	started(errors.New("boom"))
	started(nil)
	if data, _ := os.ReadFile(self); string(data) != "new" || len(*restarts) != 0 {
		t.Fatalf("program %q, restarts %v", data, *restarts)
	}
}

func TestLoadLeavesAStartingVersion(t *testing.T) {
	self, _ := fakeProgram(t)
	_ = os.WriteFile(self+".old", []byte("old"), 0o755)
	_ = writeNote(self+".update", ticket, "20261017-bbbbbbb", "")
	load()
	if !exists(self+".update") || failure != "" {
		t.Fatalf("note %v, failure %q", exists(self+".update"), failure)
	}
}

func TestLoadDropsANoteWithoutFailure(t *testing.T) {
	self, _ := fakeProgram(t)
	_ = writeNote(self+".update", ticket, "20261017-bbbbbbb", "")
	load()
	if exists(self+".update") || failure != "" || tried[ticket] {
		t.Fatalf("note %v, failure %q, tried %v", exists(self+".update"), failure, tried)
	}
}

// waits for the try ask started to end
func settled(t *testing.T) {
	t.Helper()
	for i := 0; i < 200; i++ {
		mu.Lock()
		done := !busy
		mu.Unlock()
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the try didn't end")
}

func TestAsk(t *testing.T) {
	self, _ := fakeProgram(t)
	// nowhere to download from: the try fails at once, which is all this needs
	t.Setenv("V2BX_RELEASES", "http://127.0.0.1:1")
	for _, c := range [][2]string{{"latest", ticket}, {"20261017-bbbbbbb", ""}, {"20261017-bbbbbbb", "0123"}, {"20261017-bbbbbbb", "../../x"}} {
		ask(c[0], c[1])
		if tried[c[1]] || !lastTry.IsZero() {
			t.Fatalf("%q: tried", c)
		}
	}
	ask("20261017-bbbbbbb", ticket)
	settled(t)
	if !tried[ticket] || failure == "" || failedTicket != ticket {
		t.Fatalf("tried %v, failure %q, ticket %q", tried, failure, failedTicket)
	}
	if !strings.HasPrefix(failure, "20261017-bbbbbbb: ") {
		t.Fatalf("failure %q", failure)
	}
	// that ticket again, or another one a moment later: nothing
	first := lastTry
	ask("20261017-bbbbbbb", ticket)
	ask("20261017-bbbbbbb", "4567cdef")
	if lastTry != first || tried["4567cdef"] {
		t.Fatal("tried again")
	}
	// later, the other ticket goes, and the failure it replaces is gone meanwhile
	mu.Lock()
	lastTry = time.Now().Add(-time.Minute)
	mu.Unlock()
	ask("20261017-bbbbbbb", "4567cdef")
	settled(t)
	if !tried["4567cdef"] || failedTicket != "4567cdef" {
		t.Fatalf("tried %v, ticket %q", tried, failedTicket)
	}
	if exists(self+".update") || exists(self+".old") {
		t.Fatal("a failed try left files")
	}
	entries, _ := os.ReadDir(filepath.Dir(self))
	if len(entries) != 1 {
		t.Fatalf("left behind: %v", entries)
	}
}
