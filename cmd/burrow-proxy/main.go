// Command burrow-proxy is the test stand-in egress proxy (see internal/proxy). It runs inside its
// own container, one per sandbox and per preset run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/common-sec/burrow-sandbox/internal/proxy"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:3128", "address to listen on")
	allow := flag.String("allow", "", "comma-separated host names; *.domain allows subdomains")
	ports := flag.String("ports", "443", "comma-separated destination ports")
	private := flag.Bool("allow-private", false, "TESTS ONLY: let allowed names resolve to private addresses")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	pol := &proxy.Policy{AllowPrivate: *private}
	for _, h := range strings.Split(*allow, ",") {
		if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
			pol.Hosts = append(pol.Hosts, h)
		}
	}
	for _, p := range strings.Split(*ports, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > 65535 {
			fmt.Fprintf(os.Stderr, "burrow-proxy: bad port %q\n", p)
			os.Exit(2)
		}
		pol.Ports = append(pol.Ports, n)
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           &proxy.Server{Policy: pol, Log: log},
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	log.Info("listening", "addr", *listen, "allow", pol.Hosts, "ports", pol.Ports)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
