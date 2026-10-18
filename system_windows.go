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

// systemExtract has tar.exe extract archive into stage. It returns
// errUseEngine (wrapped) when tar.exe could not, and ErrCancelled or
// ErrDiskFull when it was stopped for those reasons.
func systemExtract(archive, stage string, plan systemPlan, p *Progress) error {
	if plan.bytes > 0 {
		if err := roomFor(stage, plan.bytes); err != nil {
			return err
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
			return err
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
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %v", errUseEngine, err)
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

	var lines []string
	n := 0
	sc := bufio.NewScanner(stderr)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "x ") {
			n++
			if !plan.stdin && plan.entries > 0 && plan.bytes > 0 && n <= plan.entries {
				p.setDone(plan.bytes * int64(n) / int64(plan.entries))
			}
		} else if len(lines) < 5 {
			lines = append(lines, line)
		}
	}
	err = cmd.Wait()
	close(done)
	if why, ok := stopped.Load().(error); ok {
		return why
	}
	if err != nil {
		return fmt.Errorf("%w: tar.exe: %v %s", errUseEngine, err, strings.Join(lines, "; "))
	}
	if p != nil {
		p.setDone(p.total.Load())
	}
	return nil
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
