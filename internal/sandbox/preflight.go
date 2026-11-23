package sandbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/common-sec/burrow-sandbox/internal/podman"
)

// MinPodman is the oldest Podman burrow is tested with (docs/TESTING.md).
const MinPodman = "4.9"

// Preflight checks that the Podman burrow talks to (on Windows and macOS, the one in the
// machine) is new enough and uses the netavark network backend. Older setups fail deep inside
// podman with errors that do not point at the cause, such as CNI plugins that cannot read the
// network config Podman writes ("plugin firewall does not support config version 1.0.0").
func Preflight(ctx context.Context, r podman.Runner) error {
	out, err := r.Output(ctx, "info", "--format", "{{.Version.Version}} {{.Host.NetworkBackend}}")
	if err != nil {
		// Podman 3 has no NetworkBackend field, so the template itself fails there.
		return fmt.Errorf("podman %s or newer with the netavark network backend is needed: %w", MinPodman, err)
	}
	return checkEngine(out)
}

func checkEngine(info string) error {
	f := strings.Fields(info)
	if len(f) != 2 {
		return fmt.Errorf("podman info: unexpected output %q", info)
	}
	version, backend := f[0], f[1]
	if !atLeast(version, MinPodman) {
		return fmt.Errorf("podman %s is too old: burrow needs %s or newer (on Ubuntu, 24.04 ships 4.9; see docs/WINDOWS.md for WSL)", version, MinPodman)
	}
	if backend != "netavark" {
		return fmt.Errorf("podman uses the %q network backend: burrow needs netavark (install netavark and aardvark-dns, "+
			"set network_backend = \"netavark\" under [network] in containers.conf, then `podman system reset`, "+
			"which deletes local images and containers)", backend)
	}
	return nil
}

// atLeast compares dotted versions numerically ("4.10.1" >= "4.9"); a suffix like "-dev" is ignored.
func atLeast(have, want string) bool {
	h, w := strings.Split(strings.SplitN(have, "-", 2)[0], "."), strings.Split(want, ".")
	for i := range w {
		if i >= len(h) {
			return false
		}
		a, errA := strconv.Atoi(h[i])
		b, errB := strconv.Atoi(w[i])
		if errA != nil || errB != nil {
			return false
		}
		if a != b {
			return a > b
		}
	}
	return true
}
