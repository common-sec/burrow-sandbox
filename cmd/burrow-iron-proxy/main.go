// Command burrow-iron-proxy starts iron-proxy (github.com/ironsh/iron-proxy) as a sandbox's proxy
// (`PROXY=iron`, decision 0013). It is the entry point of the burrow-iron-proxy image and takes
// the same flags as burrow-proxy, so burrow starts either proxy the same way. It renders the
// configuration (internal/ironproxy), writes it to the container's tmpfs, and replaces itself
// with iron-proxy, which then logs every decision as JSON on stdout.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/common-sec/burrow-sandbox/internal/ironproxy"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "burrow-iron-proxy:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("burrow-iron-proxy", flag.ContinueOnError)
	listen := fs.String("listen", "0.0.0.0:3128", "address of the tunnel listener (CONNECT, SOCKS5)")
	allow := fs.String("allow", "", "comma-separated host names; *.domain allows subdomains")
	ports := fs.String("ports", "443", "destination ports: only 443 (iron-proxy in sni-only mode)")
	private := fs.Bool("allow-private", false, "TESTS ONLY: let allowed names resolve to private addresses")
	dir := fs.String("dir", "/run/burrow", "tmpfs folder for the rendered configuration")
	iron := fs.String("iron", "/iron-proxy", "the iron-proxy binary")
	printOnly := fs.Bool("print", false, "print the configuration and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if err := ironproxy.CheckPorts(*ports); err != nil {
		return err
	}
	cfg, err := ironproxy.Render(ironproxy.Options{
		Listen: *listen, Allow: strings.Split(*allow, ","), AllowPrivate: *private,
	})
	if err != nil {
		return err
	}
	if *printOnly {
		_, err := os.Stdout.Write(append(cfg, '\n'))
		return err
	}
	if *private {
		fmt.Fprintln(os.Stderr, "burrow-iron-proxy: warning: -allow-private (tests only): allowed names may resolve to private addresses")
	}
	path := filepath.Join(*dir, "iron-proxy.json")
	if err := os.WriteFile(path, cfg, 0o600); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	// No environment: iron-proxy reads IRON_* overrides and HTTP(S)_PROXY from it, and nothing
	// but the rendered file may configure it.
	return execIron(*iron, []string{"iron-proxy", "-config", path}, []string{})
}
