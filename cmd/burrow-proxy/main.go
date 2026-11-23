// Command burrow-proxy is a sandbox's only exit. It runs inside its own container, one per
// sandbox and per tool run, and takes one set of flags whatever engine enforces them
// (decision 0013):
//
//   - iron (default): iron-proxy (github.com/ironsh/iron-proxy), built into the same image.
//     burrow-proxy renders iron-proxy's configuration from the flags (internal/ironproxy),
//     writes it to the container's tmpfs and replaces itself with iron-proxy, started with an
//     empty environment.
//   - embedded: burrow's own small CONNECT proxy (internal/proxy), in this process.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/common-sec/burrow-sandbox/internal/ironproxy"
	"github.com/common-sec/burrow-sandbox/internal/proxy"
)

// Engines. burrow passes the sandbox's PROXY setting as -engine.
const (
	engineIron     = "iron"
	engineEmbedded = "embedded"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "burrow-proxy:", err)
		os.Exit(2)
	}
}

// options are the flags every engine takes.
type options struct {
	listen, allow, ports string
	private              bool
	dir, iron            string // iron engine: tmpfs for the configuration, the iron-proxy binary
	printOnly            bool   // iron engine: print the configuration and exit
}

func run(args []string) error {
	fs := flag.NewFlagSet("burrow-proxy", flag.ContinueOnError)
	engine := fs.String("engine", engineIron, "iron (iron-proxy) or embedded (burrow's own CONNECT proxy)")
	var o options
	fs.StringVar(&o.listen, "listen", "0.0.0.0:3128", "address the sandbox connects to (CONNECT; iron: also SOCKS5)")
	fs.StringVar(&o.allow, "allow", "", "comma-separated host names; *.domain allows subdomains")
	fs.StringVar(&o.ports, "ports", "443", "comma-separated destination ports (iron: only 443)")
	fs.BoolVar(&o.private, "allow-private", false, "TESTS ONLY: let allowed names resolve to private addresses")
	fs.StringVar(&o.dir, "dir", "/run/burrow", "iron: tmpfs folder for the rendered configuration")
	fs.StringVar(&o.iron, "iron", "/iron-proxy", "iron: the iron-proxy binary")
	fs.BoolVar(&o.printOnly, "print", false, "iron: print the configuration and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if o.private {
		fmt.Fprintln(os.Stderr, "burrow-proxy: warning: -allow-private (tests only): allowed names may resolve to private addresses")
	}
	switch *engine {
	case engineIron:
		return runIron(o)
	case engineEmbedded:
		return runEmbedded(o)
	}
	return fmt.Errorf("-engine %q: iron or embedded", *engine)
}

func runIron(o options) error {
	if err := ironproxy.CheckPorts(o.ports); err != nil {
		return err
	}
	cfg, err := ironproxy.Render(ironproxy.Options{
		Listen: o.listen, Allow: strings.Split(o.allow, ","), AllowPrivate: o.private,
	})
	if err != nil {
		return err
	}
	if o.printOnly {
		_, err := os.Stdout.Write(append(cfg, '\n'))
		return err
	}
	path := filepath.Join(o.dir, "iron-proxy.json")
	if err := os.WriteFile(path, cfg, 0o600); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	// No environment: iron-proxy reads IRON_* overrides and HTTP(S)_PROXY from it, and nothing
	// but the rendered file may configure it.
	return execIron(o.iron, []string{"iron-proxy", "-config", path}, []string{})
}

func runEmbedded(o options) error {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	pol := &proxy.Policy{AllowPrivate: o.private}
	for _, h := range strings.Split(o.allow, ",") {
		if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
			pol.Hosts = append(pol.Hosts, h)
		}
	}
	for _, p := range strings.Split(o.ports, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("bad port %q", p)
		}
		pol.Ports = append(pol.Ports, n)
	}
	srv := &http.Server{
		Addr:              o.listen,
		Handler:           &proxy.Server{Policy: pol, Log: log},
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh) //nolint:errcheck // shutting down; ListenAndServe reports the outcome
	}()
	log.Info("listening", "engine", engineEmbedded, "addr", o.listen, "allow", pol.Hosts, "ports", pol.Ports)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
