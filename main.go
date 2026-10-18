// ZipClick makes double-clicking a .zip on Windows extract it, the way
// Archive Utility does on macOS.
//
//	zipclick FILE.zip...   extract each archive next to itself
//	zipclick               (run from Explorer) offer to install
//	zipclick install       copy to %LOCALAPPDATA%\Programs\ZipClick and register
//	zipclick uninstall     remove the registration and the installed copy
//	zipclick status        print where .zip double-clicks currently go
//
// --quiet (anywhere) replaces dialogs with text on stderr, for scripts.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const appName = "ZipClick"

var quiet bool

func main() {
	var args []string
	for _, a := range os.Args[1:] {
		if a == "--quiet" || a == "-q" {
			quiet = true
		} else {
			args = append(args, a)
		}
	}

	if len(args) == 0 {
		if !quiet && ask(appName, "Make double-clicking a .zip file extract it, like on a Mac?\n\n"+
			"ZipClick installs for your user only — no administrator rights needed.") {
			args = []string{"install"}
		} else {
			fmt.Fprintln(os.Stderr, "usage: zipclick FILE.zip... | install | uninstall | status")
			os.Exit(2)
		}
	}

	switch strings.ToLower(args[0]) {
	case "install":
		exit(install())
	case "uninstall":
		exit(uninstall())
	case "status":
		exit(status())
	}

	failed := false
	for _, a := range args {
		dst, err := Extract(a)
		if err != nil {
			failed = true
			report(a, err)
			continue
		}
		notifyCreated(dst)
		if quiet {
			fmt.Println(dst)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func report(archive string, err error) {
	why := err.Error()
	why = strings.ToUpper(why[:1]) + why[1:] + "."
	msg := fmt.Sprintf("Could not extract %s.\n\n%s", filepath.Base(archive), why)
	if errors.Is(err, ErrEncrypted) || strings.Contains(err.Error(), "not supported") {
		msg += "\n\nRight-click the file → Open with → Windows Explorer to open it the usual way."
	}
	alert(appName, msg)
}

func exit(err error) {
	if err != nil {
		alert(appName, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

// alert shows msg in a dialog, or on stderr with --quiet.
func alert(title, msg string) {
	if quiet {
		fmt.Fprintln(os.Stderr, msg)
		return
	}
	messageBox(title, msg, false)
}

// ask asks a yes/no question; with --quiet the answer is no.
func ask(title, msg string) bool {
	if quiet {
		return false
	}
	return messageBox(title, msg, true)
}

// info tells the user something that went right; silent with --quiet.
func info(title, msg string) {
	if quiet {
		fmt.Println(msg)
		return
	}
	messageBox(title, msg, false)
}
