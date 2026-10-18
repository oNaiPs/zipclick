//go:build !windows

package main

// withProgress just runs work: the progress window is Windows-only.
func withProgress(_ string, _ *Progress, work func() error) error { return work() }
