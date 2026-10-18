//go:build windows

package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures through Windows' own tar.exe: the same tree as the engine
// writes, or (where this Windows' tar.exe lacks a format) the engine took
// over and the result is still right.
func TestTarExeExtractsTheFixtures(t *testing.T) {
	if !systemTarAvailable() {
		t.Skip("no tar.exe")
	}
	defer func(m backendMode) { backend = m }(backend)
	backend = backendTar
	for _, name := range []string{"album.7z", "album.rar", "album.tar", "album.tar.gz", "album.tar.bz2", "album.tar.xz", "album.tar.zst"} {
		t.Run(name, func(t *testing.T) {
			before := systemExtracts.Load()
			dir, archive := fixture(t, name)
			p := &Progress{}
			dst, err := Extract(archive, "", p)
			if err != nil {
				t.Fatal(err)
			}
			if dst != filepath.Join(dir, "album") {
				t.Errorf("dst = %s", dst)
			}
			checkAlbum(t, dir, archive)
			t.Logf("tar.exe used: %v", systemExtracts.Load() > before)
			if done, total := p.Bytes(); total == 0 || done != total {
				t.Errorf("progress %d/%d", done, total)
			}
		})
	}
}

// tar.exe cannot be told about the reserve: ZipClick watches the free space
// while it runs and stops it when the drive gets too full. The archive is
// the 3 KB sparse tar from the security tests (NTFS stores it sparse, so it
// is written in no time: the first check is what has to catch it).
func TestTarExeIsStoppedWhenTheDiskFills(t *testing.T) {
	if !systemTarAvailable() {
		t.Skip("no tar.exe")
	}
	defer func(m backendMode) { backend = m }(backend)
	backend = backendTar
	defer func(f func(string) (uint64, bool)) { diskFree = f }(diskFree)
	diskFree = func(string) (uint64, bool) { return diskReserve - 1, true }

	dir := t.TempDir()
	archive := filepath.Join(dir, "bomb.tar")
	if err := os.WriteFile(archive, sparseBomb(512<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	before := systemExtracts.Load()
	if _, err := Extract(archive, "", nil); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("err = %v, want ErrDiskFull", err)
	}
	if systemExtracts.Load() != before {
		t.Fatal("tar.exe was not stopped")
	}
	eq(t, tree(t, dir), []string{"bomb.tar"})
}

// tar.exe cannot make the link (and fails over it) and writes aux.c through
// a \\?\ path; the result must still be what the engine would write.
func TestTarExeGapsAreFilledByTheEngine(t *testing.T) {
	if !systemTarAvailable() {
		t.Skip("no tar.exe")
	}
	defer func(m backendMode) { backend = m }(backend)
	backend = backendTar
	dir := t.TempDir()
	archive := filepath.Join(dir, "repo.tar")
	f, _ := os.Create(archive)
	w := tar.NewWriter(f)
	add := func(h *tar.Header, body string) {
		h.Size = int64(len(body))
		h.Mode = 0o644
		w.WriteHeader(h)
		w.Write([]byte(body))
	}
	add(&tar.Header{Name: "repo/a.txt", Typeflag: tar.TypeReg}, "a")
	add(&tar.Header{Name: "repo/link", Typeflag: tar.TypeSymlink, Linkname: "a.txt"}, "")
	add(&tar.Header{Name: "repo/i2c/aux.c", Typeflag: tar.TypeReg}, "aux")
	add(&tar.Header{Name: "repo/z.txt", Typeflag: tar.TypeReg}, "z")
	w.Close()
	f.Close()
	if _, err := Extract(archive, "", nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, dir), sorted([]string{"repo.tar", "repo/", "repo/a.txt", "repo/i2c/", "repo/i2c/_aux.c", "repo/z.txt"}))
}

// A damaged archive through tar.exe fails, as through the engine: tar.exe's
// file for the damaged member is not trusted.
func TestTarExeDamageIsNotSwallowed(t *testing.T) {
	if !systemTarAvailable() {
		t.Skip("no tar.exe")
	}
	defer func(m backendMode) { backend = m }(backend)
	backend = backendTar

	dir := t.TempDir()
	var files [][2]string
	for i := range 300 {
		files = append(files, [2]string{fmt.Sprintf("pkg/f%d.txt", i), strings.Repeat(fmt.Sprintf("line %d\n", i), 2000)})
	}
	archive := zipOf(t, dir, "bad.zip", files)
	b, _ := os.ReadFile(archive)
	i := bytes.Index(b, []byte("pkg/f150.txt")) + len("pkg/f150.txt") + 60
	b[i] ^= 0xff
	os.WriteFile(archive, b, 0o644)
	if _, err := Extract(archive, "", nil); err == nil {
		t.Fatal("a damaged zip extracted without an error")
	}
	eq(t, tree(t, dir), []string{"bad.zip"})

	dir2, tgz := fixture(t, "album.tar.gz")
	b, _ = os.ReadFile(tgz)
	os.WriteFile(tgz, b[:len(b)/2], 0o644)
	if _, err := Extract(tgz, "", nil); err == nil {
		t.Fatal("a truncated tar.gz extracted without an error")
	}
	eq(t, tree(t, dir2), []string{"album.tar.gz"})
}
