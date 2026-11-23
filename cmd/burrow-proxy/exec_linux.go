//go:build linux

package main

import (
	"fmt"
	"syscall"
)

// execIron replaces this process with iron-proxy, so podman's stop signal and exit code reach it
// directly and no launcher stays in the container.
func execIron(bin string, argv, env []string) error {
	// bin is a fixed path in the image (a flag for tests); argv is built above, not user input.
	if err := syscall.Exec(bin, argv, env); err != nil { //nolint:gosec // G204: fixed binary and arguments
		return fmt.Errorf("exec %s: %w", bin, err)
	}
	return nil
}
