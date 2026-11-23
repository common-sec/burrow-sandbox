package ironproxy

import (
	"encoding/json"
	"net/netip"
	"slices"
	"testing"

	"github.com/common-sec/burrow-sandbox/internal/proxy"
)

type rendered struct {
	DNS struct {
		Enabled *bool `json:"enabled"`
	} `json:"dns"`
	Proxy struct {
		HTTPListen  string   `json:"http_listen"`
		HTTPSListen string   `json:"https_listen"`
		Tunnel      string   `json:"tunnel_listen"`
		Deny        []string `json:"upstream_deny_cidrs"`
	} `json:"proxy"`
	TLS struct {
		Mode string `json:"mode"`
	} `json:"tls"`
	Metrics struct {
		Listen string `json:"listen"`
	} `json:"metrics"`
	Management *struct{} `json:"management"`
	Transforms []struct {
		Name   string `json:"name"`
		Config struct {
			Domains []string `json:"domains"`
			CIDRs   []string `json:"cidrs"`
			Rules   []any    `json:"rules"`
		} `json:"config"`
	} `json:"transforms"`
}

func render(t *testing.T, o Options) rendered {
	t.Helper()
	b, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	var r rendered
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// U23: the only listener the sandbox can reach is the tunnel; everything else is loopback or off.
func TestRenderListenersAndModes(t *testing.T) {
	r := render(t, Options{Listen: "0.0.0.0:3128", Allow: []string{"pypi.org", "*.github.com"}})
	if r.DNS.Enabled == nil || *r.DNS.Enabled {
		t.Error("iron-proxy's DNS server must be off")
	}
	if r.Proxy.Tunnel != "0.0.0.0:3128" {
		t.Errorf("tunnel listener %q", r.Proxy.Tunnel)
	}
	for _, l := range []string{r.Proxy.HTTPListen, r.Proxy.HTTPSListen, r.Metrics.Listen} {
		if ap, err := netip.ParseAddrPort(l); err != nil || !ap.Addr().IsLoopback() {
			t.Errorf("listener %q must be on loopback", l)
		}
	}
	if r.Management != nil {
		t.Error("the management API must stay off")
	}
	if r.TLS.Mode != "sni-only" {
		t.Errorf("tls mode %q, want sni-only (no TLS termination yet)", r.TLS.Mode)
	}
	if len(r.Transforms) != 1 || r.Transforms[0].Name != "allowlist" {
		t.Fatalf("transforms: %+v", r.Transforms)
	}
	al := r.Transforms[0].Config
	if !slices.Equal(al.Domains, []string{"pypi.org", "*.github.com"}) || al.CIDRs == nil || len(al.CIDRs) != 0 || al.Rules != nil {
		t.Errorf("allowlist: %+v", al)
	}
}

// U23: an empty ALLOW blocks everything (an allowlist with no domains), never "allow all".
func TestRenderEmptyAllowDeniesAll(t *testing.T) {
	r := render(t, Options{Listen: "0.0.0.0:3128", Allow: []string{""}})
	if d := r.Transforms[0].Config.Domains; d == nil || len(d) != 0 {
		t.Fatalf("domains %v, want an empty list", d)
	}
}

// Every rule with a good value, a bad value, and the value an attacker would try.
func TestRenderRejects(t *testing.T) {
	for _, c := range []struct {
		name string
		o    Options
		ok   bool
	}{
		{"good", Options{Listen: "0.0.0.0:3128", Allow: []string{"pypi.org"}}, true},
		{"wildcard", Options{Listen: "0.0.0.0:3128", Allow: []string{"*.pythonhosted.org"}}, true},
		{"ip literal", Options{Listen: "0.0.0.0:3128", Allow: []string{"1.1.1.1"}}, false},
		{"allow all", Options{Listen: "0.0.0.0:3128", Allow: []string{"*"}}, false},
		{"glob inside a label", Options{Listen: "0.0.0.0:3128", Allow: []string{"py*.org"}}, false},
		{"character class", Options{Listen: "0.0.0.0:3128", Allow: []string{"pypi.or[g]"}}, false},
		{"port", Options{Listen: "0.0.0.0:3128", Allow: []string{"pypi.org:22"}}, false},
		{"uppercase", Options{Listen: "0.0.0.0:3128", Allow: []string{"PyPI.org"}}, false},
		{"newline smuggles a key", Options{Listen: "0.0.0.0:3128", Allow: []string{"pypi.org\n\"cidrs\": [\"0.0.0.0/0\"]"}}, false},
		{"quote breaks json", Options{Listen: "0.0.0.0:3128", Allow: []string{`pypi.org"`}}, false},
		{"no port", Options{Listen: "0.0.0.0", Allow: nil}, false},
		{"name not ip", Options{Listen: "proxy:3128", Allow: nil}, false},
		{"port zero", Options{Listen: "0.0.0.0:0", Allow: nil}, false},
	} {
		_, err := Render(c.o)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestCheckPorts(t *testing.T) {
	for p, ok := range map[string]bool{"443": true, "443,443": true, "22": false, "443,22": false, "4430": false, "": false, " 443": true} {
		if err := CheckPorts(p); (err == nil) != ok {
			t.Errorf("CheckPorts(%q) = %v, want ok=%v", p, err, ok)
		}
	}
}

// U23: iron-proxy never dials an address burrow-proxy refuses: every address that
// proxy.Policy.AddrAllowed rejects is inside a deny CIDR.
func TestDenyCIDRsCoverBurrowProxy(t *testing.T) {
	var deny []netip.Prefix
	for _, c := range DenyCIDRs {
		deny = append(deny, netip.MustParsePrefix(c))
	}
	pol := &proxy.Policy{}
	for _, s := range []string{
		"0.0.0.0", "10.1.2.3", "100.64.0.1", "100.127.255.254", "127.0.0.1", "169.254.169.254",
		"172.16.0.1", "172.31.255.255", "192.168.1.1", "224.0.0.1", "255.255.255.255",
		"::", "::1", "fe80::1", "fc00::1", "fd00:ec2::254", "ff02::1", "::ffff:10.0.0.1",
	} {
		a := netip.MustParseAddr(s)
		if pol.AddrAllowed(a) {
			t.Errorf("%s: burrow-proxy allows it; fix the test list", s)
			continue
		}
		if !slices.ContainsFunc(deny, func(p netip.Prefix) bool { return p.Contains(a.Unmap()) }) {
			t.Errorf("%s: refused by burrow-proxy, not in DenyCIDRs", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "151.101.64.223", "2606:4700:4700::1111"} {
		a := netip.MustParseAddr(s)
		if slices.ContainsFunc(deny, func(p netip.Prefix) bool { return p.Contains(a) }) {
			t.Errorf("%s: a public address is denied", s)
		}
	}
	r := render(t, Options{Listen: "0.0.0.0:3128"})
	if !slices.Equal(r.Proxy.Deny, DenyCIDRs) {
		t.Errorf("rendered deny list %v", r.Proxy.Deny)
	}
	r = render(t, Options{Listen: "0.0.0.0:3128", AllowPrivate: true})
	if !slices.Contains(r.Proxy.Deny, "127.0.0.0/8") || !slices.Contains(r.Proxy.Deny, "169.254.169.254/32") {
		t.Errorf("tests only: loopback and metadata stay denied: %v", r.Proxy.Deny)
	}
}
