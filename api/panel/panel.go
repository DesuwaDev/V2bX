package panel

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/InazumaV/V2bX/conf"
	"github.com/go-resty/resty/v2"
)

// Panel is the interface for different panel's api.

// ProgramVersion is this build's version, named in every request (cmd sets it)
var ProgramVersion = "dev"

// instance names this run of the program in every request: one program serves several nodes,
// and the panel tells which run together by it
var instance = func() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}()

var (
	hooksMu  sync.RWMutex
	headers  []func() map[string]string
	answered []func(nodeID int, header http.Header)
	usersGot []func(nodeID int, users []UserInfo)
)

// AddHeaders has every request to the panel carry the headers f gives at that moment (module/).
func AddHeaders(f func() map[string]string) {
	hooksMu.Lock()
	defer hooksMu.Unlock()
	headers = append(headers, f)
}

// OnAnswer hands f the headers of each answer to a node's config request (module/).
func OnAnswer(f func(nodeID int, header http.Header)) {
	hooksMu.Lock()
	defer hooksMu.Unlock()
	answered = append(answered, f)
}

// OnUsers hands f each user list the panel sends a node, before the node serves it (module/).
func OnUsers(f func(nodeID int, users []UserInfo)) {
	hooksMu.Lock()
	defer hooksMu.Unlock()
	usersGot = append(usersGot, f)
}

func gotUsers(nodeID int, users []UserInfo) {
	hooksMu.RLock()
	hooks := append([]func(int, []UserInfo){}, usersGot...)
	hooksMu.RUnlock()
	for _, f := range hooks {
		f(nodeID, users)
	}
}

func answer(nodeID int, header http.Header) {
	hooksMu.RLock()
	hooks := append([]func(int, http.Header){}, answered...)
	hooksMu.RUnlock()
	for _, f := range hooks {
		f(nodeID, header)
	}
}

type Client struct {
	client           *resty.Client
	APIHost          string
	APISendIP        string
	Token            string
	NodeType         string
	NodeId           int
	nodeEtag         string
	userEtag         string
	responseBodyHash string
	UserList         *UserListBody
	AliveMap         *AliveMap
}

func New(c *conf.ApiConfig) (*Client, error) {
	var client *resty.Client
	if c.APISendIP != "" {
		client = resty.NewWithLocalAddr(&net.TCPAddr{
			IP: net.ParseIP(c.APISendIP),
		})
	} else {
		client = resty.New()
	}
	client.SetRetryCount(3)
	if c.Timeout > 0 {
		client.SetTimeout(time.Duration(c.Timeout) * time.Second)
	} else {
		client.SetTimeout(5 * time.Second)
	}
	client.OnError(func(req *resty.Request, err error) {
		var v *resty.ResponseError
		if errors.As(err, &v) {
			// v.Response contains the last response from the server
			// v.Err contains the original error
			logrus.Error(v.Err)
		}
	})
	client.SetBaseURL(c.APIHost)
	client.OnBeforeRequest(func(_ *resty.Client, r *resty.Request) error {
		hooksMu.RLock()
		sources := append([]func() map[string]string{}, headers...)
		hooksMu.RUnlock()
		for _, f := range sources {
			for k, v := range f() {
				r.SetHeader(k, v)
			}
		}
		return nil
	})
	// Check node type
	c.NodeType = strings.ToLower(c.NodeType)
	switch c.NodeType {
	case "v2ray":
		c.NodeType = "vmess"
	case
		"vmess",
		"trojan",
		"shadowsocks",
		"hysteria",
		"hysteria2",
		"tuic",
		"anytls",
		"vless":
	default:
		return nil, fmt.Errorf("unsupported Node type: %s", c.NodeType)
	}
	// set params
	client.SetQueryParams(map[string]string{
		"node_type": c.NodeType,
		"node_id":   strconv.Itoa(c.NodeID),
		"token":     c.Key,
	})
	return &Client{
		client:    client,
		Token:     c.Key,
		APIHost:   c.APIHost,
		APISendIP: c.APISendIP,
		NodeType:  c.NodeType,
		NodeId:    c.NodeID,
		UserList:  &UserListBody{},
		AliveMap:  &AliveMap{},
	}, nil
}

// Announce names, in every request from now on, this build, the core that serves the node and the
// compiled-in features it has on that core (module/), so the panel asks the node only for what it
// can do: a feature the panel needs but the node lacks, it handles on its own side.
func (c *Client) Announce(core string, features []string) {
	c.client.SetHeaders(map[string]string{
		"X-V2bX-Version":  ProgramVersion,
		"X-V2bX-Core":     core,
		"X-V2bX-Features": strings.Join(features, ","),
		"X-V2bX-Instance": instance,
	})
}
