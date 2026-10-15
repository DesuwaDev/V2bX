package sing

import (
	"context"

	"github.com/InazumaV/V2bX/api/panel"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
)

// NodeRouter lets a compiled-in feature (module/) take over where a node's connections go. Route is
// asked before the node's inbound is created: a nil router leaves the node on the shared one, and
// release, if any, runs once the inbound is gone.
type NodeRouter interface {
	Route(env Env, tag string, info *panel.NodeInfo) (router adapter.Router, release func())
}

// Env is what a feature gets from the sing core.
type Env struct {
	Context context.Context
	Box     *box.Box
	// V2bX's own work on every connection (hook.go): device and speed limits, the panel's block
	// rules and the users' traffic counters. A router that takes over must still pass every
	// connection through it.
	Tracker adapter.ConnectionTracker
	Logger  log.ContextLogger
}

var nodeRouters []NodeRouter

// RegisterNodeRouter adds a feature's router; called from the feature's init().
func RegisterNodeRouter(r NodeRouter) {
	nodeRouters = append(nodeRouters, r)
}

// routeFor asks the features, in order, whether one takes over the node
func (b *Sing) routeFor(tag string, info *panel.NodeInfo) (adapter.Router, func()) {
	env := Env{
		Context: b.ctx,
		Box:     b.box,
		Tracker: b.hookServer,
		Logger:  b.logFactory.NewLogger("node/" + tag),
	}
	for _, r := range nodeRouters {
		if router, release := r.Route(env, tag, info); router != nil {
			return router, release
		}
	}
	return nil, nil
}
