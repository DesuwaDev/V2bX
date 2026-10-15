package xray

import (
	"slices"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/core/xray/app/dispatcher"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/core"
)

// NodeRoute lets a compiled-in feature (module/) send some of a node's users their own way. Route
// returns the outbounds to add, in order, and pick: for a user (as the core names one, "tag|uuid")
// the outbound its connections go to, and whether the feature takes the user at all. A user it
// doesn't take stays on the routing rules; one it takes with no outbound is refused. A nil pick
// leaves the whole node on the routing rules.
type NodeRoute interface {
	Route(tag string, info *panel.NodeInfo) (handlers []*core.OutboundHandlerConfig, pick dispatcher.UserRoute)
}

var nodeRoutes []NodeRoute

// RegisterNodeRoute adds a feature's routes; called from the feature's init().
func RegisterNodeRoute(r NodeRoute) {
	nodeRoutes = append(nodeRoutes, r)
}

// claim adds the outbounds a feature gives the node and has it pick the way out of the node's
// users, before the node's inbound exists. If its outbounds can't be added, every user the feature
// takes is refused: none of them falls back to the routing rules.
func (c *Xray) claim(tag string, info *panel.NodeInfo) error {
	for _, r := range nodeRoutes {
		handlers, pick := r.Route(tag, info)
		if pick == nil {
			continue
		}
		var added []string
		for _, h := range handlers {
			if err := c.addOutbound(h); err != nil {
				log.WithField("tag", tag).WithField("err", err).Error("Add node outbound failed, the users it was for are refused")
				c.dropOutbounds(added)
				added = nil
				takes := pick
				pick = func(user string) (string, bool) {
					_, taken := takes(user)
					return "", taken
				}
				break
			}
			added = append(added, h.Tag)
		}
		c.claimed.Store(tag, added)
		c.dispatcher.SetUserRoute(tag, pick)
		return nil
	}
	return nil
}

// release gives the node's inbound back to the routing rules and removes its outbounds, last first
func (c *Xray) release(tag string) {
	c.dispatcher.ClearUserRoute(tag)
	if added, ok := c.claimed.LoadAndDelete(tag); ok {
		c.dropOutbounds(added.([]string))
	}
}

func (c *Xray) dropOutbounds(tags []string) {
	for _, t := range slices.Backward(tags) {
		if err := c.removeOutbound(t); err != nil {
			log.WithField("tag", t).WithField("err", err).Warn("Remove node outbound failed")
		}
	}
}
