//go:build chain

package chain

import (
	"encoding/json"
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
)

func info(chain string) *panel.NodeInfo {
	extra := map[string]json.RawMessage{"host": json.RawMessage(`"example.com"`)}
	if chain != "" {
		extra["chain"] = json.RawMessage(chain)
	}
	return &panel.NodeInfo{Extra: extra}
}

func TestOf(t *testing.T) {
	if s, err := of(info("")); s != nil || err != nil {
		t.Fatalf("no chain field: want nil, nil; got %v, %v", s, err)
	}
	if s, err := of(info("null")); s != nil || err != nil {
		t.Fatalf("chain null: want nil, nil; got %v, %v", s, err)
	}
	// chained but unusable: a spec and an error, so the node refuses rather than serving directly
	for _, broken := range []string{`{"hops":[]}`, `{"hops":"x"}`, `[1,2]`} {
		if s, err := of(info(broken)); s == nil || err == nil {
			t.Fatalf("%s: want a spec and an error; got %v, %v", broken, s, err)
		}
	}
	s, err := of(info(`{"hops":[{"name":"A","sing":{"type":"direct"}},{"name":"B","sing":{"type":"direct"}}]}`))
	if err != nil || len(s.Hops) != 2 || s.names()[1] != "B" {
		t.Fatalf("two hops: got %v, %v", s, err)
	}
}

func TestHopTag(t *testing.T) {
	if got := hopTag("node", 2); got != "node/chain-2" {
		t.Fatalf("got %s", got)
	}
}
