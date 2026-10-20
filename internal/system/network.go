package system

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// Network is the IPv4 setup of the wired connection.
type Network struct {
	Connection string   `json:"connection"`
	Device     string   `json:"device"`
	Method     string   `json:"method"`  // auto (DHCP) or manual
	Address    string   `json:"address"` // with prefix, e.g. 192.168.1.10/24
	Gateway    string   `json:"gateway"`
	DNS        []string `json:"dns"`
}

// nmcli runs nmcli in terse mode and returns its output fields.
func nmcli(args ...string) (map[string]string, error) {
	out, err := exec.Command("nmcli", append([]string{"-t", "-e", "no"}, args...)...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("nmcli: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		// Multi-value fields come as KEY[1], KEY[2]...
		if i := strings.Index(k, "["); i > 0 {
			k = k[:i]
			if m[k] != "" {
				v = m[k] + "," + v
			}
		}
		m[k] = v
	}
	return m, nil
}

// primaryConnection finds the active NetworkManager connection of the
// first ethernet (or wifi) device.
func primaryConnection() (conn, device string, err error) {
	out, err := exec.Command("nmcli", "-t", "-f", "DEVICE,TYPE,STATE,CONNECTION", "device").Output()
	if err != nil {
		return "", "", fmt.Errorf("nmcli: %w", err)
	}
	for _, kind := range []string{"ethernet", "wifi"} {
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.SplitN(line, ":", 4)
			if len(f) == 4 && f[1] == kind && f[3] != "" {
				return f[3], f[0], nil
			}
		}
	}
	return "", "", errors.New("no network connection")
}

// ReadNetwork returns the setup of the primary connection; for DHCP the
// current address.
func ReadNetwork() (*Network, error) {
	conn, dev, err := primaryConnection()
	if err != nil {
		return nil, err
	}
	m, err := nmcli("-f", "ipv4.method,ipv4.addresses,ipv4.gateway,ipv4.dns", "connection", "show", conn)
	if err != nil {
		return nil, err
	}
	n := &Network{Connection: conn, Device: dev, Method: m["ipv4.method"],
		Address: m["ipv4.addresses"], Gateway: m["ipv4.gateway"]}
	if m["ipv4.dns"] != "" {
		n.DNS = strings.Split(m["ipv4.dns"], ",")
	}
	if n.Method == "auto" {
		cur, err := nmcli("-f", "IP4.ADDRESS,IP4.GATEWAY,IP4.DNS", "device", "show", dev)
		if err == nil {
			n.Address, _, _ = strings.Cut(cur["IP4.ADDRESS"], ",")
			n.Gateway = cur["IP4.GATEWAY"]
			if cur["IP4.DNS"] != "" {
				n.DNS = strings.Split(cur["IP4.DNS"], ",")
			}
		}
	}
	if n.Gateway == "--" {
		n.Gateway = ""
	}
	return n, nil
}

// Validate checks a static setup.
func (n *Network) Validate() error {
	switch n.Method {
	case "auto":
		return nil
	case "manual":
	default:
		return fmt.Errorf("unknown method %q", n.Method)
	}
	ip, ipnet, err := net.ParseCIDR(n.Address)
	if err != nil || ip.To4() == nil {
		return fmt.Errorf("invalid address %q, expected e.g. 192.168.1.10/24", n.Address)
	}
	if n.Gateway != "" {
		gw := net.ParseIP(n.Gateway)
		if gw == nil || gw.To4() == nil || !ipnet.Contains(gw) {
			return fmt.Errorf("gateway %q is not in the network %s", n.Gateway, ipnet)
		}
	}
	for _, d := range n.DNS {
		if net.ParseIP(d) == nil {
			return fmt.Errorf("invalid DNS server %q", d)
		}
	}
	return nil
}

// ApplyNetwork changes the primary connection and activates it. The
// address of the device may change, so the caller answers first.
func ApplyNetwork(n *Network) error {
	if err := n.Validate(); err != nil {
		return err
	}
	conn, _, err := primaryConnection()
	if err != nil {
		return err
	}
	args := []string{"connection", "modify", conn}
	if n.Method == "auto" {
		args = append(args, "ipv4.method", "auto", "ipv4.addresses", "", "ipv4.gateway", "", "ipv4.dns", "")
	} else {
		args = append(args, "ipv4.method", "manual", "ipv4.addresses", n.Address,
			"ipv4.gateway", n.Gateway, "ipv4.dns", strings.Join(n.DNS, ","))
	}
	if err := run("nmcli", args...); err != nil {
		return err
	}
	return run("nmcli", "connection", "up", conn)
}
