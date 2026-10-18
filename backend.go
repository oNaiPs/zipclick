package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ZipClick has two ways to write an archive's files out: its own engine, and
// the tar.exe that comes with Windows (libarchive). The second is there for
// speed, and mostly not tar.exe's own: Microsoft Defender scans every file an
// unknown program writes, about 2.5 ms each on a 16 KB file, and barely
// scans what tar.exe writes. An archive of 15,000 small files takes a minute
// one way and seven seconds the other.
//
// tar.exe only ever writes into the hidden staging folder, and ZipClick
// checks and tidies what it wrote before moving it into place, so the rules
// stay the same: nothing outside the folder, no links, no macOS metadata.
// Whatever tar.exe cannot do (passwords, names that need fixing, formats
// this Windows' tar.exe lacks), or any failure, goes to ZipClick's engine.

type backendMode string

const (
	backendAuto    backendMode = "auto"    // tar.exe where it is faster
	backendBuiltin backendMode = "builtin" // always ZipClick's engine
	backendTar     backendMode = "tar"     // tar.exe whenever it can
)

// backend is the mode in use: --backend=, then ZIPCLICK_BACKEND, then the
// saved setting, then auto. main sets it before extracting.
var backend = backendAuto

func parseBackend(s string) (backendMode, error) {
	switch m := backendMode(strings.ToLower(strings.TrimSpace(s))); m {
	case backendAuto, backendBuiltin, backendTar:
		return m, nil
	}
	return "", fmt.Errorf("unknown backend %q: use auto, builtin or tar", s)
}

// chooseBackend applies the environment variable and the saved setting;
// the command-line flag, handled in main, overrides both.
func chooseBackend() {
	for _, s := range []string{os.Getenv("ZIPCLICK_BACKEND"), savedBackend()} {
		if m, err := parseBackend(s); err == nil && s != "" {
			backend = m
			return
		}
	}
}

// fewFiles is the number of zip members below which ZipClick's own engine,
// which extracts a zip's files in parallel, beats tar.exe even with
// Defender's per-file cost.
const fewFiles = 256

// systemPlan says how tar.exe should be given an archive.
type systemPlan struct {
	stdin   bool  // feed the archive on stdin, which counts its bytes for progress
	entries int   // members, for progress when not on stdin
	bytes   int64 // the declared uncompressed total; 0 when unknown
}

// planner is a source that can say whether tar.exe may extract it.
type planner interface {
	systemPlan(force bool) (systemPlan, bool)
}

// systemExtracts counts archives tar.exe extracted, for the tests.
var systemExtracts atomic.Int64

// errPartial is what the tar.exe path returns when tar.exe failed on some
// members only.
var errPartial = errors.New("tar.exe failed on some members")

// errUseEngine is what the tar.exe path returns when ZipClick's own engine
// should extract the archive instead.
var errUseEngine = errors.New("extract with the built-in engine")

// planFor returns the plan for src, or false when the built-in engine
// should extract it.
func planFor(src source) (systemPlan, bool) {
	if backend == backendBuiltin || !systemTarAvailable() {
		return systemPlan{}, false
	}
	pl, ok := src.(planner)
	if !ok {
		return systemPlan{}, false
	}
	return pl.systemPlan(backend == backendTar)
}

// plainName reports a zip member name that tar.exe would write exactly as
// ZipClick would: valid UTF-8 in NFC, every component already legal on
// Windows. Anything else needs ZipClick's renaming.
func plainName(name string, nonUTF8 bool) bool {
	if nonUTF8 && !isASCII(name) || !utf8.ValidString(name) || !norm.NFC.IsNormalString(name) {
		return false
	}
	for _, c := range strings.Split(strings.ReplaceAll(name, `\`, "/"), "/") {
		if c == "" || c == "." {
			continue
		}
		if c == ".." || sanitizeComponent(c) != c {
			return false
		}
	}
	return true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// tidyTree removes from what tar.exe wrote under root everything ZipClick
// would not have written: links and junctions (libarchive can create them on
// Windows), macOS metadata, and names Windows refuses (tar.exe writes
// through \\?\ paths, which let a file be called "aux.c" or end in a dot;
// ZipClick's engine writes those as "_aux.c" and without the dot). removed
// reports whether any such name went, so the engine has gaps to fill.
func tidyTree(root string) (removed bool, err error) {
	var junk []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		bad := sanitizeComponent(d.Name()) != d.Name()
		if bad || d.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || isMacJunk(rel) {
			junk = append(junk, p)
			removed = removed || bad
			if d.IsDir() {
				return filepath.SkipDir // never read a folder called "con"
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	for _, p := range junk {
		if err := removeVerbatim(p); err != nil {
			return false, err
		}
	}
	return removed, nil
}

// verbatimRel is a member name as tar.exe wrote it under the destination,
// without ZipClick's renaming: leading slashes and drive letters dropped,
// as libarchive does. ok is false for a name tar.exe would have refused.
func verbatimRel(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	var parts []string
	for _, p := range strings.Split(name, "/") {
		switch {
		case p == "" || p == ".":
			continue
		case p == "..":
			return "", false
		case len(parts) == 0 && len(p) == 2 && p[1] == ':':
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "/"), true
}

// emptyDir removes everything inside dir, keeping dir.
func emptyDir(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if err := removeVerbatim(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// topNames lists root's entries, sorted.
func topNames(root string) ([]string, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	tops := make([]string, 0, len(ents))
	for _, e := range ents {
		tops = append(tops, e.Name())
	}
	sort.Strings(tops)
	return tops, nil
}
