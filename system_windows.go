//go:build windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// tarExe is Windows' own tar.exe, by full path: never one found on PATH.
func tarExe() string {
	sys32, err := windows.GetSystemDirectory()
	if err != nil {
		return ""
	}
	return filepath.Join(sys32, "tar.exe")
}

func systemTarAvailable() bool {
	p := tarExe()
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// systemExtract has tar.exe extract archive into stage and returns the
// members it reported failing on. err is nil when tar.exe succeeded;
// errPartial when it failed only on those members (whose files, if any,
// must not be trusted); errUseEngine when it failed in a way that is not
// tied to a member, and nothing it wrote can be trusted; ErrCancelled or
// ErrDiskFull when it was stopped for those reasons.
func systemExtract(archive, stage string, plan systemPlan, p *Progress) (failed []string, err error) {
	if plan.bytes > 0 {
		if err := roomFor(stage, plan.bytes); err != nil {
			return nil, err
		}
	}
	src := archive
	if plan.stdin {
		src = "-"
	}
	// -v lists each member on stderr as "x name", which is the progress for
	// formats tar.exe has to seek in. Without -P, libarchive refuses ".."
	// and absolute paths and will not write through a link.
	cmd := exec.Command(tarExe(), "-x", "-v", "-f", src, "-C", stage)
	cmd.Dir = stage
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if plan.stdin {
		f, err := os.Open(archive)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil {
			p.setTotal(fi.Size())
		}
		cmd.Stdin = &progressReader{f, p}
	} else {
		p.setTotal(plan.bytes)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: %v", errUseEngine, err)
	}

	var stopped atomic.Value // the error to report once tar.exe is killed
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		// Checked at once, then every tick: a drive already short of the
		// reserve stops tar.exe before it writes much.
		for first := true; ; first = false {
			if !first {
				select {
				case <-done:
					return
				case <-t.C:
				}
			}
			var why error
			if p.stopped() {
				why = ErrCancelled
			} else if err := roomFor(stage, 0); err != nil {
				why = err
			}
			if why != nil {
				stopped.Store(why)
				_ = cmd.Process.Kill()
				return
			}
		}
	}()

	// Each member is "x NAME", or "x NAME: MESSAGE" when it failed. Any
	// other line is about the archive as a whole, and only these two are
	// known to be harmless.
	var unexplained []string
	n := 0
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if name, ok := strings.CutPrefix(line, "x "); ok {
			if i := strings.Index(name, ": "); i >= 0 {
				failed = append(failed, name[:i])
			}
			n++
			if !plan.stdin && plan.entries > 0 && plan.bytes > 0 && n <= plan.entries {
				p.setDone(plan.bytes * int64(n) / int64(plan.entries))
			}
			continue
		}
		if strings.Contains(line, "Error exit delayed from previous errors") ||
			strings.Contains(line, "Archive entry has empty or unreadable filename") {
			continue
		}
		if line != "" && len(unexplained) < 5 {
			unexplained = append(unexplained, line)
		}
	}
	werr := cmd.Wait()
	close(done)
	if why, ok := stopped.Load().(error); ok {
		return nil, why
	}
	switch {
	case len(unexplained) > 0:
		return nil, fmt.Errorf("%w: tar.exe: %s", errUseEngine, strings.Join(unexplained, "; "))
	case werr != nil:
		return failed, fmt.Errorf("%w: tar.exe: %v", errPartial, werr)
	}
	p.setDone(p.Total())
	return failed, nil
}

// removeVerbatim deletes p without Windows reinterpreting the name: through
// a \\?\ path, "aux.c" is a file, not the AUX device. A link is removed,
// never followed.
func removeVerbatim(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	return os.RemoveAll(`\\?\` + abs)
}

// savedBackend is the backend chosen with "zipclick backend MODE".
func savedBackend() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\`+appName, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, _ := k.GetStringValue("Backend")
	return v
}

func saveBackend(m backendMode) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\`+appName, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if m == backendAuto {
		if err := k.DeleteValue("Backend"); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	return k.SetStringValue("Backend", string(m))
}
