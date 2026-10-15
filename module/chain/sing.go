//go:build chain && sing

package chain

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/InazumaV/V2bX/api/panel"
	vsing "github.com/InazumaV/V2bX/core/sing"
	"github.com/InazumaV/V2bX/module"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/json"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

func init() {
	module.Register(Feature, "sing")
	vsing.RegisterNodeRouter(singChains{})
}

type singChains struct{}

// Route gives every node a router of its own, which sends a chain's key to the chain's last hop
// (the earlier ones as its detours) past the routing rules, and a user's own key to the shared
// router as before. Every node, chains or not: a key the node has no chain for is refused there.
func (singChains) Route(env vsing.Env, tag string, info *panel.NodeInfo) (adapter.Router, func()) {
	r := &router{
		Router:      env.Box.Router(),
		tag:         tag,
		exits:       map[int]adapter.Outbound{},
		connections: service.FromContext[adapter.ConnectionManager](env.Context),
		tracker:     env.Tracker,
		logger:      env.Logger,
	}
	chains, err := of(info)
	if err != nil {
		env.Logger.Error("chains of node ", tag, " unusable, their keys are refused: ", err)
	}
	var created []string
	for _, s := range chains {
		made, exit, err := buildSing(env, tag, &s)
		if err != nil {
			removeSing(env, made)
			env.Logger.Error("chain ", s.ID, " (", s.Name, ") unusable, its keys are refused: ", err)
			continue
		}
		created = append(created, made...)
		r.exits[s.ID] = exit
		env.Logger.Info("chain ", s.ID, " (", s.Name, ") through ", strings.Join(s.names(), " -> "))
	}
	return r, func() { removeSing(env, created) }
}

// buildSing creates a chain's hops' outbounds, each dialing through the one before
func buildSing(env vsing.Env, tag string, s *spec) (created []string, exit adapter.Outbound, err error) {
	if err := s.usable(); err != nil {
		return nil, nil, err
	}
	manager := env.Box.Outbound()
	previous := ""
	for i, h := range s.Hops {
		if !present(h.Sing) {
			return created, nil, fmt.Errorf("hop %d (%s) has no sing-box outbound", i+1, h.Name)
		}
		var fields map[string]any
		if err := stdjson.Unmarshal(h.Sing, &fields); err != nil {
			return created, nil, fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		current := hopTag(tag, s.ID, i+1)
		fields["tag"] = current
		delete(fields, "detour")
		if previous != "" {
			fields["detour"] = previous
		}
		data, err := stdjson.Marshal(fields)
		if err != nil {
			return created, nil, fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		outbound, err := json.UnmarshalExtendedContext[option.Outbound](env.Context, data)
		if err != nil {
			return created, nil, fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		if err := manager.Create(env.Context, env.Box.Router(), env.Logger, current, outbound.Type, outbound.Options); err != nil {
			return created, nil, fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		created = append(created, current)
		previous = current
	}
	exit, ok := manager.Outbound(previous)
	if !ok {
		return created, nil, errors.New("the last hop's outbound is gone")
	}
	return created, exit, nil
}

// removeSing removes hops' outbounds, the last first (each depends on the one before)
func removeSing(env vsing.Env, created []string) {
	for _, t := range slices.Backward(created) {
		if err := env.Box.Outbound().Remove(t); err != nil {
			env.Logger.Warn("remove outbound ", t, ": ", err)
		}
	}
}

// router is a node's: a chain's key leaves from the chain's last hop or not at all, no routing rule
// applies; like the shared router it first hands each connection to V2bX's own hook (limits, the
// panel's rules, traffic counters). Anything else is the shared router's.
type router struct {
	adapter.Router
	tag string
	// by chain id, the last hop of each chain the node can use
	exits       map[int]adapter.Outbound
	connections adapter.ConnectionManager
	tracker     adapter.ConnectionTracker
	logger      log.ContextLogger
}

// exit is where a connection with the user's key goes: for a chain's key, the chain's last hop (an
// error when the node has no usable chain for it); nil for anyone else's, left to the shared router
func (r *router) exit(user string) (adapter.Outbound, error) {
	id, ok := chainOf(user)
	if !ok {
		return nil, nil
	}
	if exit := r.exits[id]; exit != nil {
		return exit, nil
	}
	return nil, fmt.Errorf("node %s has no usable chain %d, connection refused", r.tag, id)
}

func (r *router) RouteConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	exit, err := r.exit(metadata.User)
	if err != nil {
		r.logger.DebugContext(ctx, err)
		N.CloseOnHandshakeFailure(conn, onClose, err)
		return
	}
	if exit == nil {
		r.Router.RouteConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	metadata.Network = N.NetworkTCP
	conn = r.tracker.RoutedConnection(ctx, conn, metadata, nil, exit)
	if handler, ok := exit.(adapter.ConnectionHandlerEx); ok {
		handler.NewConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	r.connections.NewConnection(ctx, exit, conn, metadata, onClose)
}

func (r *router) RoutePacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	exit, err := r.exit(metadata.User)
	if err == nil && exit != nil && !common.Contains(exit.Network(), N.NetworkUDP) {
		err = fmt.Errorf("the chain of this key on node %s carries no UDP", r.tag)
	}
	if err != nil {
		r.logger.DebugContext(ctx, err)
		N.CloseOnHandshakeFailure(conn, onClose, err)
		return
	}
	if exit == nil {
		r.Router.RoutePacketConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	metadata.Network = N.NetworkUDP
	conn = r.tracker.RoutedPacketConnection(ctx, conn, metadata, nil, exit)
	if handler, ok := exit.(adapter.PacketConnectionHandlerEx); ok {
		handler.NewPacketConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	r.connections.NewPacketConnection(ctx, exit, conn, metadata, onClose)
}

// RouteConnection and RoutePacketConnection are the older calls some inbounds still make: they
// return once the connection is done
func (r *router) RouteConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	if _, chained := chainOf(metadata.User); !chained {
		return r.Router.RouteConnection(ctx, conn, metadata)
	}
	done := make(chan struct{})
	r.RouteConnectionEx(ctx, conn, metadata, N.OnceClose(func(error) { close(done) }))
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}

func (r *router) RoutePacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	if _, chained := chainOf(metadata.User); !chained {
		return r.Router.RoutePacketConnection(ctx, conn, metadata)
	}
	done := make(chan struct{})
	r.RoutePacketConnectionEx(ctx, conn, metadata, N.OnceClose(func(error) { close(done) }))
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}
