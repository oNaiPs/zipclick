//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// What the progress and password windows share, in plain Win32: a fixed-size
// frame centred on the monitor under the cursor (where the archive was just
// double-clicked), laid out in 96-dpi units and scaled for that monitor, with
// controls in the system's message font.

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
	pGetWindowText       = user32.NewProc("GetWindowTextW")
	pGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	pSetFocus            = user32.NewProc("SetFocus")
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
	wsExClientEdg = 0x00000200
	ssNoPrefix    = 0x00000080
	ssEndEllipsis = 0x00004000
	bsDefPush     = 0x00000001
	esPassword    = 0x00000020
	esAutoHScroll = 0x00000080

	wmDestroy  = 0x0002
	wmClose    = 0x0010
	wmSetFont  = 0x0030
	wmCommand  = 0x0111
	wmTimer    = 0x0113
	pbmSetPos  = 0x0402
	pbmSetRng  = 0x0406
	idOK       = 1
	idCancel   = 2
	colorFace  = 15
	spiNCM     = 0x0029
	iccProgres = 0x00000020
	iccStd     = 0x00004000
	idcArrow   = 32512
	imageIcon  = 1
	lrShared   = 0x00008000
	swShow     = 5

	errClassAlreadyExists = 1410
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

type winMsg struct {
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

// dialog is a top-level window being filled with controls.
type dialog struct {
	hwnd uintptr
	inst uintptr
	dpi  uint32
	font uintptr
}

// newDialog registers class (once per process) with proc and creates a
// hidden w×h window, in 96-dpi units, centred under the cursor.
func newDialog(class string, proc func(hwnd, msg, wParam, lParam uintptr) uintptr, w, h int32) (*dialog, bool) {
	icc := struct{ Size, ICC uint32 }{8, iccProgres | iccStd}
	pInitCommonControls.Call(uintptr(unsafe.Pointer(&icc)))

	d := &dialog{dpi: 96}
	d.inst, _, _ = pGetModuleHandle.Call(0)
	cls := utf16(class)
	icon, _, _ := pLoadImage.Call(d.inst, uintptr(unsafe.Pointer(utf16("APP"))), imageIcon, 0, 0, lrShared)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{
		WndProc:    syscall.NewCallback(proc),
		Instance:   d.inst,
		Icon:       icon,
		Cursor:     cursor,
		Background: colorFace + 1,
		ClassName:  cls,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 && err != syscall.Errno(errClassAlreadyExists) {
		return nil, false
	}

	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	mon, _, _ := pMonitorFromPoint.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))), 2)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	pGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	var dpiY uint32
	pGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&d.dpi)), uintptr(unsafe.Pointer(&dpiY)))

	rc := rect{0, 0, d.px(w), d.px(h)}
	style := uintptr(wsCaption | wsSysMenu)
	pAdjustWindowRectDpi.Call(uintptr(unsafe.Pointer(&rc)), style, 0, wsExDlgModal, uintptr(d.dpi))
	ww, wh := rc.Right-rc.Left, rc.Bottom-rc.Top
	x := mi.Work.Left + (mi.Work.Right-mi.Work.Left-ww)/2
	y := mi.Work.Top + (mi.Work.Bottom-mi.Work.Top-wh)/2

	d.hwnd = create(wsExDlgModal, cls, appName, style, x, y, ww, wh, 0, 0, d.inst)
	if d.hwnd == 0 {
		return nil, false
	}
	ncm := nonClientMetrics{Size: uint32(unsafe.Sizeof(nonClientMetrics{}))}
	if r, _, _ := pSysParamsForDpi.Call(spiNCM, uintptr(ncm.Size), uintptr(unsafe.Pointer(&ncm)), 0, uintptr(d.dpi)); r != 0 {
		d.font, _, _ = pCreateFontIndirect.Call(uintptr(unsafe.Pointer(&ncm.MessageFont)))
	}
	return d, true
}

func (d *dialog) px(v int32) int32 { return v * int32(d.dpi) / 96 }

// child adds a control, in 96-dpi units, in the message font.
func (d *dialog) child(exStyle uintptr, cls, text string, style uintptr, x, y, cx, cy, id int32) uintptr {
	c := create(exStyle, utf16(cls), text, wsChild|wsVisible|style, d.px(x), d.px(y), d.px(cx), d.px(cy), d.hwnd, uintptr(id), d.inst)
	if d.font != 0 {
		pSendMessage.Call(c, wmSetFont, d.font, 1)
	}
	return c
}

func (d *dialog) show() {
	pShowWindow.Call(d.hwnd, swShow)
	pSetForeground.Call(d.hwnd)
}

// run pumps messages until the window posts WM_QUIT. IsDialogMessage gives
// the window Tab between controls, Enter for the default button and Esc for
// Cancel.
func (d *dialog) run() {
	var m winMsg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if ok, _, _ := pIsDialogMessage.Call(d.hwnd, uintptr(unsafe.Pointer(&m))); ok != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func create(exStyle uintptr, class *uint16, text string, style uintptr, x, y, w, h int32, parent, id, inst uintptr) uintptr {
	hwnd, _, _ := pCreateWindowEx.Call(exStyle, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(utf16(text))),
		style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, inst, 0)
	return hwnd
}

func windowText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLength.Call(hwnd)
	buf := make([]uint16, n+1)
	pGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return windows.UTF16ToString(buf)
}
