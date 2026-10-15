//go:build chain && xray

package chain

import (
	"testing"

	"github.com/xtls/xray-core/transport/internet"
)

const chains = `[
	{"id":3,"name":"HK-JP","hops":[
		{"name":"relay","xray":{"protocol":"shadowsocks","settings":{"servers":[{"address":"192.0.2.1","port":8388,"method":"2022-blake3-aes-128-gcm","password":"AAAAAAAAAAAAAAAAAAAAAA==:AAAAAAAAAAAAAAAAAAAAAA=="}]}}},
		{"name":"exit","xray":{"protocol":"vless","settings":{"vnext":[{"address":"192.0.2.2","port":443,"users":[{"id":"6d4ef2d2-6a5c-4c88-8a2f-5b2a7d6b1f00","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"none"}}}
	]},
	{"id":4,"name":"broken","hops":[{"name":"no xray","sing":{"type":"direct"}}]}
]`

func TestXrayChains(t *testing.T) {
	keys.Store("key-3", 3)
	keys.Store("key-4", 4)
	keys.Store("key-9", 9)
	handlers, pick := xrayChains{}.Route("node", info(chains))
	if len(handlers) != 2 || handlers[0].Tag != "node/chain3-1" || handlers[1].Tag != "node/chain3-2" {
		t.Fatalf("want chain 3's two hops; got %d", len(handlers))
	}
	// the exit dials through the relay
	settings, err := handlers[1].SenderSettings.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	sockopt := settings.(interface {
		GetStreamSettings() *internet.StreamConfig
	}).GetStreamSettings().GetSocketSettings()
	if sockopt.GetDialerProxy() != "node/chain3-1" {
		t.Fatalf("exit dials through %q, want node/chain3-1", sockopt.GetDialerProxy())
	}
	for user, want := range map[string]struct {
		out   string
		taken bool
	}{
		"node|own":   {"", false},             // a user's own key: the routing rules
		"node|key-3": {"node/chain3-2", true}, // chain 3's key: its exit
		"node|key-4": {"", true},              // its chain can't be used: refused
		"node|key-9": {"", true},              // a chain the node doesn't have: refused
		"":           {"", false},
	} {
		if out, taken := pick(user); out != want.out || taken != want.taken {
			t.Errorf("%q: got %q, %v; want %q, %v", user, out, taken, want.out, want.taken)
		}
	}
}

// a node without chains, or whose chains can't be read, still refuses every chain's key
func TestXrayRefuses(t *testing.T) {
	keys.Store("key-3", 3)
	for _, c := range []string{"", `{"broken":true}`} {
		handlers, pick := xrayChains{}.Route("node", info(c))
		if len(handlers) != 0 || pick == nil {
			t.Fatalf("%q: want no outbounds and a pick", c)
		}
		if out, taken := pick("node|key-3"); out != "" || !taken {
			t.Fatalf("%q: a chain's key got %q, %v", c, out, taken)
		}
		if _, taken := pick("node|own"); taken {
			t.Fatalf("%q: a user's own key taken", c)
		}
	}
}
