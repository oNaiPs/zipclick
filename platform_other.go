//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
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

func messageBox(title, msg string, yesNo bool) bool {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, msg)
	return false
}
