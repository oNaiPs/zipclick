//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
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
