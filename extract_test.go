package main

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

type member struct {
	name    string
	body    string
	rawName bool // write name bytes as-is, without the UTF-8 flag
	flags   uint16
}

func makeZip(t *testing.T, dir, name string, members ...member) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, m := range members {
		h := &zip.FileHeader{Name: m.name, Method: zip.Deflate, Modified: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)}
		if m.rawName {
			h.NonUTF8 = true
		}
		h.Flags |= m.flags
		fw, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(m.name, "/") {
			fw.Write([]byte(m.body))
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

// tree lists every path under root, relative and slash-separated.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if p != root {
			rel, _ := filepath.Rel(root, p)
			if info.IsDir() {
				rel += "/"
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestSeveralTopLevelItemsGoInAFolderNamedAfterTheArchive(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "photos.zip", member{name: "a.jpg", body: "A"}, member{name: "b/c.jpg", body: "C"})
	dst, err := Extract(z, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dst != filepath.Join(d, "photos") {
		t.Fatalf("dst = %s", dst)
	}
	eq(t, tree(t, d), []string{"photos.zip", "photos/", "photos/a.jpg", "photos/b/", "photos/b/c.jpg"})
}

func TestASingleTopLevelFolderLandsBesideTheArchive(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "download.zip", member{name: "project/"}, member{name: "project/x.txt", body: "x"})
	if _, err := Extract(z, nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, d), []string{"download.zip", "project/", "project/x.txt"})
}

func TestASingleFileLandsBesideTheArchive(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "report.zip", member{name: "report.pdf", body: "%PDF"})
	if _, err := Extract(z, nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, d), []string{"report.pdf", "report.zip"})
	b, _ := os.ReadFile(filepath.Join(d, "report.pdf"))
	if string(b) != "%PDF" {
		t.Fatalf("content %q", b)
	}
}

func TestExistingNamesAreNeverOverwritten(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "report.zip", member{name: "report.pdf", body: "new"})
	os.WriteFile(filepath.Join(d, "report.pdf"), []byte("old"), 0o644)
	for range 2 {
		if _, err := Extract(z, nil); err != nil {
			t.Fatal(err)
		}
	}
	eq(t, tree(t, d), []string{"report 2.pdf", "report 3.pdf", "report.pdf", "report.zip"})
	b, _ := os.ReadFile(filepath.Join(d, "report.pdf"))
	if string(b) != "old" {
		t.Fatal("overwrote the existing file")
	}
}

func TestTraversalCannotEscapeAndDriveLettersAreDropped(t *testing.T) {
	d := t.TempDir()
	sub := filepath.Join(d, "sub")
	os.Mkdir(sub, 0o755)
	z := makeZip(t, sub, "evil.zip",
		member{name: "../escaped.txt", body: "x"},
		member{name: "ok/../../escaped2.txt", body: "x"},
		member{name: "/abs.txt", body: "x"},
		member{name: `C:\win.txt`, body: "x"},
	)
	if _, err := Extract(z, nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, d), []string{"sub/", "sub/evil.zip", "sub/evil/", "sub/evil/abs.txt", "sub/evil/win.txt"})
}

func TestMacMetadataIsDropped(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "fromamac.zip",
		member{name: "Folder/"},
		member{name: "Folder/file.txt", body: "x"},
		member{name: "Folder/.DS_Store", body: "x"},
		member{name: "__MACOSX/Folder/._file.txt", body: "x"},
	)
	if _, err := Extract(z, nil); err != nil {
		t.Fatal(err)
	}
	// __MACOSX would otherwise make it two top-level items, and a needless folder.
	eq(t, tree(t, d), []string{"Folder/", "Folder/file.txt", "fromamac.zip"})
}

func TestNamesAreDecodedAndNormalised(t *testing.T) {
	d := t.TempDir()
	oem, _ := charmap.CodePage437.NewEncoder().String("Façade é.txt")
	z := makeZip(t, d, "names.zip",
		member{name: oem, body: "x", rawName: true},
		member{name: "Cafe\u0301.txt", body: "x"}, // macOS writes NFD
	)
	if _, err := Extract(z, nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, d), []string{"names.zip", "names/", "names/Café.txt", "names/Façade é.txt"})
}

func TestWindowsIllegalNamesAreMadeLegal(t *testing.T) {
	for in, want := range map[string]string{
		"a:b?.txt": "a_b_.txt",
		"CON":      "_CON",
		"nul.tar":  "_nul.tar",
		"trail. ":  "trail",
		"...":      "_",
		"CONSOLE":  "CONSOLE",
	} {
		if got := sanitizeComponent(in); got != want {
			t.Errorf("sanitizeComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestModificationTimesAreKept(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "t.zip", member{name: "f.txt", body: "x"})
	if _, err := Extract(z, nil); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(d, "f.txt"))
	if want := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC); !fi.ModTime().Equal(want) {
		t.Fatalf("mtime %v, want %v", fi.ModTime(), want)
	}
}

func TestDOSOnlyTimesAreLocalTime(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "dos.zip")
	f, _ := os.Create(p)
	w := zip.NewWriter(f)
	// 2020-01-02 03:04:04 in MS-DOS fields only, as Explorer writes it.
	h := &zip.FileHeader{Name: "f.txt", ModifiedDate: (2020-1980)<<9 | 1<<5 | 2, ModifiedTime: 3<<11 | 4<<5 | 2}
	fw, _ := w.CreateHeader(h)
	fw.Write([]byte("x"))
	w.Close()
	f.Close()
	if _, err := Extract(p, nil); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(d, "f.txt"))
	if want := time.Date(2020, 1, 2, 3, 4, 4, 0, time.Local); !fi.ModTime().Equal(want) {
		t.Fatalf("mtime %v, want %v", fi.ModTime(), want)
	}
}

func TestFailuresLeaveNothingBehind(t *testing.T) {
	d := t.TempDir()
	enc := makeZip(t, d, "secret.zip", member{name: "s.txt", body: "x", flags: 0x1})
	if _, err := Extract(enc, nil); !errors.Is(err, ErrEncrypted) {
		t.Fatalf("err = %v, want ErrEncrypted", err)
	}
	empty := makeZip(t, d, "empty.zip", member{name: "__MACOSX/._x", body: "x"})
	if _, err := Extract(empty, nil); !errors.Is(err, ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
	notzip := filepath.Join(d, "notzip.zip")
	os.WriteFile(notzip, []byte("hello"), 0o644)
	if _, err := Extract(notzip, nil); err == nil {
		t.Fatal("expected an error for a non-zip")
	}
	eq(t, tree(t, d), []string{"empty.zip", "notzip.zip", "secret.zip"})
}

func TestProgressCountsEveryByte(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "p.zip", member{name: "a.txt", body: strings.Repeat("a", 1000)}, member{name: "b/"}, member{name: "b/c.txt", body: "ccc"})
	p := &Progress{}
	if _, err := Extract(z, p); err != nil {
		t.Fatal(err)
	}
	if done, total := p.Bytes(); done != 1003 || total != 1003 {
		t.Fatalf("progress %d/%d, want 1003/1003", done, total)
	}
}

func TestCancelLeavesNothingBehind(t *testing.T) {
	d := t.TempDir()
	z := makeZip(t, d, "big.zip", member{name: "a.bin", body: strings.Repeat("x", 4<<20)}, member{name: "b.bin", body: "y"})
	p := &Progress{}
	p.Cancel()
	if _, err := Extract(z, p); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want ErrCancelled", err)
	}
	eq(t, tree(t, d), []string{"big.zip"})
}

func TestDetailText(t *testing.T) {
	for _, c := range []struct {
		done, total int64
		elapsed     time.Duration
		want        string
	}{
		{0, 500, time.Second, "0 bytes of 500 bytes"},
		{1 << 20, 40 << 20, time.Second, "1.0 MB of 40.0 MB"},
		{10 << 20, 40 << 20, 10 * time.Second, "10.0 MB of 40.0 MB — about 30 seconds left"},
		{39 << 20, 40 << 20, 10 * time.Second, "39.0 MB of 40.0 MB — a few seconds left"},
		{1 << 30, 10 << 30, 30 * time.Second, "1.0 GB of 10.0 GB — about 5 minutes left"},
	} {
		if got := detailText(c.done, c.total, c.elapsed); got != c.want {
			t.Errorf("detailText(%d, %d, %v) = %q, want %q", c.done, c.total, c.elapsed, got, c.want)
		}
	}
}
