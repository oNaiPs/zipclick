package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const downloaded = "[ZoneTransfer]\r\nZoneId=3\r\nHostUrl=https://example.com/a.zip\r\n"

// markedFiles maps each file under dir to its Mark of the Web ("" if none).
func markedFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p + zoneStream)
		rel, _ := filepath.Rel(dir, p)
		got[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	return got
}

func TestTheMarkOfTheWebReachesEveryFileOfAZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	f, _ := os.Create(archive)
	w := zip.NewWriter(f)
	when := time.Date(2020, 5, 6, 7, 8, 10, 0, time.UTC)
	for i, name := range []string{"setup.exe", "docs/report.docm", "docs/deep/run.lnk"} {
		fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: when})
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(bytes.Repeat([]byte{byte('a' + i)}, 5000))
	}
	w.Close()
	f.Close()
	os.WriteFile(archive+zoneStream, []byte(downloaded), 0o644)

	dst, err := Extract(archive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := markedFiles(t, dst)
	if len(got) != 3 {
		t.Fatalf("files %v", got)
	}
	for name, zone := range got {
		if zone != downloaded {
			t.Errorf("%s: mark %q", name, zone)
		}
	}
	// Marking happens before the times are set, so they survive it.
	fi, _ := os.Stat(filepath.Join(dst, "setup.exe"))
	if !fi.ModTime().Equal(when) {
		t.Errorf("mtime %v, want %v", fi.ModTime().UTC(), when)
	}
}

func TestTheMarkOfTheWebReachesTarFilesAndHardLinks(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	tw.WriteHeader(&tar.Header{Name: "pkg/tool.exe", Mode: 0o755, Size: 4, Typeflag: tar.TypeReg})
	tw.Write([]byte("MZ.."))
	tw.WriteHeader(&tar.Header{Name: "pkg/copy.exe", Linkname: "pkg/tool.exe", Typeflag: tar.TypeLink})
	tw.Close()
	archive := filepath.Join(dir, "a.tar")
	os.WriteFile(archive, b.Bytes(), 0o644)
	os.WriteFile(archive+zoneStream, []byte(downloaded), 0o644)

	dst, err := Extract(archive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := markedFiles(t, filepath.Dir(dst))
	for _, name := range []string{"pkg/tool.exe", "pkg/copy.exe"} {
		if got[name] != downloaded {
			t.Errorf("%s: mark %q (all: %v)", name, got[name], got)
		}
	}
}

func TestAnUnmarkedArchiveGivesUnmarkedFiles(t *testing.T) {
	dir := t.TempDir()
	archive := zipOf(t, dir, "local.zip", [][2]string{{"a.txt", "a"}, {"b/c.txt", "c"}})
	dst, err := Extract(archive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, zone := range markedFiles(t, dst) {
		if zone != "" {
			t.Errorf("%s: mark %q on a file from an unmarked archive", name, zone)
		}
	}
}

func TestAnOversizedMarkCountsAsTheInternet(t *testing.T) {
	dir := t.TempDir()
	archive := zipOf(t, dir, "big.zip", [][2]string{{"a.txt", "a"}})
	os.WriteFile(archive+zoneStream, []byte(strings.Repeat("x", maxZone+1)), 0o644)
	if got := string(zoneOf(archive)); got != string(internetZone) {
		t.Fatalf("zone %q", got)
	}
	os.WriteFile(archive+zoneStream, nil, 0o644)
	if got := string(zoneOf(archive)); got != string(internetZone) {
		t.Fatalf("empty mark gave %q", got)
	}
}
