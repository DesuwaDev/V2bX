package core

import (
	"errors"

	"github.com/InazumaV/V2bX/conf"
)

var (
	cores = map[string]func(c *conf.CoreConfig) (Core, error){}
)

func NewCore(c []conf.CoreConfig) (Core, error) {
	if len(c) == 0 {
		return nil, errors.New("no have vail core")
	}
	// multi core
	if len(c) > 1 {
		return NewSelector(c)
	}
	// one core
	if f, ok := cores[c[0].Type]; ok {
		return f(&c[0])
	} else {
		return nil, errors.New("unknown core type")
	}
}

// TypeFor is the type of the core that will serve a node ("sing", "xray", "hysteria2"), or "" when
// none can: the same choice AddNode makes
func TypeFor(c Core, nodeType string, option *conf.Options) string {
	if s, ok := c.(*Selector); ok {
		if picked := s.pick(nodeType, option); picked != nil {
			return picked.Type()
		}
		return ""
	}
	return c.Type()
}

func RegisterCore(t string, f func(c *conf.CoreConfig) (Core, error)) {
	cores[t] = f
}

func RegisteredCore() []string {
	cs := make([]string, 0, len(cores))
	for k := range cores {
		cs = append(cs, k)
	}
	return cs
}
