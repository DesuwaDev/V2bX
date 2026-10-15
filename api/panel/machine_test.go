package panel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMachineID(t *testing.T) {
	dir := t.TempDir()
	id := filepath.Join(dir, "machine-id")
	_ = os.WriteFile(id, []byte("0123456789abcdef0123456789abcdef\n"), 0o644)
	empty := filepath.Join(dir, "empty")
	_ = os.WriteFile(empty, []byte("\n"), 0o644)
	none := func() []string { return nil }
	macs := func() []string { return []string{"52:54:00:12:34:56"} }

	got := machineID([]string{filepath.Join(dir, "missing"), empty, id}, macs)
	if got != hashID([]byte("0123456789abcdef0123456789abcdef")) || len(got) != 16 {
		t.Fatalf("the machine-id: got %q", got)
	}
	if again := machineID([]string{id}, none); again != got {
		t.Fatalf("not the same twice: %q, %q", got, again)
	}
	if got := machineID(nil, macs); got != hashID([]byte("52:54:00:12:34:56")) {
		t.Fatalf("no machine-id, the cards: got %q", got)
	}
	if got := machineID(nil, none); got != "" {
		t.Fatalf("nothing to go by: got %q", got)
	}
}
