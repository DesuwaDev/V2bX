//go:build chain

// Package chain sends a user's connections on through other nodes before they leave, so they
// reach the internet from the last one: a chain is the node users connect to (its entry), then the
// nodes it goes through, the last being its exit.
//
// A user has a key of their own to each chain they may use: an account of the entry node that the
// panel's user list marks with its chain (UserInfo.Chain). The entry node's config lists its chains
// ("chains"), each hop an outbound in the core's own format, made from that node's settings and
// the chain's own account on it. A connection with a chain's key dials the chain's first hop, the
// second through the first, and so on; a user's own key goes where it always did. The nodes a chain
// goes through take its account like any user, whatever program serves them.
//
// It fails closed: a chain's key never gets out directly. Whatever goes wrong (unreadable chains, a
// hop the core can't dial, a key whose chain the node doesn't have), its connections are refused.
package chain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
)

// Feature is this feature's name, as the panel hears it (module.Register)
const Feature = "chains"

// spec is one chain of the node's "chains" field from the panel
type spec struct {
	ID int `json:"id"`
	// what the panel calls it, for the logs
	Name string `json:"name"`
	Hops []hop  `json:"hops"`
}

// hop is one node to go through: its outbound in either core's format
type hop struct {
	// what the panel calls it, for the logs
	Name string          `json:"name"`
	Sing json.RawMessage `json:"sing"`
	Xray json.RawMessage `json:"xray"`
}

// keys holds every chain key the panel has sent (uuid -> chain id), for good: a key is a key to its
// chain alone, so one still served after its chain is gone is refused rather than let out
var keys sync.Map

func init() {
	panel.OnUsers(func(_ int, users []panel.UserInfo) { remember(users) })
}

// remember takes note of the chains' keys in a user list, before the node serves it
func remember(users []panel.UserInfo) {
	for _, u := range users {
		if u.Chain != 0 {
			keys.Store(u.Uuid, u.Chain)
		}
	}
}

// chainOf is the chain a key is to (by the user's uuid on the node), if it is a chain's key
func chainOf(uuid string) (int, bool) {
	id, ok := keys.Load(uuid)
	if !ok {
		return 0, false
	}
	return id.(int), true
}

// of lists the node's chains: none when the panel sends none; an error when they can't be read, so
// every chain's key is refused
func of(info *panel.NodeInfo) ([]spec, error) {
	raw, ok := info.Extra["chains"]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var chains []spec
	if err := json.Unmarshal(raw, &chains); err != nil {
		return nil, fmt.Errorf("unreadable chains: %w", err)
	}
	return chains, nil
}

// usable is why the chain can't be used as the panel sent it, if it can't
func (s *spec) usable() error {
	if s.ID == 0 {
		return errors.New("a chain without an id")
	}
	if len(s.Hops) == 0 {
		return fmt.Errorf("chain %d (%s) has no hops", s.ID, s.Name)
	}
	return nil
}

// present is whether a hop has an outbound in that format (the panel leaves out, or sends null for,
// one the hop's protocol has none in)
func present(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// hopTag is the outbound tag of a chain's n-th hop (from 1) on the node
func hopTag(tag string, chain, n int) string {
	return fmt.Sprintf("%s/chain%d-%d", tag, chain, n)
}

// names lists the hops for the logs
func (s *spec) names() []string {
	list := make([]string, len(s.Hops))
	for i, h := range s.Hops {
		list[i] = h.Name
	}
	return list
}
