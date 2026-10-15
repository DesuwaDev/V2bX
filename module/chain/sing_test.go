//go:build chain && sing

package chain

import (
	"testing"

	"github.com/sagernet/sing-box/adapter"
)

type fakeOutbound struct{ adapter.Outbound }

func TestSingExit(t *testing.T) {
	keys.Store("key-3", 3)
	keys.Store("key-9", 9)
	exit := fakeOutbound{}
	r := &router{tag: "node", exits: map[int]adapter.Outbound{3: exit}}
	if out, err := r.exit("own"); out != nil || err != nil {
		t.Fatalf("a user's own key: got %v, %v; want the shared router", out, err)
	}
	if out, err := r.exit("key-3"); out != exit || err != nil {
		t.Fatalf("chain 3's key: got %v, %v", out, err)
	}
	if out, err := r.exit("key-9"); out != nil || err == nil {
		t.Fatalf("a key to a chain the node doesn't have: got %v, %v; want refused", out, err)
	}
}
