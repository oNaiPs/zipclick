package main

import (
	"errors"
	"io"

	"github.com/nwaples/rardecode/v2"
)

// rarSource reads a RAR (versions 1.5 to 5, multi-volume included) as a
// stream, which is the only way through a solid one.
type rarSource struct {
	r         *rardecode.ReadCloser
	password  string
	encrypted bool // the current member
}

func openRar(path, password string, p *Progress) (source, error) {
	// An empty Password option reads as a wrong password, not as none.
	var opts []rardecode.Option
	if password != "" {
		opts = append(opts, rardecode.Password(password))
	}
	// Listing first gives the total for the progress bar, and finds out
	// about a password before anything is written. It only reads headers.
	files, err := rardecode.List(path, opts...)
	if err != nil {
		return nil, rarError(err, password, false)
	}
	var total int64
	for _, f := range files {
		if f.Encrypted && password == "" {
			return nil, ErrEncrypted
		}
		if !f.IsDir {
			total += f.UnPackedSize
		}
	}
	p.setTotal(total)
	r, err := rardecode.OpenReader(path, opts...)
	if err != nil {
		return nil, rarError(err, password, false)
	}
	return &rarSource{r: r, password: password}, nil
}

func (s *rarSource) next() (*member, error) {
	h, err := s.r.Next()
	if err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, rarError(err, s.password, false)
	}
	s.encrypted = h.Encrypted
	return &member{
		name:  h.Name,
		dir:   h.IsDir,
		skip:  h.LinkType != 0, // symbolic links, junctions and the like
		exec:  h.Mode().Perm()&0o100 != 0,
		mtime: h.ModificationTime,
		open:  func() (io.ReadCloser, error) { return io.NopCloser(&rarReader{s}), nil },
	}, nil
}

func (s *rarSource) countsInput() bool { return false }
func (s *rarSource) Close() error      { return s.r.Close() }

type rarReader struct{ s *rarSource }

func (r *rarReader) Read(b []byte) (int, error) {
	n, err := r.s.r.Read(b)
	if err != nil && err != io.EOF {
		err = rarError(err, r.s.password, r.s.encrypted)
	}
	return n, err
}

// rarError maps the library's password errors to ZipClick's. RAR 5 checks
// the password up front; RAR 4 cannot, so there a wrong one surfaces as a
// member that fails to decode or to match its checksum.
func rarError(err error, password string, encrypted bool) error {
	switch {
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted),
		errors.Is(err, rardecode.ErrBadPassword):
		if password == "" {
			return ErrEncrypted
		}
		return ErrBadPassword
	case encrypted && password != "":
		return ErrBadPassword
	}
	return err
}
