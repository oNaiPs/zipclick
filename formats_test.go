package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The archives in testdata are made by testdata/make.sh.

const (
	fixturePassword = "sésamo"
	aesPassword     = "open sesame"
)

var albumTree = []string{"album/", "album/one.txt", "album/sub/", "album/sub/lines.txt", "album/sub/two.txt"}

// fixture copies testdata/name into a fresh directory, since Extract writes
// next to the archive.
func fixture(t *testing.T, name string) (dir, path string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	path = filepath.Join(dir, name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func checkAlbum(t *testing.T, dir, archive string) {
	t.Helper()
	eq(t, tree(t, dir), sorted(append([]string{filepath.Base(archive)}, albumTree...)))
	for name, want := range map[string]string{"album/one.txt": "one\n", "album/sub/two.txt": "two\n"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != want {
			t.Errorf("%s = %q, %v; want %q", name, b, err, want)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "album/sub/lines.txt"))
	if lines := strings.Split(strings.TrimSpace(string(b)), "\n"); len(lines) != 5000 || lines[4999] != "5000 green bottles" {
		t.Errorf("lines.txt has %d lines, last %q", len(lines), lines[len(lines)-1])
	}
	fi, err := os.Stat(filepath.Join(dir, "album/one.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// make.sh ran in Lisbon in winter, where local time is UTC: the formats
	// that store an instant and the zips that store wall-clock time agree.
	want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	got := fi.ModTime()
	wall := time.Date(got.Year(), got.Month(), got.Day(), got.Hour(), got.Minute(), got.Second(), 0, time.UTC)
	if !got.Equal(want) && !wall.Equal(want) {
		t.Errorf("one.txt mtime %v, want %v", got, want)
	}
}

func sorted(s []string) []string {
	sort.Strings(s)
	return s
}

func TestEveryFormatExtracts(t *testing.T) {
	for _, name := range []string{
		"album.7z",
		"album.rar", "album-solid.rar",
		"album.tar", "album.tar.gz", "album.tgz", "album.tar.bz2", "album.tar.xz", "album.tar.zst",
		"album-bzip2.zip", "album-lzma.zip", "album-deflate64.zip",
	} {
		t.Run(name, func(t *testing.T) {
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
			if done, total := p.Bytes(); total == 0 || done != total {
				t.Errorf("progress %d/%d", done, total)
			}
		})
	}
}

func TestPasswords(t *testing.T) {
	for name, password := range map[string]string{
		"album-pw.7z":            fixturePassword,
		"album-pwhead.7z":        fixturePassword, // the file names are encrypted too
		"album-pw.rar":           fixturePassword,
		"album-pwhead.rar":       fixturePassword,
		"album-zipcrypto.zip":    fixturePassword,
		"album-aes.zip":          aesPassword,
		"album-aes128-bzip2.zip": aesPassword,
	} {
		t.Run(name, func(t *testing.T) {
			dir, archive := fixture(t, name)
			if _, err := Extract(archive, "", nil); !errors.Is(err, ErrEncrypted) {
				t.Fatalf("no password: err = %v, want ErrEncrypted", err)
			}
			for _, wrong := range []string{"sesamo", "open sesame!", "x"} {
				if _, err := Extract(archive, wrong, nil); !errors.Is(err, ErrBadPassword) {
					t.Fatalf("password %q: err = %v, want ErrBadPassword", wrong, err)
				}
			}
			eq(t, tree(t, dir), []string{name}) // the failures left nothing
			if _, err := Extract(archive, password, nil); err != nil {
				t.Fatal(err)
			}
			checkAlbum(t, dir, archive)
		})
	}
}

func TestACompressedFileThatIsNotATarIsOneFile(t *testing.T) {
	for _, name := range []string{"notes.txt.gz", "notes.txt.xz"} {
		t.Run(name, func(t *testing.T) {
			dir, archive := fixture(t, name)
			dst, err := Extract(archive, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if dst != filepath.Join(dir, "notes.txt") {
				t.Errorf("dst = %s", dst)
			}
			b, _ := os.ReadFile(dst)
			if string(b) != "just some notes\n" {
				t.Errorf("content %q", b)
			}
		})
	}
}

func TestTheFormatComesFromTheContentNotTheName(t *testing.T) {
	dir := t.TempDir()
	b, _ := os.ReadFile("testdata/album.7z")
	archive := filepath.Join(dir, "really a 7z.zip")
	os.WriteFile(archive, b, 0o644)
	if _, err := Extract(archive, "", nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, dir), sorted(append([]string{"really a 7z.zip"}, albumTree...)))
}

func TestUnknownFilesAreNamedAsSuch(t *testing.T) {
	dir := t.TempDir()
	notrar := filepath.Join(dir, "notes.rar")
	os.WriteFile(notrar, []byte("hello, this is not an archive"), 0o644)
	if _, err := Extract(notrar, "", nil); !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("err = %v, want ErrUnknownFormat", err)
	}
}

func TestArchiveStem(t *testing.T) {
	for in, want := range map[string]string{
		"photos.zip":        "photos",
		"linux-6.1.tar.xz":  "linux-6.1",
		"backup.TAR.GZ":     "backup",
		"site.tgz":          "site",
		"notes.txt.gz":      "notes.txt",
		"no extension":      "no extension",
		"version.1.2.3.zip": "version.1.2.3",
	} {
		if got := archiveStem(in); got != want {
			t.Errorf("archiveStem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeflate64UsesItsLongDistancesAndLengths(t *testing.T) {
	_, archive := fixture(t, "deflate64-stress.zip")
	dst, err := Extract(archive, "", nil) // the CRC is checked as it is read
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	if len(b) != 380000 || string(b[:40000]) != string(b[40000:80000]) || strings.Trim(string(b[80000:]), "\x00") != "" {
		t.Fatalf("big.bin is %d bytes, not the two copies and the zeros", len(b))
	}
}
