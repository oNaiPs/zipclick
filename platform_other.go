//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
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

// freeSpace is how many bytes may still be written on dir's file system.
func freeSpace(dir string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true
}

// renameNoReplace is os.Rename that refuses an existing destination. Outside
// Windows it is only for the tests, so the check-then-rename race is fine.
func renameNoReplace(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: os.ErrExist}
	}
	return os.Rename(from, to)
}

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
