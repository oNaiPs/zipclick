//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Stand-ins so the extraction engine builds and tests on any OS.

var errWindowsOnly = errors.New("only available on Windows")

func oemCodePage() uint32  { return 437 }
func hide(string)          {}
func unhide(string)        {}
func notifyCreated(string) {}
func reveal(string)        {}
func install() error       { return errWindowsOnly }
func uninstall() error     { return errWindowsOnly }
func status() error        { return errWindowsOnly }

func messageBox(title, text string, kind int) bool {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
	return false
}

// preferredLanguages reads the POSIX locale ("pt_PT.UTF-8" → "pt_PT").
func preferredLanguages() []string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l, _, _ := strings.Cut(os.Getenv(v), "."); l != "" && l != "C" && l != "POSIX" {
			return []string{l}
		}
	}
	return nil
}
