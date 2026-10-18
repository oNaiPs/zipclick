//go:build !windows

package main

import (
	"errors"
	"os"
)

// Outside Windows there is no system tar.exe to hand archives to; ZipClick's
// own engine extracts everything.

func systemTarAvailable() bool { return false }

func systemExtract(string, string, systemPlan, *Progress) ([]string, error) {
	return nil, errUseEngine
}

func savedBackend() string { return "" }

func saveBackend(backendMode) error { return errors.New("only available on Windows") }

// removeVerbatim deletes p; a link is removed, never followed.
func removeVerbatim(p string) error { return os.RemoveAll(p) }
