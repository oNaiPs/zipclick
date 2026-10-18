//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"time"
	"unsafe"
)

// The progress window: a caption, a bar, a line of detail and a Cancel
// button. It only appears when an extraction outlasts showAfter, so small
// archives never flash a window, which is what Archive Utility does too.

const showAfter = 600 * time.Millisecond

// progressUI is the one progress window this process shows; Explorer starts
// one process per archive, so one at a time is all there is.
type progressUI struct {
	*dialog
	p        *Progress
	done     <-chan struct{}
	title    string
	start    time.Time
	bar      uintptr
	detail   uintptr
	button   uintptr
	lastText string
	stopping bool
}

var ui *progressUI

// withProgress runs work, showing the progress window if it takes long
// enough. Without a window (--quiet) it just runs it.
func withProgress(archive string, p *Progress, work func() error) error {
	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		err = work()
	}()
	select {
	case <-done:
		return err
	case <-time.After(showAfter):
	}
	if quiet {
		<-done
		return err
	}
	ui = &progressUI{p: p, done: done, title: filepath.Base(archive), start: time.Now().Add(-showAfter)}
	if !ui.open() {
		<-done // no window, but the work still finishes
		return err
	}
	ui.run()
	<-done
	return err
}

func (u *progressUI) open() bool {
	const w, h, m = 420, 108, 16
	d, ok := newDialog("ZipClickProgress", progressProc, w, h)
	if !ok {
		return false
	}
	u.dialog = d
	// Caption; bar with Cancel beside it; detail across the whole width,
	// since "noch etwa 30 Sekunden" is half again as long as the English.
	const bw = 88
	d.child(0, "STATIC", fmt.Sprintf(msg.Extracting, u.title), ssNoPrefix|ssEndEllipsis, m, m, w-2*m, 20, 0)
	u.bar = d.child(0, "msctls_progress32", "", 0, m, m+29, w-3*m-bw+8, 14, 0)
	u.button = d.child(0, "BUTTON", msg.Cancel, wsTabStop|bsDefPush, w-m-bw, m+22, bw, 28, idCancel)
	u.detail = d.child(0, "STATIC", "", ssNoPrefix|ssEndEllipsis, m, m+56, w-2*m, 20, 0)
	pSendMessage.Call(u.bar, pbmSetRng, 0, 1000)

	u.tick()
	d.show()
	pSetTimer.Call(u.hwnd, 1, 100, 0)
	return true
}

func progressProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTimer:
		ui.tick()
		return 0
	case wmCommand:
		if wParam&0xffff == idCancel {
			ui.cancel()
		}
		return 0
	case wmClose:
		ui.cancel()
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

func (u *progressUI) cancel() {
	if u.stopping {
		return
	}
	u.stopping = true
	u.p.Cancel()
	pEnableWindow.Call(u.button, 0)
	u.setDetail(msg.Stopping)
}

func (u *progressUI) tick() {
	select {
	case <-u.done:
		pDestroyWindow.Call(u.hwnd)
		return
	default:
	}
	if u.stopping {
		return
	}
	done, total := u.p.Bytes()
	if total > 0 {
		pSendMessage.Call(u.bar, pbmSetPos, uintptr(done*1000/total), 0)
	}
	u.setDetail(detailText(done, total, time.Since(u.start)))
}

func (u *progressUI) setDetail(s string) {
	if s != u.lastText {
		u.lastText = s
		pSetWindowText.Call(u.detail, uintptr(unsafe.Pointer(utf16(s))))
	}
}
