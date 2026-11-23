// Package ironproxy renders the configuration of iron-proxy (github.com/ironsh/iron-proxy, Apache-2.0)
// for one sandbox or tool run (decision 0013). burrow-proxy's iron engine (the default,
// cmd/burrow-proxy -engine iron) calls Render and starts iron-proxy with the result.
//
// The rendered configuration keeps the guarantees of the embedded engine (internal/proxy) and adds
// iron-proxy's pipeline:
//   - the only listener the sandbox can reach is the tunnel listener (HTTP CONNECT and SOCKS5);
//     iron-proxy's transparent HTTP and HTTPS listeners, metrics and management stay on loopback
//     or off, and its DNS server is off (the sandbox's network has no DNS: names are resolved by
//     the proxy only);
//   - default deny: the allowlist transform with the sandbox's ALLOW hosts and no CIDRs, so IP
//     literals are refused;
//   - TLS in sni-only mode: no TLS termination and no CA in the sandbox, the SNI must be allowed
//     too and the upstream port is always 443 (iron-proxy ignores the CONNECT port);
//   - upstream_deny_cidrs: every private, loopback, link-local, CGNAT, multicast, reserved and
//     documentation range, checked at connect time against the resolved address (DNS rebinding).
//
// Termination of TLS for hosts with a secret (docs/SECRETS.md) is the next step: it needs a
// per-sandbox CA and the bindings loaded over stdin (decision 0012, docs/SECRETS.md); see decision 0013.
package ironproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Options are what burrow passes to the proxy container (burrow-proxy's flags, for every engine).
type Options struct {
	Listen       string   // tunnel listener inside the proxy container, e.g. 0.0.0.0:3128
	Allow        []string // exact host names or *.domain, as validated by internal/config
	AllowPrivate bool     // TESTS ONLY: let allowed names resolve to private addresses
}

// DenyCIDRs are the addresses the proxy never dials: everything that is not public unicast. The
// same rule as the embedded engine's Policy.AddrAllowed, written as CIDRs for iron-proxy, which checks
// them after resolution, on every dial (iron-proxy unmaps IPv4-mapped IPv6 addresses first).
var DenyCIDRs = []string{
	"0.0.0.0/8",       // "this network"
	"10.0.0.0/8",      // private
	"100.64.0.0/10",   // CGNAT (shared address space)
	"127.0.0.0/8",     // loopback
	"169.254.0.0/16",  // link-local, cloud metadata
	"172.16.0.0/12",   // private
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"192.168.0.0/16",  // private
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"224.0.0.0/4",     // multicast
	"240.0.0.0/4",     // reserved, broadcast
	"::/128",          // unspecified
	"::1/128",         // loopback
	"64:ff9b:1::/48",  // local-use NAT64
	"100::/64",        // discard
	"2001:db8::/32",   // documentation
	"fc00::/7",        // unique local (private), includes fd00:ec2::254 (AWS metadata)
	"fe80::/10",       // link-local
	"ff00::/8",        // multicast
}

// privateDenyCIDRs are used with AllowPrivate (tests only): still never loopback, the unspecified
// address or cloud metadata.
var privateDenyCIDRs = []string{"0.0.0.0/8", "127.0.0.0/8", "169.254.169.254/32", "::/128", "::1/128", "fd00:ec2::254/128"}

// Loopback-only addresses for the listeners iron-proxy always opens. The sandbox cannot reach
// them: they are inside the proxy container's own network namespace.
const (
	loopbackHTTP    = "127.0.0.1:18080"
	loopbackHTTPS   = "127.0.0.1:18443"
	loopbackMetrics = "127.0.0.1:19090"
)

// hostRe is internal/config's ALLOW rule: lowercase names, *.domain, no IPs, no ports. Checked
// again here because the proxy image can be started by hand.
var hostRe = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

// Render returns iron-proxy's configuration. It is JSON, which iron-proxy's YAML parser reads, so
// burrow needs no YAML dependency.
func Render(o Options) ([]byte, error) {
	host, port, err := net.SplitHostPort(o.Listen)
	if err != nil || port == "" || port == "0" {
		return nil, fmt.Errorf("listen %q: want HOST:PORT", o.Listen)
	}
	if host != "" && net.ParseIP(host) == nil {
		return nil, fmt.Errorf("listen %q: want an IP address", o.Listen)
	}
	domains := []string{}
	for _, h := range o.Allow {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if !hostRe.MatchString(h) {
			return nil, fmt.Errorf("allow %q: a lowercase host name or *.domain (no IPs, no ports)", h)
		}
		domains = append(domains, h)
	}
	deny := DenyCIDRs
	if o.AllowPrivate {
		deny = privateDenyCIDRs
	}
	cfg := map[string]any{
		// The sandbox's network has no DNS server (--disable-dns); clients name the host in the
		// CONNECT and the proxy resolves it. iron-proxy's own DNS server is not needed.
		"dns": map[string]any{"enabled": false},
		"proxy": map[string]any{
			"http_listen":         loopbackHTTP,
			"https_listen":        loopbackHTTPS,
			"tunnel_listen":       o.Listen,
			"upstream_deny_cidrs": deny,
		},
		"tls":     map[string]any{"mode": "sni-only"},
		"metrics": map[string]any{"listen": loopbackMetrics},
		"log":     map[string]any{"level": "info"},
		"transforms": []any{
			map[string]any{"name": "allowlist", "config": map[string]any{
				"domains": domains,
				"cidrs":   []string{}, // no IP literals
			}},
		},
	}
	return json.MarshalIndent(cfg, "", "  ")
}

// CheckPorts accepts only 443: in sni-only mode iron-proxy always dials the SNI host on 443,
// whatever port the CONNECT names, so any other list would be a promise the proxy does not keep.
func CheckPorts(ports string) error {
	for _, p := range strings.Split(ports, ",") {
		if strings.TrimSpace(p) != "443" {
			return errors.New("ports: iron-proxy (sni-only) dials port 443 only")
		}
	}
	return nil
}
