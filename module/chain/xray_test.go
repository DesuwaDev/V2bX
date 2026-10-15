//go:build chain && xray

package chain

import (
	"testing"

	"github.com/xtls/xray-core/transport/internet"
)

const hops = `{"hops":[
	{"name":"relay","xray":{"protocol":"shadowsocks","settings":{"servers":[{"address":"192.0.2.1","port":8388,"method":"2022-blake3-aes-128-gcm","password":"AAAAAAAAAAAAAAAAAAAAAA==:AAAAAAAAAAAAAAAAAAAAAA=="}]}}},
	{"name":"exit","xray":{"protocol":"vless","settings":{"vnext":[{"address":"192.0.2.2","port":443,"users":[{"id":"6d4ef2d2-6a5c-4c88-8a2f-5b2a7d6b1f00","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"none"}}}
]}`

func TestXrayChain(t *testing.T) {
	handlers, exit := xrayChain{}.Outbounds("node", info(hops))
	if exit != "node/chain-2" || len(handlers) != 2 {
		t.Fatalf("want two hops, out through node/chain-2; got %d, %s", len(handlers), exit)
	}
	if handlers[0].Tag != "node/chain-1" || handlers[1].Tag != "node/chain-2" {
		t.Fatalf("tags: %s, %s", handlers[0].Tag, handlers[1].Tag)
	}
	// the exit dials through the relay
	settings, err := handlers[1].SenderSettings.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	sockopt := settings.(interface {
		GetStreamSettings() *internet.StreamConfig
	}).GetStreamSettings().GetSocketSettings()
	if sockopt.GetDialerProxy() != "node/chain-1" {
		t.Fatalf("exit dials through %q, want node/chain-1", sockopt.GetDialerProxy())
	}
}

func TestXrayChainRefuses(t *testing.T) {
	for _, broken := range []string{
		`{"hops":[]}`,
		`{"hops":[{"name":"no xray","sing":{"type":"direct"}}]}`,
		`{"hops":[{"name":"null xray","xray":null}]}`,
		`{"hops":[{"name":"bad","xray":{"protocol":"no-such-protocol"}}]}`,
	} {
		handlers, exit := xrayChain{}.Outbounds("node", info(broken))
		if exit != "node/refused" || len(handlers) != 1 || handlers[0].Tag != "node/refused" {
			t.Fatalf("%s: want the refusal, got %d handlers, exit %s", broken, len(handlers), exit)
		}
	}
	if handlers, exit := (xrayChain{}).Outbounds("node", info("")); exit != "" || handlers != nil {
		t.Fatalf("unchained node: want nothing, got %d handlers, exit %s", len(handlers), exit)
	}
}
