package main

import (
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

// ErrEncrypted is returned when the archive needs a password and none was
// given.
var ErrEncrypted = errors.New("the archive is password-protected")

// ErrBadPassword is returned when the password given does not open the
// archive.
var ErrBadPassword = errors.New("the password is not correct")

// ErrEmpty is returned for an archive with nothing to extract.
var ErrEmpty = errors.New("the archive is empty")

// ErrUnknownFormat is returned for a file that is not an archive ZipClick
// reads.
var ErrUnknownFormat = errors.New("not an archive ZipClick can read")

// ErrCancelled is returned when Progress.Cancel stopped an extraction.
var ErrCancelled = errors.New("cancelled")

// MethodError is returned for a zip member compressed with a method ZipClick
// cannot read (Deflate64, which Explorer uses for very large files, say).
type MethodError struct{ Method uint16 }

func (e *MethodError) Error() string {
	return fmt.Sprintf("compression method %d is not supported", e.Method)
}

// Progress is shared between an extraction and whoever watches it: the
// extraction sets the total and advances what is done, the watcher may
// Cancel. All methods are safe from any goroutine, and a nil *Progress does
// nothing. Formats that list their contents up front count the bytes written;
// streams (tar.gz, a lone .xz) count the bytes of the archive read instead.
type Progress struct {
	total, done atomic.Int64
	cancelled   atomic.Bool
}

// Bytes returns how much is done so far, of how much.
func (p *Progress) Bytes() (done, total int64) {
	return p.done.Load(), p.total.Load()
}

// Cancel asks the extraction to stop; it then returns ErrCancelled and leaves
// nothing behind.
func (p *Progress) Cancel() { p.cancelled.Store(true) }

func (p *Progress) stopped() bool { return p != nil && p.cancelled.Load() }

func (p *Progress) setTotal(n int64) {
	if p != nil {
		p.total.Store(n)
	}
}

func (p *Progress) add(n int64) {
	if p != nil {
		p.done.Add(n)
	}
}

// A source walks the members of one archive, in order. Each format has one.
type source interface {
	// next returns the next member, or io.EOF after the last. A member's
	// contents can only be read before the following call to next.
	next() (*member, error)
	// countsInput says the source advances Progress itself, by the bytes of
	// archive it reads, instead of by the bytes extracted.
	countsInput() bool
	Close() error
}

// member is one entry of an archive, as its format describes it.
type member struct {
	name     string // UTF-8, '/' or '\' separated, not yet made safe
	dir      bool
	skip     bool   // symbolic links and devices, which are never extracted
	hardlink string // tar: the earlier member this one has the contents of
	exec     bool
	mtime    time.Time // zero when unknown
	open     func() (io.ReadCloser, error)
}

// Extract unpacks archive next to itself the way macOS Archive Utility does:
// an archive holding a single top-level item yields that item beside the
// archive; anything else yields a folder named after the archive. Existing
// names are never overwritten: "name 2", "name 3"... are used instead.
//
// Everything is written into a hidden staging folder in the same directory
// first and renamed into place at the end, so a failed or interrupted
// extraction never leaves a half-written result under the final name.
//
// It returns the path of the item created. password may be empty; an
// encrypted archive then fails with ErrEncrypted. p may be nil.
func Extract(archive, password string, p *Progress) (string, error) {
	src, err := openArchive(archive, password, p)
	if err != nil {
		return "", err
	}
	defer src.Close()

	parent := filepath.Dir(archive)
	stage, err := os.MkdirTemp(parent, ".zipclick-")
	if err != nil {
		return "", err
	}
	hide(stage)
	defer os.RemoveAll(stage) // no-op once renamed away

	tops, err := writeAll(stage, src, p)
	if err != nil {
		return "", err
	}
	if len(tops) == 0 {
		return "", ErrEmpty
	}

	if len(tops) == 1 {
		dst := uniquePath(parent, tops[0])
		if err := os.Rename(filepath.Join(stage, tops[0]), dst); err != nil {
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

// decodeName returns the member name as UTF-8. Zips written by older Windows
// (and many other tools) store names in the OEM code page without saying so;
// zips from macOS store UTF-8 without the flag, decomposed (NFD).
func decodeName(name string, nonUTF8 bool, oem *charmap.Charmap) string {
	if nonUTF8 && !utf8.ValidString(name) {
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

// archiveStem is the archive's name without its extension, or both of them
// for "name.tar.gz".
func archiveStem(archive string) string {
	base := filepath.Base(archive)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.EqualFold(filepath.Ext(stem), ".tar") {
		stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	}
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

// writeAll extracts every member of src under root and returns the
// top-level names it created, in order, without macOS metadata or anything
// unsafe.
func writeAll(root string, src source, p *Progress) ([]string, error) {
	var tops []string
	seen := map[string]bool{}
	dirTimes := map[string]time.Time{}
	countOutput := !src.countsInput()
	for {
		if p.stopped() {
			return nil, ErrCancelled
		}
		m, err := src.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		rel, ok := safeRel(m.name)
		if !ok || m.skip || isMacJunk(rel) {
			continue
		}
		first, _, _ := strings.Cut(rel, "/")
		if k := strings.ToLower(first); !seen[k] {
			seen[k] = true
			tops = append(tops, first)
		}
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if m.dir || strings.HasSuffix(m.name, "/") {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return nil, err
			}
			dirTimes[dst] = m.mtime
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if m.hardlink != "" {
			if target, ok := safeRel(m.hardlink); ok {
				_ = linkOrCopy(filepath.Join(root, filepath.FromSlash(target)), dst)
			}
			continue
		}
		var pw *Progress
		if countOutput {
			pw = p
		}
		if err := writeFile(dst, m, p, pw); err != nil {
			if errors.Is(err, ErrCancelled) || errors.Is(err, ErrBadPassword) || errors.Is(err, ErrEncrypted) {
				return nil, err
			}
			return nil, fmt.Errorf("%s: %w", rel, err)
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
	return tops, nil
}

// writeFile writes one member to dst. cancel is checked on every buffer;
// count, when not nil, is advanced by the bytes written.
func writeFile(dst string, m *member, cancel, count *Progress) error {
	rc, err := m.open()
	if err != nil {
		return err
	}
	defer rc.Close()

	perm := os.FileMode(0o644)
	if m.exec {
		perm = 0o755
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.CopyBuffer(&progressWriter{out, cancel, count}, rc, make([]byte, 256<<10)); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if !m.mtime.IsZero() {
		_ = os.Chtimes(dst, m.mtime, m.mtime)
	}
	return nil
}

// linkOrCopy gives dst the contents of the already extracted src.
func linkOrCopy(src, dst string) error {
	if os.Link(src, dst) == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
	w      io.Writer
	cancel *Progress
	count  *Progress
}

func (pw *progressWriter) Write(b []byte) (int, error) {
	if pw.cancel.stopped() {
		return 0, ErrCancelled
	}
	n, err := pw.w.Write(b)
	pw.count.add(int64(n))
	return n, err
}

// progressReader advances p by what is read through it: the archive itself,
// for sources that count input.
type progressReader struct {
	r io.Reader
	p *Progress
}

func (pr *progressReader) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	pr.p.add(int64(n))
	return n, err
}
