// Command burrow-forward relays one TCP port to one fixed target (see internal/forward).
package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/common-sec/burrow-sandbox/internal/forward"
)

func main() {
	listen := flag.String("listen", "", "address to listen on, e.g. 0.0.0.0:8080")
	target := flag.String("target", "", "fixed target, e.g. 10.89.0.5:8080")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if *listen == "" || *target == "" {
		log.Error("both -listen and -target are required")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *listen)
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	log.Info("forwarding", "listen", *listen, "target", *target)
	if err := forward.Serve(ctx, ln, *target, log); err != nil {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
