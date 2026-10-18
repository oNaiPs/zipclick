package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
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

// ErrDiskFull is returned when extracting would leave less than diskReserve
// free on the destination's drive.
var ErrDiskFull = errors.New("not enough free disk space")

// errTooLong is returned for a member that holds more data than its header
// says, which no honest archive does.
var errTooLong = errors.New("a file in the archive is larger than it declares")

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

// Total is the total set so far; 0 for a nil Progress.
func (p *Progress) Total() int64 {
	if p == nil {
		return 0
	}
	return p.total.Load()
}

func (p *Progress) setDone(n int64) {
	if p != nil {
		p.done.Store(n)
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
	size     int64     // what the header declares; -1 when unknown
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
func Extract(archive, password string, p *Progress) (dst string, err error) {
	// The format libraries read attacker-controlled headers and some can
	// still panic on a crafted one; that is a damaged archive, not a crash.
	defer func() {
		if r := recover(); r != nil {
			dst, err = "", fmt.Errorf("%w: %v", errDamaged, r)
		}
	}()
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
	defer removeStage(stage) // no-op once renamed away

	tops, err := extractInto(archive, stage, src, p)
	if err != nil {
		return "", err
	}
	if len(tops) == 0 {
		return "", ErrEmpty
	}

	if len(tops) == 1 {
		return moveUnique(filepath.Join(stage, tops[0]), parent, tops[0])
	}
	if dst, err = moveUnique(stage, parent, archiveStem(archive)); err == nil {
		unhide(dst)
	}
	return dst, err
}

// extractInto extracts src into stage, through tar.exe when the backend and
// the archive allow it, else (or if tar.exe fails) with ZipClick's engine.
func extractInto(archive, stage string, src source, p *Progress) ([]string, error) {
	if plan, ok := planFor(src); ok {
		failed, err := systemExtract(archive, stage, plan, p)
		if errors.Is(err, ErrCancelled) || errors.Is(err, ErrDiskFull) {
			return nil, err
		}
		if errors.Is(err, errUseEngine) {
			// Nothing tar.exe wrote can be trusted: start from nothing.
			if err := emptyDir(stage); err != nil {
				return nil, err
			}
			p.setDone(0)
			return writeAll(stage, src, p)
		}
		// A member tar.exe failed on may have left a file behind (an
		// encrypted 7z leaves garbage of the right size): it goes, and the
		// engine writes it, or reports why it cannot.
		for _, name := range failed {
			if rel, ok := verbatimRel(name); ok {
				if err := removeVerbatim(filepath.Join(stage, filepath.FromSlash(rel))); err != nil {
					return nil, err
				}
			}
		}
		// Whatever tar.exe wrote that ZipClick would not have (links,
		// macOS metadata, names Windows itself refuses) goes. If that
		// left gaps, or tar.exe stopped short (it cannot make links on
		// Windows, and fails over them), the engine fills in exactly what
		// is missing, by its own rules; src has not been read yet.
		removed, terr := tidyTree(stage)
		if terr != nil {
			return nil, terr
		}
		if err == nil && !removed {
			systemExtracts.Add(1)
			return topNames(stage)
		}
		p.setDone(0)
		if _, err := writeAllMissing(stage, src, p); err != nil {
			return nil, err
		}
		return topNames(stage)
	}
	return writeAll(stage, src, p)
}

// errDamaged wraps what a format library panicked with.
var errDamaged = errors.New("the archive is damaged")

// moveUnique renames from to dir/name, or "name 2", "name 3"... when that is
// taken, never replacing anything: if a name is claimed between choosing it
// and the rename, the next one is tried.
func moveUnique(from, dir, name string) (string, error) {
	for range 100 {
		dst := uniquePath(dir, name)
		err := renameNoReplace(from, dst)
		if err == nil {
			return dst, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free name for %s in %s", name, dir)
}

// removeStage deletes the staging folder. A virus scanner can hold a file
// just written open for a moment, so a failure is retried before giving up.
func removeStage(stage string) {
	for i := 0; i < 10; i++ {
		if err := os.RemoveAll(stage); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// diskReserve is how much free space extracting always leaves on the drive,
// so a decompression bomb (a few KB that claims, or produces, gigabytes)
// stops with an error instead of filling the disk.
const diskReserve = 1 << 30

// checkInterval is how many bytes are written between free-space checks.
const checkInterval = 64 << 20

// diskFree reports the free bytes on the drive holding dir; ok is false when
// it cannot tell, and then nothing is refused. A variable for the tests.
var diskFree = freeSpace

// roomFor fails with ErrDiskFull when writing n more bytes under dir would
// leave less than diskReserve free.
func roomFor(dir string, n int64) error {
	free, ok := diskFree(dir)
	if ok && (free < diskReserve || uint64(n) > free-diskReserve) {
		return ErrDiskFull
	}
	return nil
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
	// Windows also treats the superscript digits as device numbers.
	"COM¹": true, "COM²": true, "COM³": true, "LPT¹": true, "LPT²": true, "LPT³": true,
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
// unsafe. Streams (tar, rar, solid 7z) are written member by member as they
// are read; a zip, whose members can be read in any order, is planned first
// and its files written several at a time.
func writeAll(root string, src source, p *Progress) ([]string, error) {
	return writeMembers(root, src, p, false)
}

// writeAllMissing is writeAll that leaves alone every file already there:
// it fills in what tar.exe could not write.
func writeAllMissing(root string, src source, p *Progress) ([]string, error) {
	return writeMembers(root, src, p, true)
}

func writeMembers(root string, src source, p *Progress, missingOnly bool) ([]string, error) {
	w := &writer{root: root, p: p, missingOnly: missingOnly, seen: map[string]bool{}, dirTimes: map[string]time.Time{}}
	if !src.countsInput() {
		w.count = p
	}
	ps, parallel := src.(parallelSource)
	parallel = parallel && ps.parallel()
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
		j, err := w.place(m)
		if err != nil {
			return nil, err
		}
		if j == nil {
			continue
		}
		if parallel {
			w.jobs = append(w.jobs, j)
		} else if err := w.write(j, p.stopped); err != nil {
			return nil, err
		}
	}
	if err := w.writeJobs(); err != nil {
		return nil, err
	}
	w.setDirTimes()
	return w.tops, nil
}

// parallelSource is a source whose members can be opened and read in any
// order and at the same time: a zip.
type parallelSource interface {
	parallel() bool
}

// maxWriters is how many files of a zip are extracted at once.
var maxWriters = min(runtime.GOMAXPROCS(0), 8)

type writer struct {
	root     string
	p        *Progress // cancels; may be nil
	count    *Progress // advanced by bytes written, unless the source counts input
	tops     []string
	seen     map[string]bool
	dirTimes map[string]time.Time
	jobs     []*job
	inflight atomic.Int64 // declared bytes of the files being written

	missingOnly bool // skip files that already exist (filling in after tar.exe)
}

// job is one file to write.
type job struct {
	m   *member
	rel string
	dst string
}

// place makes the folders a member needs and returns the file to write for
// it, or nil when there is none (a folder, a link, something skipped).
func (w *writer) place(m *member) (*job, error) {
	rel, ok := safeRel(m.name)
	if !ok || m.skip || isMacJunk(rel) {
		return nil, nil
	}
	first, _, _ := strings.Cut(rel, "/")
	if k := strings.ToLower(first); !w.seen[k] {
		w.seen[k] = true
		w.tops = append(w.tops, first)
	}
	dst := filepath.Join(w.root, filepath.FromSlash(rel))
	if m.dir || strings.HasSuffix(m.name, "/") {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return nil, err
		}
		w.dirTimes[dst] = m.mtime
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if w.missingOnly {
		if _, err := os.Lstat(dst); err == nil {
			return nil, nil
		}
	}
	if m.hardlink != "" {
		if target, ok := safeRel(m.hardlink); ok {
			_ = linkOrCopy(filepath.Join(w.root, filepath.FromSlash(target)), dst)
		}
		return nil, nil
	}
	return &job{m: m, rel: rel, dst: dst}, nil
}

// safeWrite is write for a worker goroutine: a decoder panic there is out
// of reach of Extract's recover, so it is turned into an error here.
func (w *writer) safeWrite(j *job, stopped func() bool) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("%w: %v", errDamaged, v)
		}
	}()
	return w.write(j, stopped)
}

func (w *writer) write(j *job, stopped func() bool) error {
	if j.m.size > 0 {
		// Files being written at the same time all count against the
		// reserve, not just this one.
		reserved := w.inflight.Add(j.m.size)
		defer w.inflight.Add(-j.m.size)
		if err := roomFor(w.root, reserved); err != nil {
			return err
		}
	}
	if err := writeFile(j.dst, j.m, stopped, w.count); err != nil {
		if errors.Is(err, ErrCancelled) || errors.Is(err, ErrBadPassword) || errors.Is(err, ErrEncrypted) || errors.Is(err, ErrDiskFull) {
			return err
		}
		return fmt.Errorf("%s: %w", j.rel, err)
	}
	return nil
}

// writeJobs writes the planned files of a zip, maxWriters at a time. The
// first error stops the others. Two members with the same name (a later one
// replaces an earlier one) are written one after another instead, in order.
func (w *writer) writeJobs() error {
	if len(w.jobs) == 0 {
		return nil
	}
	names := map[string]bool{}
	fold := cases.Fold()
	for _, j := range w.jobs {
		// Full case folding, as NTFS and APFS compare names: σ and ς are
		// the same file there, though their lower cases differ.
		k := fold.String(j.dst)
		if names[k] {
			for _, j := range w.jobs {
				if err := w.write(j, w.p.stopped); err != nil {
					return err
				}
			}
			return nil
		}
		names[k] = true
	}

	var (
		failed   atomic.Bool
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	stopped := func() bool { return failed.Load() || w.p.stopped() }
	next := make(chan *job)
	for range min(maxWriters, len(w.jobs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range next {
				if stopped() {
					continue
				}
				if err := w.safeWrite(j, stopped); err != nil {
					mu.Lock()
					if firstErr == nil && !(failed.Load() && errors.Is(err, ErrCancelled)) {
						firstErr = err
					}
					mu.Unlock()
					failed.Store(true)
				}
			}
		}()
	}
	for _, j := range w.jobs {
		next <- j
	}
	close(next)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if w.p.stopped() {
		return ErrCancelled
	}
	return nil
}

// setDirTimes sets folder times last, deepest first: writing into a folder
// bumps its time.
func (w *writer) setDirTimes() {
	dirs := make([]string, 0, len(w.dirTimes))
	for d := range w.dirTimes {
		dirs = append(dirs, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		if t := w.dirTimes[d]; !t.IsZero() {
			_ = os.Chtimes(d, t, t)
		}
	}
}

// writeFile writes one member to dst. stopped, when not nil, is checked on
// every buffer; count, when not nil, is advanced by the bytes written.
func writeFile(dst string, m *member, stopped func() bool, count *Progress) error {
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
	// Closed on every way out, a decoder panic included: Windows will not
	// delete the staging folder while a file in it is open.
	closed := false
	defer func() {
		if !closed {
			out.Close()
		}
	}()
	var r io.Reader = rc
	if m.size >= 0 {
		r = io.LimitReader(rc, m.size+1)
	}
	w := &progressWriter{w: out, stopped: stopped, count: count, dir: filepath.Dir(dst)}
	var n int64
	if m.size >= 0 && m.size <= copyChunk {
		// Small enough to decode in one go: a second goroutine would only
		// add a hand-off.
		buf := chunks.Get().(*[]byte)
		n, err = io.CopyBuffer(w, r, *buf)
		chunks.Put(buf)
	} else {
		n, err = copyOverlapped(w, r)
	}
	if err == nil && m.size >= 0 && n > m.size {
		err = errTooLong
	}
	if err != nil {
		return err
	}
	closed = true
	if err := out.Close(); err != nil {
		return err
	}
	if !m.mtime.IsZero() {
		_ = os.Chtimes(dst, m.mtime, m.mtime)
	}
	return nil
}

// copyChunk is how much copyOverlapped decodes ahead of the disk at a time.
const copyChunk = 1 << 20

// chunks recycles copy buffers: allocating fresh ones for each of thousands
// of files cost more than the copying, for fast formats like zstd.
var chunks = sync.Pool{New: func() any {
	b := make([]byte, copyChunk)
	return &b
}}

// copyOverlapped copies r to w like io.Copy, but reads (that is, decodes)
// in its own goroutine, so the next chunk is decompressed while the disk
// writes this one. It returns once both sides have stopped; r is never read
// after it returns.
func copyOverlapped(w io.Writer, r io.Reader) (int64, error) {
	type chunk struct {
		b []byte
		n int
	}
	full := make(chan chunk, 2)
	empty := make(chan []byte, 3)
	var bufs []*[]byte
	for range 3 {
		b := chunks.Get().(*[]byte)
		bufs = append(bufs, b)
		empty <- *b
	}
	// Returned to the pool only once both goroutines are done with them.
	defer func() {
		for _, b := range bufs {
			chunks.Put(b)
		}
	}()
	stop := make(chan struct{})
	var rerr error
	go func() {
		defer close(full)
		// A decoder panicking here is out of reach of Extract's recover,
		// and would take the process (and the staging folder) with it.
		defer func() {
			if v := recover(); v != nil {
				rerr = fmt.Errorf("%w: %v", errDamaged, v)
			}
		}()
		for {
			var b []byte
			select {
			case b = <-empty:
			case <-stop:
				return
			}
			n, err := fill(r, b)
			if n > 0 {
				full <- chunk{b, n}
			}
			if err != nil {
				if err != io.EOF {
					rerr = err
				}
				return
			}
		}
	}()
	var written int64
	var werr error
	for c := range full {
		if werr == nil {
			n, err := w.Write(c.b[:c.n])
			written += int64(n)
			if err != nil {
				werr = err
				close(stop)
			}
		}
		empty <- c.b
	}
	if werr != nil {
		return written, werr
	}
	return written, rerr
}

// fill reads into b until it is full, the data ends (io.EOF) or r fails.
// Unlike io.ReadFull it keeps a decoder's own io.ErrUnexpectedEOF, which
// means a truncated archive, as an error.
func fill(r io.Reader, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := r.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
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
	w       io.Writer
	stopped func() bool
	count   *Progress
	dir     string // checked for free space every checkInterval bytes
	since   int64
}

func (pw *progressWriter) Write(b []byte) (int, error) {
	if pw.stopped != nil && pw.stopped() {
		return 0, ErrCancelled
	}
	if pw.since += int64(len(b)); pw.since >= checkInterval {
		pw.since = 0
		if err := roomFor(pw.dir, 0); err != nil {
			return 0, err
		}
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
