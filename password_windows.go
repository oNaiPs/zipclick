//go:build windows

package main

import "fmt"

// The password window: what is locked, a field for the password, OK and
// Cancel. Asked again, after a wrong password, it says so.

type passwordUI struct {
	*dialog
	field    uintptr
	password string
	ok       bool
}

var pwUI *passwordUI

// askPassword asks for the password of archive (its file name). wrong says
// the last one tried did not open it. ok is false when the user cancelled.
func askPassword(archive string, wrong bool) (password string, ok bool) {
	const w, h, m = 380, 150, 16
	d, created := newDialog("ZipClickPassword", passwordProc, w, h)
	if !created {
		return "", false
	}
	pwUI = &passwordUI{dialog: d}
	prompt := msg.PasswordAsk
	if wrong {
		prompt = msg.PasswordWrong
	}
	const bw = 88
	d.child(0, "STATIC", fmt.Sprintf(msg.PasswordFor, archive), ssNoPrefix|ssEndEllipsis, m, m, w-2*m, 20, 0)
	d.child(0, "STATIC", prompt, ssNoPrefix, m, m+22, w-2*m, 20, 0)
	pwUI.field = d.child(wsExClientEdg, "EDIT", "", wsTabStop|esPassword|esAutoHScroll, m, m+50, w-2*m, 24, 0)
	d.child(0, "BUTTON", msg.OK, wsTabStop|bsDefPush, w-m-2*bw-8, m+90, bw, 28, idOK)
	d.child(0, "BUTTON", msg.Cancel, wsTabStop, w-m-bw, m+90, bw, 28, idCancel)

	d.show()
	pSetFocus.Call(pwUI.field)
	d.run()
	return pwUI.password, pwUI.ok
}

func passwordProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		switch wParam & 0xffff {
		case idOK:
			pwUI.password = windowText(pwUI.field)
			pwUI.ok = true
			pDestroyWindow.Call(hwnd)
		case idCancel:
			pDestroyWindow.Call(hwnd)
		}
		return 0
	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}
