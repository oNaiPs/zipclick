//go:build !windows

package main

import "errors"

// Outside Windows there is no system tar.exe to hand archives to; ZipClick's
// own engine extracts everything.

func systemTarAvailable() bool { return false }

func systemExtract(string, string, systemPlan, *Progress) error { return errUseEngine }

func savedBackend() string { return "" }

func saveBackend(backendMode) error { return errors.New("only available on Windows") }
