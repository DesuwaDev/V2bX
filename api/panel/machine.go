package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// machine names the server this program runs on, the same for every run of it and every program on
// that server, so the panel tells which nodes share one: a hash of the system's machine-id, or,
// where there is none (a container), of its network cards' hardware addresses; "" when neither can
// be had. The hash keeps the machine-id itself from the panel.
var machine = machineID([]string{"/etc/machine-id", "/var/lib/dbus/machine-id"}, hardwareAddrs)

func machineID(files []string, addrs func() []string) string {
	for _, f := range files {
		if raw, err := os.ReadFile(f); err == nil {
			if id := bytes.TrimSpace(raw); len(id) > 0 {
				return hashID(id)
			}
		}
	}
	if macs := addrs(); len(macs) > 0 {
		return hashID([]byte(strings.Join(macs, ",")))
	}
	return ""
}

// hardwareAddrs are the addresses of the server's own network cards: those with a device behind
// them (/sys/class/net/<name>/device), not the bridges and veth pairs containers come and go with
func hardwareAddrs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var macs []string
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 || len(i.HardwareAddr) == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join("/sys/class/net", i.Name, "device")); err != nil {
			continue
		}
		macs = append(macs, i.HardwareAddr.String())
	}
	sort.Strings(macs)
	return macs
}

func hashID(raw []byte) string {
	sum := sha256.Sum256(append([]byte("v2bx-machine:"), raw...))
	return hex.EncodeToString(sum[:8])
}
