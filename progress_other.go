//go:build !windows

package main

// withProgress just runs work: the progress window is Windows-only.
func withProgress(_ string, _ *Progress, work func() error) error { return work() }

// askPassword has no dialog to show outside Windows; use --password.
func askPassword(string, bool) (string, bool) { return "", false }
