// Package forward relays TCP connections from one listening address to one fixed target.
//
// It publishes a port of a sandbox that is already running (R9): Podman cannot add a published
// port to a running container, so burrow starts this forwarder next to it. The target is fixed at
// start; the forwarder never dials anything else, so it is not a way out for the sandbox.
package forward

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/common-sec/burrow-sandbox/internal/proxy"
)

// Serve accepts on ln and relays each connection to target until ctx is done.
func Serve(ctx context.Context, ln net.Listener, target string, log *slog.Logger) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	d := net.Dialer{Timeout: 5 * time.Second}
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr // Accept fails because shutdown closed the listener
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go func() {
			up, err := d.DialContext(ctx, "tcp", target)
			if err != nil {
				log.Warn("dial failed", "client", c.RemoteAddr().String(), "target", target, "err", err)
				_ = c.Close()
				return
			}
			proxy.Pipe(c, up)
		}()
	}
}
