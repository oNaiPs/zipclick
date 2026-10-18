package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Archives built to hurt the extractor, from the security review. Each must
// end in an ordinary error with nothing left behind, never a crash, a full
// disk or a giant allocation.

func TestCrafted7zIsDamagedNotACrash(t *testing.T) {
	for _, name := range []string{
		"no-streams.7z",       // files but no streams: a nil dereference in the library
		"bad-digest-index.7z", // a checksum index past the end of its table
		"numfiles-4G.7z",      // 0xFFFFFFFF files: a 479 GiB allocation
	} {
		t.Run(name, func(t *testing.T) {
			dir, archive := fixture(t, "hostile/"+name)
			_, err := Extract(archive, "", nil)
			if err == nil {
				t.Fatal("extracted a crafted archive")
			}
			if !damaged(err) {
				t.Errorf("err = %v, want one reported as a damaged archive", err)
			}
			eq(t, tree(t, dir), []string{name})
		})
	}
}

// A 3 KB sparse tar that expands to as much as it says (256 MiB here; it
// could say a terabyte). The free-space guard has to stop it.
func TestDecompressionBombStopsBeforeTheDiskFills(t *testing.T) {
	const realsize = 256 << 20
	var recs bytes.Buffer
	for _, kv := range [][2]string{{"GNU.sparse.major", "1"}, {"GNU.sparse.minor", "0"}, {"GNU.sparse.name", "big.bin"}, {"GNU.sparse.realsize", fmt.Sprint(realsize)}} {
		s := fmt.Sprintf(" %s=%s\n", kv[0], kv[1])
		n := len(s) + 1
		for len(fmt.Sprint(n))+len(s) != n {
			n++
		}
		fmt.Fprintf(&recs, "%d%s", n, s)
	}
	var b []byte
	b = append(b, ustarHeader("./PaxHeaders/big.bin", 'x', int64(recs.Len()))...)
	b = append(b, pad512(recs.Bytes())...)
	b = append(b, ustarHeader("./GNUSparseFile.0/big.bin", '0', 512)...)
	b = append(b, pad512([]byte("0\n"))...)
	b = append(b, make([]byte, 1024)...)

	dir := t.TempDir()
	archive := filepath.Join(dir, "bomb.tar")
	os.WriteFile(archive, b, 0o644)

	// Pretend the drive has room for the reserve and 100 MiB more.
	defer func(f func(string) (uint64, bool)) { diskFree = f }(diskFree)
	diskFree = func(string) (uint64, bool) { return diskReserve + 100<<20, true }

	if _, err := Extract(archive, "", nil); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("err = %v, want ErrDiskFull", err)
	}
	eq(t, tree(t, dir), []string{"bomb.tar"})
}

func TestTheFreeSpaceGuardChecksAsItWrites(t *testing.T) {
	// A member with no declared size (a lone .gz) is only caught while
	// writing: the drive "fills up" after the first check interval.
	dir := t.TempDir()
	var calls int
	defer func(f func(string) (uint64, bool)) { diskFree = f }(diskFree)
	diskFree = func(string) (uint64, bool) {
		calls++
		return diskReserve - 1, true
	}
	m := &member{name: "x", size: -1, open: func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(make([]byte, checkInterval+1))), nil
	}}
	if err := writeFile(filepath.Join(dir, "x"), m, nil, nil); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("err = %v after %d checks, want ErrDiskFull", err, calls)
	}
}

func TestAMemberCannotOutgrowItsHeader(t *testing.T) {
	dir := t.TempDir()
	m := &member{name: "x", size: 10, open: func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(make([]byte, 11))), nil
	}}
	if err := writeFile(filepath.Join(dir, "x"), m, nil, nil); !errors.Is(err, errTooLong) {
		t.Fatalf("err = %v, want errTooLong", err)
	}
}

// A 146-byte zip whose LZMA member asks for a 2 GiB dictionary.
func TestAHugeLZMADictionaryIsRefused(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "lzma.zip")
	f, _ := os.Create(archive)
	w := zip.NewWriter(f)
	raw := append([]byte{0x09, 0x14, 0x05, 0x00, 0x5d, 0xff, 0xff, 0xff, 0x7f}, make([]byte, 64)...)
	fw, err := w.CreateRaw(&zip.FileHeader{Name: "big", Method: 14, UncompressedSize64: 1 << 40, CompressedSize64: uint64(len(raw)), CRC32: 1})
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(raw)
	w.Close()
	f.Close()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := Extract(archive, "", nil); err == nil {
		t.Fatal("extracted it")
	}
	runtime.ReadMemStats(&after)
	if grew := (after.TotalAlloc - before.TotalAlloc) >> 20; grew > 300 {
		t.Errorf("allocated %d MiB", grew)
	}
	eq(t, tree(t, dir), []string{"lzma.zip"})
}

func TestTheFinalRenameNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new.txt")
	os.WriteFile(src, []byte("new"), 0o644)
	os.WriteFile(filepath.Join(dir, "taken.txt"), []byte("old"), 0o644)
	if err := renameNoReplace(src, filepath.Join(dir, "taken.txt")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want os.ErrExist", err)
	}
	dst, err := moveUnique(src, dir, "taken.txt")
	if err != nil || filepath.Base(dst) != "taken 2.txt" {
		t.Fatalf("moveUnique = %s, %v", dst, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "taken.txt")); string(b) != "old" {
		t.Fatal("the existing file was replaced")
	}
}

func ustarHeader(name string, typ byte, size int64) []byte {
	b := make([]byte, 512)
	copy(b[0:], name)
	copy(b[100:], "0000644\x00")
	copy(b[108:], "0000000\x00")
	copy(b[116:], "0000000\x00")
	copy(b[124:], fmt.Sprintf("%011o\x00", size))
	copy(b[136:], "00000000000\x00")
	b[156] = typ
	copy(b[257:], "ustar\x00")
	copy(b[263:], "00")
	copy(b[148:], "        ")
	sum := 0
	for _, c := range b {
		sum += int(c)
	}
	copy(b[148:], fmt.Sprintf("%06o\x00 ", sum))
	return b
}

func pad512(b []byte) []byte {
	for len(b)%512 != 0 {
		b = append(b, 0)
	}
	return b
}
