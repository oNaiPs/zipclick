//go:build windows

package main

import (
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The progress window, in plain Win32: a caption, a bar, a line of detail and
// a Cancel button. It only appears when an extraction outlasts showAfter, so
// small archives never flash a window, which is what Archive Utility does too.

const showAfter = 600 * time.Millisecond

func init() {
	// Win32 windows belong to the thread that made them; keep main on one.
	runtime.LockOSThread()
}

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")
	comctl  = windows.NewLazySystemDLL("comctl32.dll")
	shcore  = windows.NewLazySystemDLL("shcore.dll")
	modkern = windows.NewLazySystemDLL("kernel32.dll")

	pRegisterClassEx     = user32.NewProc("RegisterClassExW")
	pCreateWindowEx      = user32.NewProc("CreateWindowExW")
	pDefWindowProc       = user32.NewProc("DefWindowProcW")
	pDestroyWindow       = user32.NewProc("DestroyWindow")
	pShowWindow          = user32.NewProc("ShowWindow")
	pGetMessage          = user32.NewProc("GetMessageW")
	pIsDialogMessage     = user32.NewProc("IsDialogMessageW")
	pTranslateMessage    = user32.NewProc("TranslateMessage")
	pDispatchMessage     = user32.NewProc("DispatchMessageW")
	pPostQuitMessage     = user32.NewProc("PostQuitMessage")
	pSendMessage         = user32.NewProc("SendMessageW")
	pSetWindowText       = user32.NewProc("SetWindowTextW")
	pSetTimer            = user32.NewProc("SetTimer")
	pEnableWindow        = user32.NewProc("EnableWindow")
	pGetCursorPos        = user32.NewProc("GetCursorPos")
	pMonitorFromPoint    = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	pAdjustWindowRectDpi = user32.NewProc("AdjustWindowRectExForDpi")
	pSysParamsForDpi     = user32.NewProc("SystemParametersInfoForDpi")
	pLoadImage           = user32.NewProc("LoadImageW")
	pLoadCursor          = user32.NewProc("LoadCursorW")
	pSetForeground       = user32.NewProc("SetForegroundWindow")
	pCreateFontIndirect  = gdi32.NewProc("CreateFontIndirectW")
	pInitCommonControls  = comctl.NewProc("InitCommonControlsEx")
	pGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
	pGetModuleHandle     = modkern.NewProc("GetModuleHandleW")
)

const (
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsTabStop     = 0x00010000
	wsExDlgModal  = 0x00000001
	ssNoPrefix    = 0x00000080
	ssEndEllipsis = 0x00004000
	bsDefPush     = 0x00000001

	wmDestroy  = 0x0002
	wmClose    = 0x0010
	wmSetFont  = 0x0030
	wmCommand  = 0x0111
	wmTimer    = 0x0113
	pbmSetPos  = 0x0402
	pbmSetRng  = 0x0406
	idCancel   = 2
	colorFace  = 15
	spiNCM     = 0x0029
	iccProgres = 0x00000020
	iccStd     = 0x00004000
	idcArrow   = 32512
	imageIcon  = 1
	lrShared   = 0x00008000
	swShow     = 5
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type logFont struct {
	Height, Width, Escapement, Orientation, Weight int32
	Italic, Underline, StrikeOut, CharSet          byte
	OutPrecision, ClipPrecision, Quality, Pitch    byte
	FaceName                                       [32]uint16
}

type nonClientMetrics struct {
	Size                                                             uint32
	BorderWidth, ScrollWidth, ScrollHeight, CaptionWidth, CaptionHgt int32
	CaptionFont                                                      logFont
	SmCaptionWidth, SmCaptionHeight                                  int32
	SmCaptionFont                                                    logFont
	MenuWidth, MenuHeight                                            int32
	MenuFont, StatusFont, MessageFont                                logFont
	PaddedBorderWidth                                                int32
}

// progressUI is the one window this process shows; Explorer starts one
// process per archive, so one at a time is all there is.
type progressUI struct {
	p        *Progress
	done     <-chan struct{}
	title    string
	start    time.Time
	hwnd     uintptr
	caption  uintptr
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
	var m msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if ok, _, _ := pIsDialogMessage.Call(ui.hwnd, uintptr(unsafe.Pointer(&m))); ok != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	<-done
	return err
}

func (u *progressUI) open() bool {
	icc := struct{ Size, ICC uint32 }{8, iccProgres | iccStd}
	pInitCommonControls.Call(uintptr(unsafe.Pointer(&icc)))

	inst, _, _ := pGetModuleHandle.Call(0)
	class := utf16("ZipClickProgress")
	icon, _, _ := pLoadImage.Call(inst, uintptr(unsafe.Pointer(utf16("APP"))), imageIcon, 0, 0, lrShared)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   inst,
		Icon:       icon,
		Cursor:     cursor,
		Background: colorFace + 1,
		ClassName:  class,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, _ := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return false
	}

	// Lay out in 96-dpi units, scaled for the monitor under the cursor (where
	// the archive was just double-clicked).
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 2)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	var dpiX, dpiY uint32 = 96, 96
	pGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	px := func(v int32) int32 { return v * int32(dpiX) / 96 }

	const w, h, m = 420, 112, 16
	rc := rect{0, 0, px(w), px(h)}
	style := uintptr(wsCaption | wsSysMenu)
	pAdjustWindowRectDpi.Call(uintptr(unsafe.Pointer(&rc)), style, 0, wsExDlgModal, uintptr(dpiX))
	ww, wh := rc.Right-rc.Left, rc.Bottom-rc.Top
	x := mi.Work.Left + (mi.Work.Right-mi.Work.Left-ww)/2
	y := mi.Work.Top + (mi.Work.Bottom-mi.Work.Top-wh)/2

	u.hwnd = create(wsExDlgModal, class, appName, style, x, y, ww, wh, 0, 0, inst)
	if u.hwnd == 0 {
		return false
	}
	child := func(cls, text string, st uintptr, x, y, cx, cy, id int32) uintptr {
		return create(0, utf16(cls), text, wsChild|wsVisible|st, px(x), px(y), px(cx), px(cy), u.hwnd, uintptr(id), inst)
	}
	u.caption = child("STATIC", "Extracting “"+u.title+"”", ssNoPrefix|ssEndEllipsis, m, m, w-2*m, 20, 0)
	u.bar = child("msctls_progress32", "", 0, m, m+28, w-2*m, 14, 0)
	u.detail = child("STATIC", "", ssNoPrefix|ssEndEllipsis, m, m+54, w-3*m-88, 20, 0)
	u.button = child("BUTTON", "Cancel", wsTabStop|bsDefPush, w-m-88, m+50, 88, 28, idCancel)
	pSendMessage.Call(u.bar, pbmSetRng, 0, 1000)

	ncm := nonClientMetrics{Size: uint32(unsafe.Sizeof(nonClientMetrics{}))}
	if r, _, _ := pSysParamsForDpi.Call(spiNCM, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0, uintptr(dpiX)); r != 0 {
		if font, _, _ := pCreateFontIndirect.Call(uintptr(unsafe.Pointer(&ncm.MessageFont))); font != 0 {
			for _, c := range []uintptr{u.caption, u.detail, u.button} {
				pSendMessage.Call(c, wmSetFont, font, 1)
			}
		}
	}

	u.tick()
	pShowWindow.Call(u.hwnd, swShow)
	pSetForeground.Call(u.hwnd)
	pSetTimer.Call(u.hwnd, 1, 100, 0)
	return true
}

func create(exStyle uintptr, class *uint16, text string, style uintptr, x, y, w, h int32, parent, id, inst uintptr) uintptr {
	hwnd, _, _ := pCreateWindowEx.Call(exStyle, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(utf16(text))),
		style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, inst, 0)
	return hwnd
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
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
	u.setDetail("Stopping…")
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
