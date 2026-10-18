// ZipClick makes double-clicking an archive on Windows (.zip, .7z, .rar,
// .tar.gz and the like) extract it, the way Archive Utility does on macOS.
//
//	zipclick ARCHIVE...    extract each archive next to itself
//	zipclick               (run from Explorer) offer to install
//	zipclick install       copy to %LOCALAPPDATA%\Programs\ZipClick and register
//	zipclick uninstall     remove the registration and the installed copy
//	zipclick status        print where .zip double-clicks currently go
//
// --quiet (anywhere) replaces dialogs with text on stderr, for scripts, and
// --password=PW supplies the password that would otherwise be asked for.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const appName = "ZipClick"

var (
	quiet    bool
	password string
)

func main() {
	setLanguage()
	var args []string
	for _, a := range os.Args[1:] {
		if a == "--quiet" || a == "-q" {
			quiet = true
		} else if pw, ok := strings.CutPrefix(a, "--password="); ok {
			password = pw
		} else {
			args = append(args, a)
		}
	}

	if len(args) == 0 {
		if !quiet && ask(appName, msg.InstallPrompt) {
			args = []string{"install"}
		} else {
			fmt.Fprintln(os.Stderr, "usage: zipclick [--quiet] [--password=PW] ARCHIVE... | install | uninstall | status")
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
		dst, err := extract(a)
		if errors.Is(err, ErrCancelled) {
			continue
		}
		if err != nil {
			failed = true
			report(a, err)
			continue
		}
		notifyCreated(dst)
		if quiet {
			fmt.Println(dst)
		} else {
			reveal(dst)
		}
	}
	if failed {
		os.Exit(1)
	}
}

// extract extracts one archive, with the progress window, asking for the
// password for as long as the archive needs one and the user keeps trying.
func extract(archive string) (string, error) {
	pw := password
	for {
		var dst string
		p := &Progress{}
		err := withProgress(archive, p, func() (err error) {
			dst, err = Extract(archive, pw, p)
			return err
		})
		wrong := errors.Is(err, ErrBadPassword)
		if quiet || !(wrong || errors.Is(err, ErrEncrypted)) {
			return dst, err
		}
		var ok bool
		if pw, ok = askPassword(filepath.Base(archive), wrong); !ok {
			return "", ErrCancelled
		}
	}
}

func report(archive string, err error) {
	text := fmt.Sprintf(msg.CouldNotExtract, filepath.Base(archive)) + "\n\n" + describe(err)
	var me *MethodError
	if errors.As(err, &me) {
		text += "\n\n" + msg.TryExtractAll
	}
	alert(appName, text)
}

// describe says why an extraction failed, in the user's language when the
// cause is one ZipClick knows; anything else is the system's own message,
// which Windows already words in the user's language.
func describe(err error) string {
	var me *MethodError
	switch {
	case errors.Is(err, ErrEncrypted):
		return msg.Encrypted
	case errors.Is(err, ErrBadPassword):
		return msg.BadPassword
	case errors.Is(err, ErrUnknownFormat):
		return msg.NotArchive
	case errors.Is(err, ErrEmpty):
		return msg.Empty
	case errors.As(err, &me):
		return fmt.Sprintf(msg.Method, me.Method)
	case damaged(err):
		return msg.Damaged
	}
	s := err.Error()
	return strings.ToUpper(s[:1]) + s[1:]
}

// damaged reports a failure that came from the archive's contents rather
// than from the disk: the decoders' errors are all of that kind, and the
// system's (no space, access denied, path too long) all carry a path or an
// errno.
func damaged(err error) bool {
	var pe *fs.PathError
	var le *os.LinkError
	var se *os.SyscallError
	return !errors.As(err, &pe) && !errors.As(err, &le) && !errors.As(err, &se)
}

func exit(err error) {
	if err != nil {
		alert(appName, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

// alert shows msg in a dialog, or on stderr with --quiet.
func alert(title, text string) {
	if quiet {
		fmt.Fprintln(os.Stderr, text)
		return
	}
	messageBox(title, text, boxWarning)
}

// ask asks a yes/no question; with --quiet the answer is no.
func ask(title, text string) bool {
	if quiet {
		return false
	}
	return messageBox(title, text, boxQuestion)
}

// info tells the user something that went right; silent with --quiet.
func info(title, text string) {
	if quiet {
		fmt.Println(text)
		return
	}
	messageBox(title, text, boxInfo)
}

// The icon and buttons of a message box.
const (
	boxInfo = iota
	boxWarning
	boxQuestion // Yes/No; messageBox returns true for Yes
)
