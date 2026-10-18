package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"
)

// ErrEncrypted is returned for password-protected archives, which archive/zip
// cannot read.
var ErrEncrypted = errors.New("the archive is password-protected")

// ErrEmpty is returned for an archive with nothing to extract.
var ErrEmpty = errors.New("the archive is empty")

// ErrCancelled is returned when Progress.Cancel stopped an extraction.
var ErrCancelled = errors.New("cancelled")

// Progress is shared between an extraction and whoever watches it: the
// extraction fills in Total and advances Done, the watcher may Cancel. All
// methods are safe from any goroutine, and a nil *Progress does nothing.
type Progress struct {
	total, done atomic.Int64
	cancelled   atomic.Bool
}

// Bytes returns how many uncompressed bytes are written so far, of how many.
func (p *Progress) Bytes() (done, total int64) {
	return p.done.Load(), p.total.Load()
}

// Cancel asks the extraction to stop; it then returns ErrCancelled and leaves
// nothing behind.
func (p *Progress) Cancel() { p.cancelled.Store(true) }

func (p *Progress) stopped() bool { return p != nil && p.cancelled.Load() }

func (p *Progress) add(n int64) {
	if p != nil {
		p.done.Add(n)
	}
}

// entry is one archive member, already mapped to a safe relative path.
type entry struct {
	file *zip.File
	rel  string // slash-separated, sanitized, never escapes the destination
	dir  bool
}

// Extract unpacks archive next to itself the way macOS Archive Utility does:
// an archive holding a single top-level item yields that item beside the
// archive; anything else yields a folder named after the archive. Existing
// names are never overwritten — "name 2", "name 3"... are used instead.
//
// Everything is written into a hidden staging folder in the same directory
// first and renamed into place at the end, so a failed or interrupted
// extraction never leaves a half-written result under the final name.
//
// It returns the path of the item created. p may be nil.
func Extract(archive string, p *Progress) (string, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", filepath.Base(archive), err)
	}
	defer r.Close()

	entries, err := plan(r.File, oemDecoder())
	if err != nil {
		return "", err
	}

	parent := filepath.Dir(archive)
	stage, err := os.MkdirTemp(parent, ".zipclick-")
	if err != nil {
		return "", err
	}
	hide(stage)
	defer os.RemoveAll(stage) // no-op once renamed away

	if p != nil {
		var total int64
		for _, e := range entries {
			if !e.dir {
				total += int64(e.file.UncompressedSize64)
			}
		}
		p.total.Store(total)
	}
	if err := writeAll(stage, entries, p); err != nil {
		return "", err
	}

	if top, ok := singleTop(entries); ok {
		dst := uniquePath(parent, top)
		if err := os.Rename(filepath.Join(stage, top), dst); err != nil {
			return "", err
		}
		return dst, nil
	}

	dst := uniquePath(parent, archiveStem(archive))
	if err := os.Rename(stage, dst); err != nil {
		return "", err
	}
	unhide(dst)
	return dst, nil
}

// plan decodes and sanitizes every member name, drops macOS metadata, and
// refuses archives it cannot extract faithfully.
func plan(files []*zip.File, oem *charmap.Charmap) ([]entry, error) {
	var out []entry
	for _, f := range files {
		if f.Flags&0x1 != 0 {
			return nil, ErrEncrypted
		}
		name := decodeName(f, oem)
		rel, ok := safeRel(name)
		if !ok || isMacJunk(rel) {
			continue
		}
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 {
			continue // a link pointing outside the destination is not worth the risk
		}
		out = append(out, entry{file: f, rel: rel, dir: mode.IsDir() || strings.HasSuffix(name, "/")})
	}
	if len(out) == 0 {
		return nil, ErrEmpty
	}
	return out, nil
}

// decodeName returns the member name as UTF-8. Zips written by older Windows
// (and many other tools) store names in the OEM code page without saying so;
// zips from macOS store UTF-8 without the flag, decomposed (NFD).
func decodeName(f *zip.File, oem *charmap.Charmap) string {
	name := f.Name
	if f.NonUTF8 && !utf8.ValidString(name) {
		if s, err := oem.NewDecoder().String(name); err == nil {
			name = s
		}
	}
	return norm.NFC.String(name)
}

// safeRel turns a member name into a relative path that cannot escape the
// destination and is a legal Windows path. ok is false for names that are
// nothing but traversal or root.
func safeRel(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	var parts []string
	for _, p := range strings.Split(name, "/") {
		switch p {
		case "", ".":
			continue
		case "..":
			return "", false
		}
		if len(parts) == 0 && len(p) == 2 && p[1] == ':' {
			continue // drive letter: "C:/..."
		}
		parts = append(parts, sanitizeComponent(p))
	}
	if len(parts) == 0 {
		return "", false
	}
	return path.Join(parts...), true
}

var reserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitizeComponent makes one path component legal on Windows: forbidden
// characters become "_", trailing dots and spaces are dropped, and device
// names (CON, NUL, COM1.txt...) get a leading "_".
func sanitizeComponent(p string) string {
	p = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, p)
	p = strings.TrimRight(p, ". ")
	if p == "" {
		return "_"
	}
	base := strings.ToUpper(p)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if reserved[strings.TrimRight(base, " ")] {
		p = "_" + p
	}
	return p
}

// isMacJunk reports the resource-fork and Finder files macOS adds to zips.
func isMacJunk(rel string) bool {
	first, _, _ := strings.Cut(rel, "/")
	base := path.Base(rel)
	return first == "__MACOSX" || base == ".DS_Store"
}

// singleTop returns the only top-level name when the archive has exactly one.
func singleTop(entries []entry) (string, bool) {
	var top string
	for _, e := range entries {
		first, _, _ := strings.Cut(e.rel, "/")
		if top == "" {
			top = first
		} else if !strings.EqualFold(top, first) {
			return "", false
		}
	}
	return top, true
}

func archiveStem(archive string) string {
	base := filepath.Base(archive)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = base
	}
	return sanitizeComponent(stem)
}

// uniquePath returns dir/name, or the first of "name 2", "name 3"... (before
// the extension, for files) that does not exist yet.
func uniquePath(dir, name string) string {
	p := filepath.Join(dir, name)
	if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" { // ".env" is a name, not an extension
		stem, ext = name, ""
	}
	for i := 2; ; i++ {
		p = filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, i, ext))
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
	}
}

func writeAll(root string, entries []entry, p *Progress) error {
	dirTimes := map[string]time.Time{}
	for _, e := range entries {
		if p.stopped() {
			return ErrCancelled
		}
		dst := filepath.Join(root, filepath.FromSlash(e.rel))
		if e.dir {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			dirTimes[dst] = modTime(e.file)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := writeFile(dst, e.file, p); err != nil {
			if errors.Is(err, ErrCancelled) {
				return err
			}
			return fmt.Errorf("%s: %w", e.rel, err)
		}
	}
	// Directory times last, deepest first: writing into a directory bumps it.
	dirs := make([]string, 0, len(dirTimes))
	for d := range dirTimes {
		dirs = append(dirs, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		if t := dirTimes[d]; !t.IsZero() {
			_ = os.Chtimes(d, t, t)
		}
	}
	return nil
}

func writeFile(dst string, f *zip.File, p *Progress) error {
	rc, err := f.Open()
	if err != nil {
		if errors.Is(err, zip.ErrAlgorithm) {
			return fmt.Errorf("compression method %d is not supported", f.Method)
		}
		return err
	}
	defer rc.Close()

	perm := os.FileMode(0o644)
	if f.Mode().Perm()&0o100 != 0 {
		perm = 0o755
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(&progressWriter{out, p}, rc, make([]byte, 256<<10)); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if t := modTime(f); !t.IsZero() {
		_ = os.Chtimes(dst, t, t)
	}
	return nil
}

// modTime is when the member was last modified. A zip that carries only the
// MS-DOS date fields (Explorer's do) records local wall-clock time, which
// archive/zip labels UTC; read it as local time instead.
func modTime(f *zip.File) time.Time {
	t := f.Modified
	if t.IsZero() || hasExtendedTime(f.Extra) {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
}

// hasExtendedTime reports an NTFS (0x000a) or Unix extended (0x5455)
// timestamp field, both of which are true instants.
func hasExtendedTime(extra []byte) bool {
	for len(extra) >= 4 {
		tag := uint16(extra[0]) | uint16(extra[1])<<8
		size := int(uint16(extra[2]) | uint16(extra[3])<<8)
		if tag == 0x000a || tag == 0x5455 {
			return true
		}
		if 4+size > len(extra) {
			break
		}
		extra = extra[4+size:]
	}
	return false
}

// codePages maps the OEM code pages Windows may report to their decoders.
var codePages = map[uint32]*charmap.Charmap{
	437: charmap.CodePage437,
	850: charmap.CodePage850,
	852: charmap.CodePage852,
	855: charmap.CodePage855,
	858: charmap.CodePage858,
	860: charmap.CodePage860,
	862: charmap.CodePage862,
	863: charmap.CodePage863,
	865: charmap.CodePage865,
	866: charmap.CodePage866,
}

func oemDecoder() *charmap.Charmap {
	if cm, ok := codePages[oemCodePage()]; ok {
		return cm
	}
	return charmap.CodePage437
}

// progressWriter counts what passes through it and fails once cancelled, which
// stops io.Copy within one buffer of the request.
type progressWriter struct {
	w io.Writer
	p *Progress
}

func (pw *progressWriter) Write(b []byte) (int, error) {
	if pw.p.stopped() {
		return 0, ErrCancelled
	}
	n, err := pw.w.Write(b)
	pw.p.add(int64(n))
	return n, err
}
