// Package proxy is a minimal HTTPS CONNECT proxy with a host allowlist.
//
// It is a TEMPORARY STAND-IN, the Go version of the research PoCs' proxy.py, until a proxy
// solution is chosen (an authenticating proxy with an HTTPS allowlist by host, method and path,
// R7; roadmap 0.3). It does not inspect TLS, does not inject credentials and has no method or
// path rules, and it is not extended except for a test or an investigation. It does enforce:
//   - only CONNECT, only to allowed host names (exact, or *.domain for subdomains);
//   - only allowed ports (443 by default);
//   - no IP literals, and no allowed name that resolves to a private, loopback or link-local
//     address (DNS rebinding, metadata endpoints), unless AllowPrivate is set for tests;
//   - one JSON audit line per decision.
package proxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Policy decides which CONNECT targets are allowed.
type Policy struct {
	Hosts        []string // exact names, or *.domain for any subdomain (not the domain itself)
	Ports        []int    // default [443]
	AllowPrivate bool     // tests only: let allowed names resolve to private addresses
}

// Allowed reports whether host:port may be dialled, before resolution.
func (p *Policy) Allowed(host string, port int) bool {
	if !p.portOK(port) {
		return false
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return false // IP literal
	}
	for _, h := range p.Hosts {
		if suffix, ok := strings.CutPrefix(h, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
		} else if host == h {
			return true
		}
	}
	return false
}

func (p *Policy) portOK(port int) bool {
	ports := p.Ports
	if len(ports) == 0 {
		ports = []int{443}
	}
	for _, x := range ports {
		if x == port {
			return true
		}
	}
	return false
}

// AddrAllowed reports whether a resolved address may be dialled.
func (p *Policy) AddrAllowed(a netip.Addr) bool {
	a = a.Unmap()
	if p.AllowPrivate {
		return !a.IsUnspecified() && !a.IsMulticast()
	}
	return a.IsGlobalUnicast() && !a.IsPrivate() && !isShared(a)
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func isShared(a netip.Addr) bool { return cgnat.Contains(a) }

// Server is the proxy.
type Server struct {
	Policy *Policy
	Log    *slog.Logger
	// Lookup resolves a host name; nil uses the system resolver. Tests replace it.
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	Dialer *net.Dialer // nil: 10 s timeout
}

// ServeHTTP handles one proxy request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	audit := func(action string, status int, reason string) {
		s.Log.Info("request", "method", r.Method, "target", r.Host, "client", r.RemoteAddr,
			"action", action, "status", status, "reason", reason, "ms", time.Since(start).Milliseconds())
	}
	if r.Method != http.MethodConnect {
		audit("reject", http.StatusMethodNotAllowed, "only CONNECT")
		http.Error(w, "only CONNECT (HTTPS) is supported", http.StatusMethodNotAllowed)
		return
	}
	host, portStr, err := net.SplitHostPort(r.Host)
	port, perr := strconv.Atoi(portStr)
	if err != nil || perr != nil {
		audit("reject", http.StatusBadRequest, "bad target")
		http.Error(w, "bad target", http.StatusBadRequest)
		return
	}
	if !s.Policy.Allowed(host, port) {
		audit("reject", http.StatusForbidden, "not in allowlist")
		http.Error(w, "blocked by burrow proxy policy", http.StatusForbidden)
		return
	}
	upstream, err := s.dial(r.Context(), host, port)
	if err != nil {
		audit("reject", http.StatusBadGateway, err.Error())
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		audit("error", http.StatusInternalServerError, "no hijack")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	client, buf, err := hj.Hijack()
	if err != nil {
		_ = upstream.Close()
		audit("error", http.StatusInternalServerError, "hijack failed")
		return
	}
	audit("allow", http.StatusOK, "")
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	// Bytes the client already sent after the CONNECT line (e.g. a TLS ClientHello).
	if n := buf.Reader.Buffered(); n > 0 {
		pending, _ := buf.Peek(n)
		if _, err := upstream.Write(pending); err != nil {
			_ = client.Close()
			_ = upstream.Close()
			return
		}
	}
	Pipe(client, upstream)
}

// dial resolves the name itself, checks every address, and dials a checked address, so a name
// cannot be rebound to a private address between the check and the dial.
func (s *Server) dial(ctx context.Context, host string, port int) (net.Conn, error) {
	lookup := s.Lookup
	if lookup == nil {
		lookup = func(ctx context.Context, h string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", h)
		}
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, errors.New("resolve failed")
	}
	d := s.Dialer
	if d == nil {
		d = &net.Dialer{Timeout: 10 * time.Second}
	}
	for _, a := range addrs {
		if !s.Policy.AddrAllowed(a) {
			return nil, errors.New("resolves to a non-public address")
		}
	}
	last := errors.New("no address")
	for _, a := range addrs {
		c, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(a.Unmap(), uint16(port)).String()) //nolint:gosec // port checked by Policy
		if err == nil {
			return c, nil
		}
		last = err
	}
	return nil, last
}

// Pipe copies both ways until either side closes, then closes both.
func Pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	wg.Add(2)
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}
