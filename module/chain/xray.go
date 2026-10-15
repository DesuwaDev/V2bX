//go:build chain && xray

package chain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/InazumaV/V2bX/api/panel"
	vxray "github.com/InazumaV/V2bX/core/xray"
	"github.com/InazumaV/V2bX/module"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/core"
	coreConf "github.com/xtls/xray-core/infra/conf"
)

func init() {
	module.Register(Feature, "xray")
	vxray.RegisterNodeOutbound(xrayChain{})
}

type xrayChain struct{}

// Outbounds gives a chained node the hops' outbounds, each dialing through the one before, and
// sends the node to the last one
func (xrayChain) Outbounds(tag string, info *panel.NodeInfo) ([]*core.OutboundHandlerConfig, string) {
	s, err := of(info)
	if s == nil && err == nil {
		return nil, ""
	}
	if err == nil {
		var handlers []*core.OutboundHandlerConfig
		var exit string
		handlers, exit, err = buildXray(tag, s)
		if err == nil {
			log.WithField("tag", tag).Infof("Chained through %s", strings.Join(s.names(), " -> "))
			return handlers, exit
		}
	}
	log.WithField("tag", tag).WithField("err", err).Error("Chain unusable, the node refuses every connection")
	refusal := vxray.Refusal(tag)
	return []*core.OutboundHandlerConfig{refusal}, refusal.Tag
}

func buildXray(tag string, s *spec) ([]*core.OutboundHandlerConfig, string, error) {
	handlers := make([]*core.OutboundHandlerConfig, 0, len(s.Hops))
	previous := ""
	for i, h := range s.Hops {
		if len(h.Xray) == 0 {
			return nil, "", fmt.Errorf("hop %d (%s) has no xray outbound", i+1, h.Name)
		}
		c := &coreConf.OutboundDetourConfig{}
		if err := json.Unmarshal(h.Xray, c); err != nil {
			return nil, "", fmt.Errorf("hop %d (%s): %w", i+1, h.Name, err)
		}
		c.Tag = hopTag(tag, i+1)
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
