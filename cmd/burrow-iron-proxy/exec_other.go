//go:build !linux

package main

import "errors"

// execIron is Linux only: the proxy image is a Linux image.
func execIron(string, []string, []string) error {
	return errors.New("burrow-iron-proxy runs only on Linux (inside its container)")
}
