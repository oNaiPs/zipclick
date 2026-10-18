package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseBackend(t *testing.T) {
	for in, want := range map[string]backendMode{"auto": backendAuto, " Builtin ": backendBuiltin, "TAR": backendTar} {
		if got, err := parseBackend(in); err != nil || got != want {
			t.Errorf("parseBackend(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := parseBackend("7zip"); err == nil {
		t.Error("accepted an unknown backend")
	}
}

func TestOnlyPlainZipNamesGoToTar(t *testing.T) {
	for name, want := range map[string]bool{
		"docs/readme.txt":      true,
		"Café/notes.txt":       true,
		"Café.txt":            false, // NFD, as macOS writes it
		"a:b.txt":              false, // illegal on Windows
		"CON/x.txt":            false,
		"trailing. /x":         false,
		"../escape.txt":        false,
		`back\slash\ok.txt`:    true,
		"\x83\x82\x81 dos.txt": false, // OEM bytes without the UTF-8 flag
	} {
		nonUTF8 := name == "\x83\x82\x81 dos.txt"
		if got := plainName(name, nonUTF8); got != want {
			t.Errorf("plainName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestZipPlans(t *testing.T) {
	dir := t.TempDir()
	plan := func(name string, n int, method uint16, flags uint16, force bool) bool {
		t.Helper()
		p := filepath.Join(dir, name)
		f, _ := os.Create(p)
		w := zip.NewWriter(f)
		for i := range n {
			h := &zip.FileHeader{Name: fmt.Sprintf("f%d.txt", i), Method: method, Flags: flags}
			if method == 12 {
				fw, _ := w.CreateRaw(h)
				fw.Write([]byte{0})
			} else {
				fw, _ := w.CreateHeader(h)
				fw.Write([]byte("x"))
			}
		}
		w.Close()
		f.Close()
		src, err := openZip(p, "pw", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		_, ok := src.(planner).systemPlan(force)
		return ok
	}
	if !plan("many.zip", fewFiles+1, zip.Deflate, 0, false) {
		t.Error("a zip of many plain files should go to tar.exe")
	}
	if plan("few.zip", 10, zip.Deflate, 0, false) {
		t.Error("a zip of few files is faster with the engine")
	}
	if !plan("few-forced.zip", 10, zip.Deflate, 0, true) {
		t.Error("backend=tar should take a small zip too")
	}
	if plan("bzip2.zip", fewFiles+1, 12, 0, true) {
		t.Error("a method tar.exe may lack should stay with the engine")
	}
	if plan("locked.zip", fewFiles+1, zip.Store, 0x1, true) {
		t.Error("an encrypted zip should stay with the engine")
	}
}

func TestTidyTreeRemovesLinksAndMacJunk(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("keep"), 0o644)
	os.MkdirAll(filepath.Join(root, "pkg", "__MACOSX"), 0o755)
	os.MkdirAll(filepath.Join(root, "__MACOSX", "pkg"), 0o755)
	os.WriteFile(filepath.Join(root, "__MACOSX", "pkg", "._a"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "pkg", ".DS_Store"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "pkg", "a.txt"), []byte("a"), 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "pkg", "link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	removed, err := tidyTree(root)
	if err != nil || removed {
		t.Fatalf("removed=%v err=%v: links and junk are not gaps to fill", removed, err)
	}
	tops, _ := topNames(root)
	eq(t, tops, []string{"pkg"})
	eq(t, tree(t, root), []string{"pkg/", "pkg/__MACOSX/", "pkg/a.txt"})
	if b, _ := os.ReadFile(filepath.Join(outside, "keep.txt")); string(b) != "keep" {
		t.Fatal("followed the link")
	}
}

func TestTidyTreeRemovesNamesWindowsRefusesAndSaysSo(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "i2c"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "i2c", "ok.c"), []byte("ok"), 0o644)
	// What tar.exe writes through a \\?\ path; here, a plain file.
	os.WriteFile(filepath.Join(root, "src", "i2c", "aux.c"), []byte("aux"), 0o644)
	removed, err := tidyTree(root)
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v, want the bad name removed and reported", removed, err)
	}
	eq(t, tree(t, root), []string{"src/", "src/i2c/", "src/i2c/ok.c"})
}

// After tar.exe, the engine writes only what is missing, by its own rules.
func TestFillingInWritesOnlyWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	archive := zipOf(t, dir, "fill.zip", [][2]string{{"pkg/a.txt", "from the archive"}, {"pkg/aux.c", "aux"}, {"pkg/b.txt", "b"}})
	stage := filepath.Join(dir, "stage")
	os.MkdirAll(filepath.Join(stage, "pkg"), 0o755)
	os.WriteFile(filepath.Join(stage, "pkg", "a.txt"), []byte("written by tar.exe"), 0o644)
	src, err := openZip(archive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := writeAllMissing(stage, src, nil); err != nil {
		t.Fatal(err)
	}
	eq(t, tree(t, stage), []string{"pkg/", "pkg/_aux.c", "pkg/a.txt", "pkg/b.txt"})
	if b, _ := os.ReadFile(filepath.Join(stage, "pkg", "a.txt")); string(b) != "written by tar.exe" {
		t.Fatal("rewrote a file that was already there")
	}
}
