package xray

import (
	"slices"

	"github.com/InazumaV/V2bX/api/panel"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/core"
	coreConf "github.com/xtls/xray-core/infra/conf"
)

// NodeOutbound lets a compiled-in feature (module/) give a node its own way out. Outbounds returns
// the handlers to add, in order, and the tag of the one every connection of the node goes to; an
// empty tag leaves the node on the routing rules.
type NodeOutbound interface {
	Outbounds(tag string, info *panel.NodeInfo) (handlers []*core.OutboundHandlerConfig, exit string)
}

var nodeOutbounds []NodeOutbound

// RegisterNodeOutbound adds a feature's outbounds; called from the feature's init().
func RegisterNodeOutbound(o NodeOutbound) {
	nodeOutbounds = append(nodeOutbounds, o)
}

// Refusal is an outbound that drops everything, for a node whose own way out can't be had.
func Refusal(tag string) *core.OutboundHandlerConfig {
	h, err := (&coreConf.OutboundDetourConfig{Protocol: "blackhole", Tag: tag + "/refused"}).Build()
	if err != nil {
		panic(err) // a bare blackhole always builds
	}
	return h
}

// claim adds the outbounds a feature gives the node and sends the node to its exit, before the
// node's inbound exists. A node a feature claimed never falls back to the routing rules: if its
// outbounds can't be added, it gets one that refuses everything.
func (c *Xray) claim(tag string, info *panel.NodeInfo) error {
	for _, o := range nodeOutbounds {
		handlers, exit := o.Outbounds(tag, info)
		if exit == "" {
			continue
		}
		var added []string
		for _, h := range handlers {
			if err := c.addOutbound(h); err != nil {
				log.WithField("tag", tag).WithField("err", err).Error("Add node outbound failed, the node refuses every connection")
				c.dropOutbounds(added)
				refusal := Refusal(tag)
				if err := c.addOutbound(refusal); err != nil {
					return err
				}
				added, exit = []string{refusal.Tag}, refusal.Tag
				break
			}
			added = append(added, h.Tag)
		}
		c.claimed.Store(tag, added)
		c.dispatcher.SetInboundRoute(tag, exit)
		return nil
	}
	return nil
}

// release gives the node's inbound back to the routing rules and removes its outbounds, last first
func (c *Xray) release(tag string) {
	c.dispatcher.ClearInboundRoute(tag)
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
