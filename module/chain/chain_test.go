//go:build chain

package chain

import (
	"encoding/json"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
)

func info(chains string) *panel.NodeInfo {
	extra := map[string]json.RawMessage{"host": json.RawMessage(`"example.com"`)}
	if chains != "" {
		extra["chains"] = json.RawMessage(chains)
	}
	return &panel.NodeInfo{Extra: extra}
}

func TestOf(t *testing.T) {
	for _, none := range []string{"", "null", "[]"} {
		if chains, err := of(info(none)); len(chains) != 0 || err != nil {
			t.Fatalf("%q: want none; got %v, %v", none, chains, err)
		}
	}
	// unreadable: an error, so every key is refused rather than let out
	for _, broken := range []string{`{"hops":[]}`, `"x"`, `[1,2]`} {
		if _, err := of(info(broken)); err == nil {
			t.Fatalf("%s: want an error", broken)
		}
	}
	chains, err := of(info(`[{"id":3,"name":"HK-JP","hops":[{"name":"A","sing":{"type":"direct"}},{"name":"B","sing":{"type":"direct"}}]},{"id":4,"name":"none","hops":[]}]`))
	if err != nil || len(chains) != 2 || chains[0].ID != 3 || chains[0].names()[1] != "B" {
		t.Fatalf("two chains: got %v, %v", chains, err)
	}
	if chains[0].usable() != nil || chains[1].usable() == nil {
		t.Fatalf("usable: %v, %v", chains[0].usable(), chains[1].usable())
	}
	if (&spec{Hops: chains[0].Hops}).usable() == nil {
		t.Fatal("a chain without an id: want it unusable")
	}
}

func TestHopTag(t *testing.T) {
	if got := hopTag("node", 3, 2); got != "node/chain3-2" {
		t.Fatalf("got %s", got)
	}
}

// keys, as the user lists name them: a user's own key is none, a chain's key is its chain's for good
func TestKeys(t *testing.T) {
	remember([]panel.UserInfo{{Id: 1, Uuid: "own"}, {Id: 1000000000000001, Uuid: "key-a", Chain: 3}})
	if _, ok := chainOf("own"); ok {
		t.Fatal("a user's own key taken for a chain's")
	}
	if id, ok := chainOf("key-a"); !ok || id != 3 {
		t.Fatalf("key-a: got %d, %v", id, ok)
	}
	// a later list without it: still a key to chain 3 (refused, never let out)
	remember([]panel.UserInfo{{Id: 1, Uuid: "own"}})
	if id, ok := chainOf("key-a"); !ok || id != 3 {
		t.Fatalf("key-a after: got %d, %v", id, ok)
	}
}
