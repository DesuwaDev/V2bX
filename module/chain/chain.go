//go:build chain

// Package chain sends a node's connections on through other nodes before they leave, so they
// reach the internet from the last one (a relay in front of an exit, with any number of relays in
// between).
//
// The panel names the hops in the node's config, each as an outbound in the core's own format,
// made from that node's settings and the chain's own account on it: the node dials the first hop,
// the second through the first, and so on. Only the node users connect to needs this feature; the
// nodes it goes through take the chain's account like any user, whatever program serves them.
//
// It fails closed: a node the panel chains never lets a connection out directly, whatever goes
// wrong (an unreadable chain, a hop the core can't dial), it refuses them instead.
package chain

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/InazumaV/V2bX/api/panel"
)

// Feature is this feature's name, as the panel hears it (module.Register)
const Feature = "chain"

// spec is the node's "chain" field from the panel
type spec struct {
	Hops []hop `json:"hops"`
}

// hop is one node to go through: its outbound in either core's format
type hop struct {
	// what the panel calls it, for the logs
	Name string          `json:"name"`
	Sing json.RawMessage `json:"sing"`
	Xray json.RawMessage `json:"xray"`
}

// of is the node's chain: nil when the panel doesn't chain the node; an error when it does but the
// chain can't be used, so the node refuses everything
func of(info *panel.NodeInfo) (*spec, error) {
	raw, ok := info.Extra[Feature]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	s := &spec{}
	if err := json.Unmarshal(raw, s); err != nil {
		return s, fmt.Errorf("unreadable chain: %w", err)
	}
	if len(s.Hops) == 0 {
		return s, errors.New("chain without hops")
	}
	return s, nil
}

// hopTag is the outbound tag of the node's n-th hop (from 1)
func hopTag(tag string, n int) string {
	return fmt.Sprintf("%s/chain-%d", tag, n)
}

// names lists the hops for the logs
func (s *spec) names() []string {
	list := make([]string, len(s.Hops))
	for i, h := range s.Hops {
		list[i] = h.Name
	}
	return list
}
