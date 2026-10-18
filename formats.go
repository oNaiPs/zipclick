package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// openArchive recognises the archive by its first bytes, not its extension:
// a .zip that is really a 7z, or a .tar.gz renamed .tgz, opens all the same.
func openArchive(path, password string, p *Progress) (source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	f.Close()

	switch {
	case bytes.HasPrefix(head, []byte("7z\xbc\xaf\x27\x1c")):
		return open7z(path, password, p)
	case bytes.HasPrefix(head, []byte("Rar!\x1a\x07")):
		return openRar(path, password, p)
	case isTar(head):
		return openStream(path, p, nil)
	}
	if dec := streamDecoder(head); dec != nil {
		return openStream(path, p, dec)
	}
	// Zip last: its signature is at the end of the file, and a zip may sit
	// behind a self-extractor's code.
	src, err := openZip(path, password, p)
	if err == nil || strings.EqualFold(filepath.Ext(path), ".zip") || !isFormatError(err) {
		return src, err
	}
	return nil, ErrUnknownFormat
}

// streamDecoder returns how to decompress a single-stream file (gzip, bzip2,
// xz, zstd), or nil when head is none of them.
func streamDecoder(head []byte) func(io.Reader) (io.ReadCloser, error) {
	switch {
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		return func(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) }
	case bytes.HasPrefix(head, []byte("BZh")):
		return func(r io.Reader) (io.ReadCloser, error) { return io.NopCloser(bzip2.NewReader(r)), nil }
	case bytes.HasPrefix(head, []byte{0xfd, '7', 'z', 'X', 'Z', 0}):
		return func(r io.Reader) (io.ReadCloser, error) {
			x, err := xz.NewReader(r)
			return io.NopCloser(x), err
		}
	case bytes.HasPrefix(head, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return func(r io.Reader) (io.ReadCloser, error) {
			z, err := zstd.NewReader(r, zstd.WithDecoderLowmem(true))
			if err != nil {
				return nil, err
			}
			return z.IOReadCloser(), nil
		}
	}
	return nil
}

// isTar recognises a tar header: POSIX and GNU ones say "ustar" at 257, and
// the old v7 format is known only by its checksum.
func isTar(head []byte) bool {
	if len(head) < 512 {
		return false
	}
	if bytes.Equal(head[257:262], []byte("ustar")) {
		return true
	}
	stored := strings.TrimRight(strings.TrimSpace(string(bytes.TrimRight(head[148:156], "\x00 "))), "\x00")
	if stored == "" {
		return false
	}
	var sum int64
	for i, b := range head {
		if i >= 148 && i < 156 {
			b = ' '
		}
		sum += int64(b)
	}
	var want int64
	for _, c := range stored {
		if c < '0' || c > '7' {
			return false
		}
		want = want*8 + int64(c-'0')
	}
	return sum == want
}

// openStream opens a tar, or something compressed with a single-stream
// method: a tar inside becomes its members, anything else a single file named
// after the archive ("notes.txt.gz" gives "notes.txt"). Progress is counted on
// the compressed side, since the uncompressed size is not known in advance.
func openStream(path string, p *Progress, dec func(io.Reader) (io.ReadCloser, error)) (source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err == nil {
		p.setTotal(fi.Size())
	}
	var r io.Reader = bufio.NewReaderSize(&progressReader{f, p}, 256<<10)
	var dc io.ReadCloser
	if dec != nil {
		if dc, err = dec(r); err != nil {
			f.Close()
			return nil, err
		}
		r = dc
	}
	closer := closers{dc, f}

	br := bufio.NewReaderSize(r, 64<<10)
	head, _ := br.Peek(512)
	if dec == nil || isTar(head) {
		return newTarSource(br, closer), nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	s := &singleSource{m: &member{name: name, size: -1, open: func() (io.ReadCloser, error) { return io.NopCloser(br), nil }}, c: closer}
	if gz, ok := dc.(*gzip.Reader); ok {
		s.m.mtime = gz.ModTime
	}
	return s, nil
}

// closers closes each non-nil Closer in order.
type closers []io.Closer

func (cs closers) Close() error {
	var first error
	for _, c := range cs {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// singleSource is a compressed file that holds one file, not an archive.
type singleSource struct {
	m    *member
	done bool
	c    io.Closer
}

func (s *singleSource) next() (*member, error) {
	if s.done {
		return nil, io.EOF
	}
	s.done = true
	return s.m, nil
}

func (s *singleSource) countsInput() bool { return true }
func (s *singleSource) Close() error      { return s.c.Close() }

// isFormatError reports that the file is not a zip at all, as opposed to a
// zip that failed for another reason.
func isFormatError(err error) bool {
	return errors.Is(err, zip.ErrFormat)
}
