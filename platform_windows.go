//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	exeName     = "zipclick.exe"
	settingsURI = "ms-settings:defaultapps?registeredAppUser=" + appName

	classes       = `Software\Classes`
	capabilities  = `Software\` + appName + `\Capabilities`
	uninstallKey  = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + appName
	applicationKy = classes + `\Applications\` + exeName
	fileExts      = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`
)

// fileTypes are the extensions ZipClick registers for, each with the format
// name Explorer's Type column shows ("RAR archive"). A .tar.gz is a .gz to
// Windows, which only looks at the last extension.
var fileTypes = []struct{ ext, kind string }{
	{".zip", "ZIP"},
	{".7z", "7-Zip"},
	{".rar", "RAR"},
	{".tar", "TAR"},
	{".tgz", "TAR"},
	{".gz", "GZIP"},
	{".tbz2", "TAR"},
	{".bz2", "BZIP2"},
	{".txz", "TAR"},
	{".xz", "XZ"},
	{".tzst", "TAR"},
	{".zst", "Zstandard"},
}

// progID is what a double-click on ext resolves to once ZipClick is the
// choice for it: ZipClick.zip, ZipClick.7z...
func progID(ext string) string { return appName + ext }

func contextVerb(ext string) string {
	return classes + `\SystemFileAssociations\` + ext + `\shell\` + appName
}

// version is set at build time: -ldflags "-X main.version=..."
var version = "dev"

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	procGetOEMCP     = kernel32.NewProc("GetOEMCP")
	procSHChangeNoti = shell32.NewProc("SHChangeNotify")
	procILCreate     = shell32.NewProc("ILCreateFromPathW")
	procILFree       = shell32.NewProc("ILFree")
	procSHOpenSelect = shell32.NewProc("SHOpenFolderAndSelectItems")
)

const (
	idYes = 6 // MessageBox return value

	shcneCreate       = 0x00000002
	shcneMkdir        = 0x00000008
	shcneAssocChanged = 0x08000000
	shcnfIDList       = 0x0000
	shcnfPathW        = 0x0005
	shcnfFlush        = 0x1000
)

func oemCodePage() uint32 {
	cp, _, _ := procGetOEMCP.Call()
	return uint32(cp)
}

func hide(p string) {
	if u, err := windows.UTF16PtrFromString(p); err == nil {
		_ = windows.SetFileAttributes(u, windows.FILE_ATTRIBUTE_HIDDEN)
	}
}

func unhide(p string) {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return
	}
	if attrs, err := windows.GetFileAttributes(u); err == nil {
		_ = windows.SetFileAttributes(u, attrs&^windows.FILE_ATTRIBUTE_HIDDEN)
	}
}

// notifyCreated tells Explorer about the new item so an open window shows it
// at once instead of on its next poll.
func notifyCreated(p string) {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return
	}
	event := uintptr(shcneCreate)
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		event = shcneMkdir
	}
	procSHChangeNoti.Call(event, shcnfPathW|shcnfFlush, uintptr(unsafe.Pointer(u)), 0)
}

// reveal selects the extracted item in the Explorer window showing its
// folder, as Finder does — reusing that window rather than opening another.
// Not for the desktop: there is no folder window to select in, and asking
// would open one.
func reveal(p string) {
	if desk, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0); err == nil && samePath(filepath.Dir(p), desk) {
		return
	}
	// S_FALSE (already initialised) still needs its CoUninitialize.
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil || err == syscall.Errno(windows.S_FALSE) {
		defer windows.CoUninitialize()
	}
	pidl, _, _ := procILCreate.Call(uintptr(unsafe.Pointer(utf16(p))))
	if pidl == 0 {
		return
	}
	defer procILFree.Call(pidl)
	procSHOpenSelect.Call(pidl, 0, 0, 0)
}

func messageBox(title, text string, kind int) bool {
	flags := map[int]uint32{
		boxInfo:     windows.MB_OK | windows.MB_ICONINFORMATION,
		boxWarning:  windows.MB_OK | windows.MB_ICONWARNING,
		boxQuestion: windows.MB_YESNO | windows.MB_ICONQUESTION,
	}[kind]
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	ret, _ := windows.MessageBox(0, m, t, flags|windows.MB_SETFOREGROUND)
	return ret == idYes
}

// preferredLanguages is the user's Windows display languages, in order
// ("pt-PT", "en-US"...).
func preferredLanguages() []string {
	l, _ := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	return l
}

func installDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", appName)
}

// install copies the running executable to the per-user programs folder and
// registers it as a handler for every archive type. Windows does not let a
// program make itself the default (the choice is hash-protected), so the last
// step opens the Default apps page for the user to pick ZipClick there.
func install() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := installDir()
	exe := filepath.Join(dir, exeName)
	if !samePath(self, exe) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := copyFile(self, exe); err != nil {
			return fmt.Errorf("cannot copy to %s: %w", dir, err)
		}
	}
	if err := register(exe); err != nil {
		return fmt.Errorf("cannot register .zip handler: %w", err)
	}
	procSHChangeNoti.Call(shcneAssocChanged, shcnfIDList, 0, 0)

	if currentProgID(".zip") == progID(".zip") {
		info(appName, msg.Installed)
		return nil
	}
	if quiet {
		fmt.Println("registered; choose ZipClick for each type in " + settingsURI)
		return nil
	}
	messageBox(appName, msg.AlmostDone, boxInfo)
	return windows.ShellExecute(0, utf16("open"), utf16(settingsURI), nil, nil, windows.SW_SHOWNORMAL)
}

func register(exe string) error {
	cmd := fmt.Sprintf(`"%s" "%%1"`, exe)
	icon := zipIcon()
	assoc := map[string]string{}
	supported := map[string]string{}

	for _, t := range fileTypes {
		id := classes + `\` + progID(t.ext)
		name := fmt.Sprintf(msg.TypeName, t.kind)
		// The ProgID double-clicks resolve to once it is the user's choice.
		if err := setValues(id, map[string]string{"": name, "FriendlyTypeName": name}); err != nil {
			return err
		}
		if err := setExpand(id+`\DefaultIcon`, icon); err != nil {
			return err
		}
		if err := setValues(id+`\shell`, map[string]string{"": "open"}); err != nil {
			return err
		}
		if err := setValues(id+`\shell\open`, map[string]string{"": msg.VerbExtract}); err != nil {
			return err
		}
		if err := setValues(id+`\shell\open\command`, map[string]string{"": cmd}); err != nil {
			return err
		}
		// Offered in "Open with" and in Default apps.
		if err := setValues(classes+`\`+t.ext+`\OpenWithProgids`, map[string]string{progID(t.ext): ""}); err != nil {
			return err
		}
		// A context-menu verb on every file of the type, whatever the default is.
		if err := setValues(contextVerb(t.ext), map[string]string{"": msg.VerbExtractHere, "Icon": exe}); err != nil {
			return err
		}
		if err := setValues(contextVerb(t.ext)+`\command`, map[string]string{"": cmd}); err != nil {
			return err
		}
		assoc[t.ext] = progID(t.ext)
		supported[t.ext] = ""
	}

	if err := setValues(applicationKy, map[string]string{"FriendlyAppName": appName}); err != nil {
		return err
	}
	if err := setValues(applicationKy+`\SupportedTypes`, supported); err != nil {
		return err
	}
	if err := setValues(applicationKy+`\shell\open\command`, map[string]string{"": cmd}); err != nil {
		return err
	}
	if err := setValues(capabilities, map[string]string{
		"ApplicationName":        appName,
		"ApplicationDescription": msg.AppDescription,
	}); err != nil {
		return err
	}
	if err := setValues(capabilities+`\FileAssociations`, assoc); err != nil {
		return err
	}
	if err := setValues(`Software\RegisteredApplications`, map[string]string{appName: capabilities}); err != nil {
		return err
	}

	// Installed apps, so it can be removed the usual way.
	if err := setValues(uninstallKey, map[string]string{
		"DisplayName":     appName,
		"DisplayVersion":  version,
		"Publisher":       appName,
		"DisplayIcon":     exe,
		"InstallLocation": filepath.Dir(exe),
		"UninstallString": fmt.Sprintf(`"%s" uninstall`, exe),
	}); err != nil {
		return err
	}
	return setDWORDs(uninstallKey, map[string]uint32{"NoModify": 1, "NoRepair": 1})
}

func uninstall() error {
	var errs []error
	keys := []string{applicationKy, `Software\` + appName, uninstallKey}
	for _, t := range fileTypes {
		keys = append(keys, classes+`\`+progID(t.ext), contextVerb(t.ext))
		deleteValue(classes+`\`+t.ext+`\OpenWithProgids`, progID(t.ext))
	}
	for _, k := range keys {
		if err := deleteTree(registry.CURRENT_USER, k); err != nil {
			errs = append(errs, err)
		}
	}
	deleteValue(`Software\RegisteredApplications`, appName)
	procSHChangeNoti.Call(shcneAssocChanged, shcnfIDList, 0, 0)

	// The running copy cannot delete itself: let a detached shell do it once
	// this process has exited.
	if self, err := os.Executable(); err == nil && samePath(filepath.Dir(self), installDir()) {
		// CmdLine verbatim: exec.Command would escape the inner quotes as \",
		// which cmd.exe does not understand. Leave our job object if allowed,
		// so the cleanup outlives a parent that kills its job on exit.
		cmdline := fmt.Sprintf(`cmd.exe /c ping -n 3 127.0.0.1 >nul & rmdir /s /q "%s"`, installDir())
		for _, flags := range []uint32{windows.CREATE_BREAKAWAY_FROM_JOB, 0} {
			c := exec.Command("cmd.exe")
			c.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdline, HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | flags}
			if c.Start() == nil {
				break
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	info(appName, msg.Removed)
	return nil
}

func status() error {
	exe := filepath.Join(installDir(), exeName)
	_, err := os.Stat(exe)
	fmt.Printf("installed:  %v (%s)\n", err == nil, exe)
	k, err := registry.OpenKey(registry.CURRENT_USER, classes+`\`+progID(".zip")+`\shell\open\command`, registry.QUERY_VALUE)
	if err == nil {
		v, _, _ := k.GetStringValue("")
		k.Close()
		fmt.Printf("registered: %s\n", v)
	} else {
		fmt.Println("registered: no")
	}
	fmt.Println("double-clicks:")
	for _, t := range fileTypes {
		cur := currentProgID(t.ext)
		switch {
		case cur == progID(t.ext):
			cur = "ZipClick"
		case cur == "":
			cur = "Windows default"
		}
		fmt.Printf("  %-6s %s\n", t.ext, cur)
	}
	return nil
}

// currentProgID is what a double-click on ext resolves to for this user.
// Windows 11 25H2 keeps the choice in UserChoiceLatest\ProgId and leaves the
// older UserChoice key behind with a stale value, so the newer one wins.
func currentProgID(ext string) string {
	for _, sub := range []string{`UserChoiceLatest\ProgId`, "UserChoice"} {
		k, err := registry.OpenKey(registry.CURRENT_USER, fileExts+ext+`\`+sub, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("ProgId")
		k.Close()
		if err == nil && v != "" {
			return v
		}
	}
	return ""
}

// zipIcon reuses the icon Windows shows for zip files.
func zipIcon() string {
	if k, err := registry.OpenKey(registry.CLASSES_ROOT, `CompressedFolder\DefaultIcon`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue(""); err == nil && v != "" {
			return v
		}
	}
	return `%SystemRoot%\System32\zipfldr.dll`
}

func setValues(path string, vals map[string]string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for n, v := range vals {
		if err := k.SetStringValue(n, v); err != nil {
			return err
		}
	}
	return nil
}

func setExpand(path, v string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetExpandStringValue("", v)
}

func setDWORDs(path string, vals map[string]uint32) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	for n, v := range vals {
		if err := k.SetDWordValue(n, v); err != nil {
			return err
		}
	}
	return nil
}

func deleteValue(path, name string) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue(name)
		k.Close()
	}
}

// deleteTree removes a key and everything under it; a missing key is fine.
func deleteTree(root registry.Key, path string) error {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	subs, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := deleteTree(root, path+`\`+s); err != nil {
			return err
		}
	}
	return registry.DeleteKey(root, path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func utf16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}
