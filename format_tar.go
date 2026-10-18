package main

import (
	"archive/tar"
	"io"
)

// tarSource reads a tar stream; its members come in order and each must be
// read before the next.
type tarSource struct {
	r *tar.Reader
	c io.Closer
}

func newTarSource(r io.Reader, c io.Closer) *tarSource {
	return &tarSource{tar.NewReader(r), c}
}

func (s *tarSource) next() (*member, error) {
	h, err := s.r.Next()
	if err != nil {
		return nil, err
	}
	m := &member{
		name:  h.Name,
		exec:  h.Mode&0o100 != 0,
		size:  h.Size,
		mtime: h.ModTime,
		open:  func() (io.ReadCloser, error) { return io.NopCloser(s.r), nil },
	}
	switch h.Typeflag {
	case tar.TypeReg, tar.TypeGNUSparse:
	case tar.TypeDir:
		m.dir = true
	case tar.TypeLink:
		m.hardlink = h.Linkname
	default:
		m.skip = true // symbolic links, devices, FIFOs
	}
	return m, nil
}

func (s *tarSource) countsInput() bool { return true }

// systemPlan hands a tar, compressed or not, to tar.exe on stdin, so the
// progress still counts the archive's bytes as they are read.
func (s *tarSource) systemPlan(bool) (systemPlan, bool) {
	return systemPlan{stdin: true}, true
}
func (s *tarSource) Close() error { return s.c.Close() }
