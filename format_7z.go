package main

import (
	"errors"
	"io"
	"os"

	"github.com/bodgit/sevenzip"
)

// sevenZipSource reads a 7z. Members are read in archive order, which is
// what keeps solid archives from being decompressed more than once.
type sevenZipSource struct {
	r        *sevenzip.Reader
	f        *os.File
	i        int
	password string
}

func open7z(path, password string, p *Progress) (src source, err error) {
	// The file is opened here, not by the library, so that it is closed even
	// when a crafted header makes the library panic: Windows keeps an open
	// file locked against deletion.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if src == nil {
			f.Close()
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	r, err := sevenzip.NewReaderWithPassword(newBlockReaderAt(f), fi.Size(), password)
	if err != nil {
		return nil, sevenZipError(err, password)
	}
	var total int64
	for _, f := range r.File {
		total += int64(f.UncompressedSize)
	}
	p.setTotal(total)
	return &sevenZipSource{r: r, f: f, password: password}, nil
}

func (s *sevenZipSource) next() (*member, error) {
	if s.i >= len(s.r.File) {
		return nil, io.EOF
	}
	f := s.r.File[s.i]
	s.i++
	mode := f.Mode()
	return &member{
		name:  f.Name,
		dir:   mode.IsDir(),
		skip:  mode&(os.ModeSymlink|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0,
		exec:  mode.Perm()&0o100 != 0,
		size:  int64(f.UncompressedSize),
		mtime: f.Modified,
		open: func() (io.ReadCloser, error) {
			rc, err := f.Open()
			if err != nil {
				return nil, sevenZipError(err, s.password)
			}
			return &sevenZipReader{rc, s.password}, nil
		},
	}, nil
}

func (s *sevenZipSource) countsInput() bool { return false }
func (s *sevenZipSource) Close() error      { return s.f.Close() }

type sevenZipReader struct {
	io.ReadCloser
	password string
}

func (r *sevenZipReader) Read(b []byte) (int, error) {
	n, err := r.ReadCloser.Read(b)
	if err != nil && err != io.EOF {
		err = sevenZipError(err, r.password)
	}
	return n, err
}

// sevenZipError turns a failure the library flags as involving encryption
// into the password errors: there is no password check in 7z, so a wrong one
// only shows as data that does not decode or does not match its checksum.
func sevenZipError(err error, password string) error {
	var re *sevenzip.ReadError
	if errors.As(err, &re) && re.Encrypted {
		if password == "" {
			return ErrEncrypted
		}
		return ErrBadPassword
	}
	return err
}
