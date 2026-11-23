//go:build !linux

package main

import "errors"

// execIron is Linux only: the proxy image is a Linux image.
func execIron(string, []string, []string) error {
	return errors.New("the iron engine runs only on Linux (inside the proxy container)")
}
