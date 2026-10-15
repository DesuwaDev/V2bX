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
	vsing.RegisterNodeRouter(singChain{})
}

type singChain struct{}

// Route gives a chained node a router of its own: every connection goes to the chain's last hop
// (the earlier ones as its detours), past the routing rules
func (singChain) Route(env vsing.Env, tag string, info *panel.NodeInfo) (adapter.Router, func()) {
	s, err := of(info)
	if s == nil && err == nil {
		return nil, nil
	}
	r := &router{Router: env.Box.Router(), tag: tag, tracker: env.Tracker, logger: env.Logger}
	if err == nil {
		var created []string
		created, r.exit, err = buildSing(env, tag, s)
		if err == nil {
			r.connections = service.FromContext[adapter.ConnectionManager](env.Context)
			env.Logger.Info("chained through ", strings.Join(s.names(), " -> "))
			return r, func() { removeSing(env, created) }
		}
		removeSing(env, created)
	}
	r.refused = fmt.Errorf("chain of node %s unusable, connection refused: %w", tag, err)
	env.Logger.Error(r.refused)
	return r, nil
}

// buildSing creates the hops' outbounds, each dialing through the one before
func buildSing(env vsing.Env, tag string, s *spec) (created []string, exit adapter.Outbound, err error) {
	manager := env.Box.Outbound()
	previous := ""
	for i, h := range s.Hops {
		if len(h.Sing) == 0 {
			return created, nil, fmt.Errorf("hop %d (%s) has no sing-box outbound", i+1, h.Name)
		}
		var fields map[string]any
		if err := stdjson.Unmarshal(h.Sing, &fields); err != nil {
			return created, nil, fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		current := hopTag(tag, i+1)
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

// removeSing removes the hops' outbounds, the last first (each depends on the one before)
func removeSing(env vsing.Env, created []string) {
	for _, t := range slices.Backward(created) {
		if err := env.Box.Outbound().Remove(t); err != nil {
			env.Logger.Warn("remove outbound ", t, ": ", err)
		}
	}
}

// router sends a chained node's connections to the chain's last hop. Like the shared router, it
// first hands each one to V2bX's own hook (limits, the panel's rules, traffic counters); unlike
// it, no routing rule applies: the connection leaves from the last hop or not at all.
type router struct {
	adapter.Router
	tag         string
	exit        adapter.Outbound
	connections adapter.ConnectionManager
	tracker     adapter.ConnectionTracker
	logger      log.ContextLogger
	// set when the chain can't be used: every connection is refused
	refused error
}

func (r *router) RouteConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	if r.refused != nil {
		r.logger.DebugContext(ctx, r.refused)
		N.CloseOnHandshakeFailure(conn, onClose, r.refused)
		return
	}
	metadata.Network = N.NetworkTCP
	conn = r.tracker.RoutedConnection(ctx, conn, metadata, nil, r.exit)
	if handler, ok := r.exit.(adapter.ConnectionHandlerEx); ok {
		handler.NewConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	r.connections.NewConnection(ctx, r.exit, conn, metadata, onClose)
}

func (r *router) RoutePacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	refused := r.refused
	if refused == nil && !common.Contains(r.exit.Network(), N.NetworkUDP) {
		refused = fmt.Errorf("the chain of node %s carries no UDP", r.tag)
	}
	if refused != nil {
		r.logger.DebugContext(ctx, refused)
		N.CloseOnHandshakeFailure(conn, onClose, refused)
		return
	}
	metadata.Network = N.NetworkUDP
	conn = r.tracker.RoutedPacketConnection(ctx, conn, metadata, nil, r.exit)
	if handler, ok := r.exit.(adapter.PacketConnectionHandlerEx); ok {
		handler.NewPacketConnectionEx(ctx, conn, metadata, onClose)
		return
	}
	r.connections.NewPacketConnection(ctx, r.exit, conn, metadata, onClose)
}

// RouteConnection and RoutePacketConnection are the older calls some inbounds still make: they
// return once the connection is done
func (r *router) RouteConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	done := make(chan struct{})
	r.RouteConnectionEx(ctx, conn, metadata, N.OnceClose(func(error) { close(done) }))
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}

func (r *router) RoutePacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	done := make(chan struct{})
	r.RoutePacketConnectionEx(ctx, conn, metadata, N.OnceClose(func(error) { close(done) }))
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}
