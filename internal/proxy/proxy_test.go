package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestAllowed(t *testing.T) {
	p := &Policy{Hosts: []string{"pypi.org", "*.example.com"}}
	cases := []struct {
		host string
		port int
		want bool
	}{
		{"pypi.org", 443, true},
		{"PyPI.org.", 443, true},
		{"pypi.org", 80, false},           // port not allowed
		{"evil-pypi.org", 443, false},     // not a suffix match
		{"pypi.org.evil.net", 443, false}, // prefix trick
		{"a.example.com", 443, true},      // subdomain
		{"example.com", 443, false},       // *.domain does not cover the domain itself
		{"a.b.example.com", 443, true},    // deeper subdomain
		{"aexample.com", 443, false},      // no dot boundary
		{"1.1.1.1", 443, false},           // IP literal
		{"[2606:4700::1111]", 443, false}, // IPv6 literal
		{"2606:4700::1111", 443, false},   // IPv6 literal, no brackets
	}
	for _, c := range cases {
		if got := p.Allowed(c.host, c.port); got != c.want {
			t.Errorf("Allowed(%q, %d) = %v, want %v", c.host, c.port, got, c.want)
		}
	}
}

func TestAddrAllowed(t *testing.T) {
	p := &Policy{}
	for _, a := range []string{
		"10.0.0.1", "192.168.1.1", "172.16.0.1", "127.0.0.1", "169.254.169.254",
		"100.64.0.1", "::1", "fd00::1", "fe80::1", "0.0.0.0", "::ffff:10.0.0.1",
	} {
		if p.AddrAllowed(netip.MustParseAddr(a)) {
			t.Errorf("%s allowed, want refused", a)
		}
	}
	if !p.AddrAllowed(netip.MustParseAddr("151.101.0.223")) {
		t.Error("public address refused")
	}
}

// startProxy runs the proxy with a fake resolver that maps every name to the given address.
func startProxy(t *testing.T, pol *Policy, to netip.Addr) *httptest.Server {
	t.Helper()
	s := &Server{
		Policy: pol, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Lookup: func(context.Context, string) ([]netip.Addr, error) { return []netip.Addr{to}, nil },
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}

func TestConnectEndToEnd(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()
	_, portStr, _ := net.SplitHostPort(target.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	loop := netip.MustParseAddr("127.0.0.1")

	get := func(pol *Policy, host string) (int, error) {
		px := startProxy(t, pol, loop)
		pu, _ := url.Parse(px.URL)
		tr := &http.Transport{
			Proxy:           http.ProxyURL(pu),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		} //nolint:gosec // test target, self-signed
		defer tr.CloseIdleConnections()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://"+host+":"+portStr+"/", nil)
		resp, err := (&http.Client{Transport: tr}).Do(req)
		if err != nil {
			return 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode, nil
	}

	// Allowed name, private address permitted for the test: the request goes through.
	if code, err := get(&Policy{Hosts: []string{"target.test"}, Ports: []int{port}, AllowPrivate: true}, "target.test"); err != nil || code != 200 {
		t.Fatalf("allowed host: code %d, err %v", code, err)
	}
	// Other name: refused at CONNECT.
	if _, err := get(&Policy{Hosts: []string{"target.test"}, Ports: []int{port}, AllowPrivate: true}, "other.test"); err == nil {
		t.Fatal("other host: want error")
	}
	// Allowed name resolving to loopback without AllowPrivate: refused (DNS rebinding).
	if _, err := get(&Policy{Hosts: []string{"target.test"}, Ports: []int{port}}, "target.test"); err == nil {
		t.Fatal("private address: want error")
	}
}

func TestOnlyConnect(t *testing.T) {
	px := startProxy(t, &Policy{Hosts: []string{"target.test"}}, netip.MustParseAddr("127.0.0.1"))
	c, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", px.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_, _ = io.WriteString(c, "GET http://target.test/ HTTP/1.1\r\nHost: target.test\r\n\r\n")
	line, _ := bufio.NewReader(c).ReadString('\n')
	if !strings.Contains(line, "405") {
		t.Fatalf("plain HTTP: got %q, want 405", line)
	}
}
