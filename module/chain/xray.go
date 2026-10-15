//go:build chain && xray

package chain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/InazumaV/V2bX/api/panel"
	vxray "github.com/InazumaV/V2bX/core/xray"
	"github.com/InazumaV/V2bX/core/xray/app/dispatcher"
	"github.com/InazumaV/V2bX/module"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/core"
	coreConf "github.com/xtls/xray-core/infra/conf"
)

func init() {
	module.Register(Feature, "xray")
	vxray.RegisterNodeRoute(xrayChains{})
}

type xrayChains struct{}

// Route gives every node its chains' outbounds, each hop dialing through the one before, and sends
// a chain's key to the chain's last hop; a user's own key stays on the routing rules. Every node,
// chains or not: a key the node has no chain for is refused there.
func (xrayChains) Route(tag string, info *panel.NodeInfo) ([]*core.OutboundHandlerConfig, dispatcher.UserRoute) {
	exits := map[int]string{}
	var handlers []*core.OutboundHandlerConfig
	chains, err := of(info)
	if err != nil {
		log.WithField("tag", tag).WithField("err", err).Error("Chains unusable, their keys are refused")
	}
	for _, s := range chains {
		built, exit, err := buildXray(tag, &s)
		if err != nil {
			log.WithField("tag", tag).WithField("err", err).Errorf("Chain %d (%s) unusable, its keys are refused", s.ID, s.Name)
			continue
		}
		handlers = append(handlers, built...)
		exits[s.ID] = exit
		log.WithField("tag", tag).Infof("Chain %d (%s) through %s", s.ID, s.Name, strings.Join(s.names(), " -> "))
	}
	return handlers, func(user string) (string, bool) {
		// the core names a user "tag|uuid"
		id, ok := chainOf(user[strings.LastIndexByte(user, '|')+1:])
		if !ok {
			return "", false
		}
		return exits[id], true
	}
}

func buildXray(tag string, s *spec) ([]*core.OutboundHandlerConfig, string, error) {
	if err := s.usable(); err != nil {
		return nil, "", err
	}
	handlers := make([]*core.OutboundHandlerConfig, 0, len(s.Hops))
	previous := ""
	for i, h := range s.Hops {
		if !present(h.Xray) {
			return nil, "", fmt.Errorf("hop %d (%s) has no xray outbound", i+1, h.Name)
		}
		c := &coreConf.OutboundDetourConfig{}
		if err := json.Unmarshal(h.Xray, c); err != nil {
			return nil, "", fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		c.Tag = hopTag(tag, s.ID, i+1)
		c.ProxySettings = nil
		if previous != "" {
			if c.StreamSetting == nil {
				c.StreamSetting = &coreConf.StreamConfig{}
			}
			if c.StreamSetting.SocketSettings == nil {
				c.StreamSetting.SocketSettings = &coreConf.SocketConfig{}
			}
			c.StreamSetting.SocketSettings.DialerProxy = previous
		}
		built, err := c.Build()
		if err != nil {
			return nil, "", fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		handlers = append(handlers, built)
		previous = c.Tag
	}
	return handlers, previous, nil
}
