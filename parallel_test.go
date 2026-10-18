package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zipOf writes a zip of name -> body, deflated, in the order given.
func zipOf(t *testing.T, dir, name string, files [][2]string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, nb := range files {
		fw, err := w.Create(nb[0])
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(nb[1]))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func TestManyZipMembersAreWrittenInParallelAndIntact(t *testing.T) {
	dir := t.TempDir()
	var files [][2]string
	var total int64
	for i := range 300 {
		body := strings.Repeat(fmt.Sprintf("file %d ", i), 1000+i*37)
		files = append(files, [2]string{fmt.Sprintf("pkg/d%d/f%d.txt", i%7, i), body})
		total += int64(len(body))
	}
	archive := zipOf(t, dir, "many.zip", files)
	p := &Progress{}
	dst, err := Extract(archive, "", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(dst), filepath.FromSlash(f[0])))
		if err != nil || string(b) != f[1] {
			t.Fatalf("%s: %d bytes, %v", f[0], len(b), err)
		}
	}
	if done, all := p.Bytes(); done != total || all != total {
		t.Errorf("progress %d/%d, want %d", done, all, total)
	}
}

func TestDuplicateZipNamesStillLetTheLastOneWin(t *testing.T) {
	dir := t.TempDir()
	archive := zipOf(t, dir, "dup.zip", [][2]string{{"a.txt", "first"}, {"b.txt", "b"}, {"A.TXT", "second"}})
	dst, err := Extract(archive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	ents, _ := os.ReadDir(dst)
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(dst, e.Name()))
		got[e.Name()] = string(b)
	}
	switch len(got) {
	case 2: // case-insensitive (Windows, macOS): one file, the later body
		if got["b.txt"] != "b" || (got["a.txt"] != "second" && got["A.TXT"] != "second") {
			t.Fatalf("contents %v", got)
		}
	case 3: // case-sensitive (Linux): two files, each its own body
		if got["a.txt"] != "first" || got["A.TXT"] != "second" || got["b.txt"] != "b" {
			t.Fatalf("contents %v", got)
		}
	default:
		t.Fatalf("contents %v", got)
	}
}

func TestADamagedMemberStopsTheOthersAndLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	var files [][2]string
	for i := range 40 {
		files = append(files, [2]string{fmt.Sprintf("f%d.txt", i), strings.Repeat("abc", 20000)})
	}
	archive := zipOf(t, dir, "bad.zip", files)
	// Flip a byte in the compressed data of a member in the middle.
	b, _ := os.ReadFile(archive)
	i := bytes.Index(b, []byte("f20.txt")) + len("f20.txt") + 40
	b[i] ^= 0xff
	os.WriteFile(archive, b, 0o644)
	if _, err := Extract(archive, "", nil); err == nil {
		t.Fatal("a damaged member went unnoticed")
	}
	eq(t, tree(t, dir), []string{"bad.zip"})
}

func TestCancellingAParallelZipLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	var files [][2]string
	for i := range 20 {
		files = append(files, [2]string{fmt.Sprintf("f%d.bin", i), strings.Repeat("x", 3<<20)})
	}
	archive := zipOf(t, dir, "big.zip", files)
	p := &Progress{}
	p.Cancel()
	if _, err := Extract(archive, "", p); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	eq(t, tree(t, dir), []string{"big.zip"})
}

type failingReader struct {
	data []byte
	err  error
}

func (f *failingReader) Read(b []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, f.err
	}
	n := copy(b, f.data)
	f.data = f.data[n:]
	return n, nil
}

type failingWriter struct{ after int }

func (f *failingWriter) Write(b []byte) (int, error) {
	if f.after -= len(b); f.after < 0 {
		return 0, errors.New("disk on fire")
	}
	return len(b), nil
}

func TestOverlappedCopyKeepsErrors(t *testing.T) {
	// A decoder that runs out mid-stream: the error survives, it is not EOF.
	r := &failingReader{data: make([]byte, 3*copyChunk+5), err: io.ErrUnexpectedEOF}
	var out bytes.Buffer
	n, err := copyOverlapped(&out, r)
	if !errors.Is(err, io.ErrUnexpectedEOF) || n != 3*copyChunk+5 {
		t.Fatalf("n=%d err=%v, want everything read and ErrUnexpectedEOF", n, err)
	}
	// A clean end is not an error.
	if n, err := copyOverlapped(io.Discard, bytes.NewReader(make([]byte, 5*copyChunk+1))); err != nil || n != 5*copyChunk+1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	// A failing write stops the copy and is what is reported.
	r = &failingReader{data: make([]byte, 10*copyChunk), err: io.EOF}
	if _, err := copyOverlapped(&failingWriter{after: copyChunk}, r); err == nil || err.Error() != "disk on fire" {
		t.Fatalf("err = %v", err)
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("decoder bug") }

func TestADecoderPanicInTheCopyGoroutineIsAnError(t *testing.T) {
	dir := t.TempDir()
	m := &member{name: "x", size: 2 * copyChunk, open: func() (io.ReadCloser, error) {
		return io.NopCloser(panicReader{}), nil
	}}
	if err := writeFile(filepath.Join(dir, "x"), m, nil, nil, nil); !errors.Is(err, errDamaged) {
		t.Fatalf("err = %v, want errDamaged", err)
	}
}

func TestADecoderPanicInAZipWorkerIsAnError(t *testing.T) {
	const method = 200
	zipMethods[method] = func(io.Reader, *zip.File) (io.ReadCloser, error) { return io.NopCloser(panicReader{}), nil }
	defer delete(zipMethods, method)
	dir := t.TempDir()
	archive := filepath.Join(dir, "p.zip")
	f, _ := os.Create(archive)
	w := zip.NewWriter(f)
	for i := range 4 {
		fw, _ := w.CreateRaw(&zip.FileHeader{Name: fmt.Sprintf("f%d", i), Method: method, CompressedSize64: 1, UncompressedSize64: 1})
		fw.Write([]byte{0})
	}
	w.Close()
	f.Close()
	if _, err := Extract(archive, "", nil); !errors.Is(err, errDamaged) {
		t.Fatalf("err = %v, want errDamaged", err)
	}
	eq(t, tree(t, dir), []string{"p.zip"})
}

// σ and ς are different lower-case letters but the same file name on
// Windows and macOS: never written by two workers at once.
func TestNamesThatFoldTogetherAreNotWrittenConcurrently(t *testing.T) {
	dir := t.TempDir()
	a, b := strings.Repeat("A", 8<<20), strings.Repeat("b", 3<<20)
	archive := zipOf(t, dir, "greek.zip", [][2]string{{"σ.txt", a}, {"other.txt", "x"}, {"ς.txt", b}})
	dst, err := Extract(archive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dst)
	for _, e := range ents {
		got, _ := os.ReadFile(filepath.Join(dst, e.Name()))
		if s := string(got); s != a && s != b && s != "x" {
			t.Fatalf("%s holds a mix of two members (%d bytes)", e.Name(), len(got))
		}
	}
}

func TestFilesInFlightCountAgainstTheReserve(t *testing.T) {
	defer func(f func(string) (uint64, bool)) { diskFree = f }(diskFree)
	diskFree = func(string) (uint64, bool) { return diskReserve + 100<<20, true }
	w := &writer{root: t.TempDir()}
	w.inflight.Store(90 << 20) // other workers are writing 90 MiB
	j := &job{m: &member{name: "x", size: 20 << 20}, rel: "x", dst: filepath.Join(w.root, "x")}
	if err := w.write(j, nil); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("err = %v, want ErrDiskFull", err)
	}
	if got := w.inflight.Load(); got != 90<<20 {
		t.Fatalf("inflight %d after the refusal, want it restored", got)
	}
}

type failAt struct{ data []byte }

func (f failAt) ReadAt(p []byte, off int64) (int, error) {
	n := copy(p, f.data[off:])
	return n, errors.New("bad sector")
}

func TestBlockReaderKeepsARealReadError(t *testing.T) {
	b := newBlockReaderAt(failAt{make([]byte, 100)})
	if _, err := b.ReadAt(make([]byte, 200), 0); err == nil || err.Error() != "bad sector" {
		t.Fatalf("err = %v, want the read error, not EOF", err)
	}
}
