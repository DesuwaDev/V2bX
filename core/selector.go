package core

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/conf"
)

type Selector struct {
	cores map[string]Core
	nodes sync.Map
}

func NewSelector(c []conf.CoreConfig) (Core, error) {
	cs := make(map[string]Core, len(c))
	for _, t := range c {
		f, ok := cores[strings.ToLower(t.Type)]
		if !ok {
			return nil, errors.New("unknown core type: " + t.Type)
		}
		core1, err := f(&t)
		if err != nil {
			return nil, err
		}
		if t.Name == "" {
			cs[t.Type] = core1
		} else {
			cs[t.Name] = core1
		}
	}
	return &Selector{
		cores: cs,
	}, nil
}

func (s *Selector) Start() error {
	for i := range s.cores {
		err := s.cores[i].Start()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Selector) Close() error {
	var errs []error
	for i := range s.cores {
		if err := s.cores[i].Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func isSupported(protocol string, protocols []string) bool {
	for i := range protocols {
		if protocol == protocols[i] {
			return true
		}
	}
	return false
}

// pick is the core that serves a node of the given type: by name, by type, or the last one (in name
// order, so it is always the same one) that supports the node's protocol
func (s *Selector) pick(nodeType string, option *conf.Options) Core {
	if len(option.CoreName) > 0 {
		return s.cores[option.CoreName]
	}
	var core Core
	for _, name := range slices.Sorted(maps.Keys(s.cores)) {
		c := s.cores[name]
		if len(option.Core) == 0 {
			if !isSupported(nodeType, c.Protocols()) {
				continue
			}
		} else if option.Core != c.Type() {
			continue
		}
		core = c
	}
	return core
}

func (s *Selector) AddNode(tag string, info *panel.NodeInfo, option *conf.Options) error {
	core := s.pick(info.Type, option)
	if core == nil {
		return errors.New("the node type is not support")
	}
	if len(option.Core) == 0 {
		option.Core = core.Type()
		err := option.UnmarshalJSON(option.RawOptions)
		if err != nil {
			return fmt.Errorf("unmarshal option error: %s", err)
		}
		option.RawOptions = nil
	}
	err := core.AddNode(tag, info, option)
	if err != nil {
		return err
	}
	s.nodes.Store(tag, core)
	return nil
}

func (s *Selector) DelNode(tag string) error {
	if t, e := s.nodes.Load(tag); e {
		err := t.(Core).DelNode(tag)
		if err != nil {
			return err
		}
		s.nodes.Delete(tag)
		return nil
	}
	return errors.New("the node is not have")
}

func (s *Selector) AddUsers(p *AddUsersParams) (added int, err error) {
	t, e := s.nodes.Load(p.Tag)
	if !e {
		return 0, errors.New("the node is not have")
	}
	return t.(Core).AddUsers(p)
}

func (s *Selector) GetUserTrafficSlice(tag string, reset bool) ([]panel.UserTraffic, error) {
	t, e := s.nodes.Load(tag)
	if !e {
		return nil, errors.New("the node is not have")
	}
	return t.(Core).GetUserTrafficSlice(tag, reset)
}

func (s *Selector) DelUsers(users []panel.UserInfo, tag string, info *panel.NodeInfo) error {
	t, e := s.nodes.Load(tag)
	if !e {
		return errors.New("the node is not have")
	}
	return t.(Core).DelUsers(users, tag, info)
}

func (s *Selector) Protocols() []string {
	protocols := make([]string, 0)
	for i := range s.cores {
		protocols = append(protocols, s.cores[i].Protocols()...)
	}
	return protocols
}

func (s *Selector) Type() string {
	t := "Selector("
	var flag bool
	for n, c := range s.cores {
		if flag {
			t += " "
		} else {
			flag = true
		}
		if len(n) == 0 {
			t += c.Type()
		} else {
			t += n
		}
	}
	t += ")"
	return t
}
